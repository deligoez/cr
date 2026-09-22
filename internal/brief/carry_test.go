package brief

import (
	"encoding/json"
	"os"
	"path/filepath"
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
		return `{"id":"` + id + `","kind":"finding","role":"correctness","class":"unchecked-error",` +
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

// answeringGH is answering at a moved head, with no threads.
func answeringGH(head, base string) gh.Client {
	return gh.WithRunner(answering(head, base, noThreads))
}
