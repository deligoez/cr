package brief

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/deligoez/cr/internal/finding"
	"github.com/deligoez/cr/internal/gh"
	"github.com/deligoez/cr/internal/migrate"
	"github.com/deligoez/cr/internal/state"
)

// hashOf is §9.2's content hash of lines, computed the way cr computes it.
func hashOf(t *testing.T, lines ...string) string {
	t.Helper()
	hash, err := finding.AnchorContentHash(lines)
	require.NoError(t, err)
	return hash
}

// pushOverOneRecord stores one draft record at order.go:3 in round 1, on the
// anchor side given and with the extra fields given, lets prepare touch the
// state before the push, and briefs a push that writes two comment lines above
// the code, which moves it to line 5, the file's last.
func pushOverOneRecord(t *testing.T, side, extra string, prepare func(src *Sources)) (*Brief, *Sources, error) {
	t.Helper()
	dir, first, base := repository(t)
	src := sources(t, dir, answering(first, base, oneThread))
	_, err := Run(src)
	require.NoError(t, err)

	total := "func Total() int { return subtotal() + shipping() }"
	encoded, err := json.Marshal(finding.Anchor{
		Path: "order.go", Side: "RIGHT", StartLine: 3, Line: 3,
		ContentHash: hashOf(t, total), ContextBefore: []string{"package shop", ""}, ContextAfter: []string{},
	})
	require.NoError(t, err)
	anchor := strings.Replace(string(encoded), `"RIGHT"`, `"`+side+`"`, 1)
	held, err := src.Layout.LockPR(testOwner, testRepo, testPR)
	require.NoError(t, err)
	require.NoError(t, held.Write(state.FileFindings, []byte(
		`{"id":"f1","kind":"question","role":"correctness","class":"c","severity":"low","unit":"u1",`+
			`"summary":"s","evidence":"e","state":"draft","anchor":`+anchor+`,`+extra+
			`"head":"`+first+`","round":1}`+"\n")))
	require.NoError(t, held.Unlock())
	if prepare != nil {
		prepare(src)
	}

	require.NoError(t, os.WriteFile(filepath.Join(dir, "order.go"), []byte(
		"package shop\n\n// Total sums the order.\n// It adds shipping.\n"+total+"\n"), 0o600))
	runGit(t, dir, "add", "order.go")
	runGit(t, dir, "commit", "--quiet", "-m", "document the total")
	src.GH = answeringGH(runGit(t, dir, "rev-parse", "HEAD"), base)
	briefed, err := Run(src)
	return briefed, src, err
}

// §9.3.4 and §9.4.5 on one push: the author adds two lines above the code a
// drafted record sits on. The record is carried to the line the code moved to,
// into the new round and its unit, back to `draft`; a record whose code is gone
// is staled; a posted one is not touched. §9.4.8 reads the carried record's
// evidence again: the citation whose line changed loses its stamp, the one
// whose line did not keeps it, and the probe is cleared and reported.
func TestAPushCarriesTheRecordWhoseCodeMovedAndStalesTheOneWhoseCodeIsGone(t *testing.T) {
	dir, first, base := repository(t)
	src := sources(t, dir, answering(first, base, oneThread))
	_, err := Run(src)
	require.NoError(t, err)

	total := "func Total() int { return subtotal() + shipping() }"
	anchor := func(line int, hash string, before []string) string {
		encoded, err := json.Marshal(finding.Anchor{
			Path: "order.go", Side: "RIGHT", StartLine: line, Line: line,
			ContentHash: hash, ContextBefore: before, ContextAfter: []string{},
		})
		require.NoError(t, err)
		return string(encoded)
	}
	citations := `[{"path":"order.go","line":1,"content_hash":"` + hashOf(t, "package shop") + `"},` +
		`{"path":"order.go","line":3,"content_hash":"` + hashOf(t, total) + `"}]`
	stamp := `"head":"` + first + `","round":1`
	record := func(id, state, anchor, extra string) string {
		return `{"id":"` + id + `","kind":"finding","role":"correctness","axis":"correctness",` +
			`"class":"unchecked-error","grade":"probed",` +
			`"severity":"high","unit":"u1","summary":"s","evidence":"e","state":"` + state + `",` +
			`"anchor":` + anchor + `,` + extra + stamp + `}`
	}
	held, err := src.Layout.LockPR(testOwner, testRepo, testPR)
	require.NoError(t, err)
	require.NoError(t, held.Write(state.FileFindings, []byte(
		record("f1", "queued", anchor(3, hashOf(t, total), []string{"package shop", ""}),
			`"citations":`+citations+`,"probe":"p1",`)+"\n"+
			record("f2", "draft", anchor(3, hashOf(t, "code nobody kept"), []string{"package shop", ""}), "")+"\n"+
			record("f3", "posted", anchor(3, hashOf(t, total), []string{"package shop", ""}), "")+"\n")))
	require.NoError(t, held.Unlock())

	require.NoError(t, os.WriteFile(filepath.Join(dir, "order.go"), []byte(
		"package shop\n\n// Total sums the order.\n// It adds shipping.\n"+total+"\n"), 0o600))
	runGit(t, dir, "add", "order.go")
	runGit(t, dir, "commit", "--quiet", "-m", "document the total")
	second := runGit(t, dir, "rev-parse", "HEAD")
	src.GH = answeringGH(second, base)

	briefed, err := Run(src)
	require.NoError(t, err)
	require.Equal(t, 2, briefed.Round)
	assert.Equal(t, []string{"f1"}, briefed.Carried)
	assert.Equal(t, []string{"f2"}, briefed.Staled)

	stored := lines(t, src, state.FileFindings)
	require.Len(t, stored, 3)
	var carried finding.Finding
	raw, err := json.Marshal(stored[0])
	require.NoError(t, err)
	require.NoError(t, json.Unmarshal(raw, &carried))
	assert.Equal(t, finding.StateDraft, carried.State, "§9.3.4: a carried record returns to draft")
	assert.Equal(t, 2, carried.Round, "§9.4.5: the line moves to the new round")
	assert.Equal(t, second, carried.Head)
	assert.Equal(t, "u1", carried.Unit)
	assert.Equal(t, 5, carried.Anchor.Line, "the code moved two lines down")
	assert.Equal(t, 5, carried.Anchor.StartLine)
	assert.Equal(t, []string{"", "// Total sums the order.", "// It adds shipping."}, carried.Anchor.ContextBefore,
		"§9.4.5: the window is read again at the new place")
	assert.Empty(t, carried.Probe, "§9.4.8: the probe ran against code the push changed")
	require.Len(t, carried.Citations, 2)
	assert.NotEmpty(t, carried.Citations[0].ContentHash, "line 1 still holds what it was stamped with")
	assert.Empty(t, carried.Citations[1].ContentHash, "line 3 now holds a comment, so the stamp is gone")
	assert.Equal(t, finding.GradeArgued, carried.Grade,
		"§9.4.8: graded again without the probe; the citation that still resolves lies inside the "+
			"new unit (lines 1–5), which §6.2's `cited` row does not count, so `probed` falls to `argued`")

	assert.Equal(t, `"stale"`, string(stored[1]["state"]))
	assert.Equal(t, `"posted"`, string(stored[2]["state"]), "§9.4.1: a posted record's place is GitHub's")

	migrations := make([]migrate.Record, 0)
	for _, fields := range lines(t, src, state.FileMigrations) {
		raw, err := json.Marshal(fields)
		require.NoError(t, err)
		var one migrate.Record
		require.NoError(t, json.Unmarshal(raw, &one))
		migrations = append(migrations, one)
	}
	require.Len(t, migrations, 2, "§9.4.7: one line per record the increment migrated")
	assert.Equal(t, "f1", migrations[0].Record)
	assert.True(t, migrations[0].Carried)
	assert.Equal(t, "order.go:3", migrations[0].From)
	assert.Equal(t, "order.go:5", migrations[0].To)
	assert.Equal(t, "p1", migrations[0].Probe, "§9.4.8: the cleared probe is named")
	assert.Equal(t, 1, migrations[0].FromRound)
	assert.Equal(t, 2, migrations[0].Round)
	assert.Equal(t, "f2", migrations[1].Record)
	assert.False(t, migrations[1].Carried)
	assert.False(t, migrations[1].Placed)

	var moved []finding.Transition
	for _, fields := range lines(t, src, state.FileTransitions) {
		raw, err := json.Marshal(fields)
		require.NoError(t, err)
		var one finding.Transition
		require.NoError(t, json.Unmarshal(raw, &one))
		moved = append(moved, one)
	}
	require.Len(t, moved, 2)
	assert.Equal(t, "f1", moved[0].Record)
	assert.Equal(t, finding.StateDraft, moved[0].To, "§9.1: queued → draft by cr brief")
}

// §9.4.2 reads only the head's tree, so a record anchored on the LEFT side is
// not placed, and its migration line says why with the `left` key.
func TestALeftSideRecordIsReportedUnderTheLeftKey(t *testing.T) {
	briefed, _, err := pushOverOneRecord(t, "LEFT", "", nil)
	require.NoError(t, err)

	assert.Empty(t, briefed.Carried)
	require.Len(t, briefed.Migrations, 1)
	assert.Equal(t, migrate.KeyLeft, briefed.Migrations[0].Key)
}

// §9.4.3 through the carry: code the push moved into another file the diff
// touches is found there, and the record follows it into that file's unit.
// v0.6's migration searched the anchor's own file only.
func TestAPushThatMovesCodeToAnotherFileCarriesTheRecordThere(t *testing.T) {
	dir, first, base := repository(t)
	src := sources(t, dir, answering(first, base, oneThread))
	_, err := Run(src)
	require.NoError(t, err)

	total := "func Total() int { return subtotal() + shipping() }"
	encoded, err := json.Marshal(finding.Anchor{
		Path: "order.go", Side: "RIGHT", StartLine: 3, Line: 3,
		ContentHash: hashOf(t, total), ContextBefore: []string{"package shop", ""}, ContextAfter: []string{},
	})
	require.NoError(t, err)
	held, err := src.Layout.LockPR(testOwner, testRepo, testPR)
	require.NoError(t, err)
	require.NoError(t, held.Write(state.FileFindings, []byte(
		`{"id":"f1","kind":"question","role":"correctness","class":"c","severity":"low","unit":"u1",`+
			`"summary":"s","evidence":"e","state":"draft","anchor":`+string(encoded)+`,`+
			`"head":"`+first+`","round":1}`+"\n")))
	require.NoError(t, held.Unlock())

	require.NoError(t, os.WriteFile(filepath.Join(dir, "order.go"), []byte("package shop\n"), 0o600))
	require.NoError(t, os.WriteFile(filepath.Join(dir, "total.go"), []byte("package shop\n\n"+total+"\n"), 0o600))
	runGit(t, dir, "add", "order.go", "total.go")
	runGit(t, dir, "commit", "--quiet", "-m", "move the total")
	src.GH = answeringGH(runGit(t, dir, "rev-parse", "HEAD"), base)

	briefed, err := Run(src)
	require.NoError(t, err)
	require.Equal(t, []string{"f1"}, briefed.Carried)
	require.Len(t, briefed.Migrations, 1)
	assert.Equal(t, "total.go:3", briefed.Migrations[0].To)

	var carried finding.Finding
	raw, err := json.Marshal(lines(t, src, state.FileFindings)[0])
	require.NoError(t, err)
	require.NoError(t, json.Unmarshal(raw, &carried))
	assert.Equal(t, "total.go", carried.Anchor.Path)
	var unitPath string
	for _, formed := range briefed.Units {
		if formed.ID == carried.Unit {
			unitPath = formed.Path
		}
	}
	assert.Equal(t, "total.go", unitPath, "§9.4.5: the unit that contains the migrated anchor")
}

// §9.4.8's re-reading reaches the file's last line: a citation of the line the
// push left as the last one, still holding what it was stamped with, keeps its
// stamp.
func TestACitationOfTheLastLineKeepsItsStamp(t *testing.T) {
	total := "func Total() int { return subtotal() + shipping() }"
	briefed, src, err := pushOverOneRecord(t, "RIGHT",
		`"citations":[{"path":"order.go","line":5,"content_hash":"`+hashOf(t, total)+`"}],`, nil)
	require.NoError(t, err)
	require.Equal(t, []string{"f1"}, briefed.Carried)

	var carried finding.Finding
	raw, err := json.Marshal(lines(t, src, state.FileFindings)[0])
	require.NoError(t, err)
	require.NoError(t, json.Unmarshal(raw, &carried))
	require.Len(t, carried.Citations, 1)
	assert.NotEmpty(t, carried.Citations[0].ContentHash, "line 5 still holds the total")
}

// answeringGH is answering at a moved head, with no threads.
func answeringGH(head, base string) gh.Client {
	return gh.WithRunner(answering(head, base, noThreads))
}

// §7.1.7 through the carry: a body the reviewer edited in the closing round's
// draft travels with the record, and the migration line carries it. No
// rendered.json stands beside the draft here, so every body in it reads as
// edited.
func TestAPushCarriesTheBodyTheReviewerEdited(t *testing.T) {
	const edited = "Does the total count the shipping twice?"
	briefed, _, err := pushOverOneRecord(t, "RIGHT", "", func(src *Sources) {
		draftFile := src.Layout.RoundFile(testOwner, testRepo, testPR, 1, state.FileDraft)
		require.NoError(t, os.MkdirAll(filepath.Dir(draftFile), 0o700))
		require.NoError(t, os.WriteFile(draftFile, []byte(`<!-- cr:record id="f1" kind="question" `+
			`path="order.go" side="RIGHT" start_line="3" line="3" severity="low" grade="argued" `+
			`disposition="" -->`+"\n\n"+edited+"\n"), 0o600))
	})
	require.NoError(t, err)

	require.Equal(t, []string{"f1"}, briefed.Carried)
	require.Len(t, briefed.Migrations, 1)
	assert.Equal(t, edited, briefed.Migrations[0].Body)
}

// A closing round's draft cr cannot read stops the carry rather than carrying
// the record with no body, which would drop an edit the reviewer made. A
// directory where draft.md belongs is a read that fails with nothing read.
func TestADraftCrCannotReadStopsTheCarry(t *testing.T) {
	_, _, err := pushOverOneRecord(t, "RIGHT", "", func(src *Sources) {
		require.NoError(t, os.MkdirAll(src.Layout.RoundFile(testOwner, testRepo, testPR, 1, state.FileDraft), 0o700))
	})

	require.Error(t, err)
}
