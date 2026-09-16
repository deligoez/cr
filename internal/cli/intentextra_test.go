package cli

import (
	"encoding/json"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/deligoez/cr/internal/intent"
	"github.com/deligoez/cr/internal/note"
	"github.com/deligoez/cr/internal/state"
)

// extraDesign is the text of the extra intent file these tests append: a
// requirement the tracker never states, a paragraph of its own, and a link cr
// does not read.
const extraDesign = "The queue drains oldest first.\n" +
	"\n" +
	"Spec: https://docs.test/queue\n"

// separatorFor is §3.1.5's separator line for one path, with its newline.
func separatorFor(path string) string {
	return "--- cr intent file: " + path + " ---\n"
}

// recordedClaims is the claims document `cr claims record` prints.
type recordedClaims struct {
	Recorded []intent.Claim `json:"recorded"`
}

// recordClaimsWithExtra runs `cr claims record` over lines, reading the issue
// from issue and appending every path in extra, and returns what it stored.
func recordClaimsWithExtra(
	t *testing.T, issue string, extra []string, lines ...string,
) (recordedClaims, error) {
	t.Helper()
	args := []string{"claims", "record", fixturePR,
		writeOutside(t, "claims.ndjson", strings.Join(lines, "\n")+"\n"),
		"--repo", fixtureSlug, "--intent-file", issue}
	for _, path := range extra {
		args = append(args, "--intent-extra", path)
	}
	printed, err := runCLIPrinting(t, args...)
	if err != nil {
		return recordedClaims{}, err
	}
	var stored recordedClaims
	require.NoError(t, json.Unmarshal([]byte(printed), &stored))
	return stored, nil
}

// §3.1.5 through `cr brief`: the issue text is the tracker's — here the file
// §3.1.4 puts in its place — followed by each extra intent file's text under a
// separator line naming the path as given, in the order the flags gave them.
//
// The whole text is asserted rather than a substring of it, because §3.3.1
// checks a span against one part of exactly this document and §3.3's
// issue_hash is taken over the whole of it. `cr status` reads it back out of
// the state the brief wrote, so both are asserted from one run.
func TestBriefAppendsEveryExtraIntentFileUnderItsSeparatorLine(t *testing.T) {
	rerecordHome(t)
	design := writeOutside(t, "design.md", extraDesign)
	api := writeOutside(t, "api.md", "POST /jobs returns 202.\n")
	issue := writeOutside(t, "issue.txt", rerecordIssue)

	printed, err := runCLIPrinting(t, "brief", fixturePR, "--repo", fixtureSlug,
		"--issue", fixtureIssue, "--intent-file", issue,
		"--intent-extra", design, "--intent-extra", api)
	require.NoError(t, err)
	var briefed ingestBrief
	require.NoError(t, json.Unmarshal([]byte(printed), &briefed))

	joined := rerecordIssue +
		separatorFor(design) + extraDesign +
		separatorFor(api) + "POST /jobs returns 202.\n"
	assert.Equal(t, joined, briefed.Issue.Text)
	assert.Contains(t, briefed.Honesty, intent.LinkDisclosure("https://docs.test/queue"),
		"§3.1.7: a link an extra intent file carries is one the issue text carries")

	// §3.3.4 over the joined text: the separator lines are no paragraphs and
	// break the runs around them, so each part's paragraphs are its own.
	assert.Equal(t, intent.Paragraphs{Total: 4, Uncovered: []intent.Paragraph{
		{StartLine: 1, EndLine: 2, Text: rerecordIssue[:len(rerecordIssue)-1]},
		{StartLine: 4, EndLine: 4, Text: "The queue drains oldest first."},
		{StartLine: 6, EndLine: 6, Text: "Spec: https://docs.test/queue"},
		{StartLine: 8, EndLine: 8, Text: "POST /jobs returns 202."},
	}}, briefed.Paragraphs, "a separator line belongs to no paragraph")
	assert.Equal(t, briefed.Paragraphs, statusParagraphs(t).Paragraphs.Paragraphs,
		"`cr status` reports over the text the brief stored")
}

// §3.1.5's second source through `cr claims record`: `intent.extra_files`
// appends the same way the flag does, so a project that configures a document
// need not name it on every command.
func TestTheConfiguredExtraIntentFilesAreAppendedToo(t *testing.T) {
	layout, issue, _ := rerecordHome(t)
	design := writeOutside(t, "design.md", extraDesign)
	t.Setenv("CR_INTENT_EXTRA_FILES", `["`+design+`"]`)

	stored, err := recordClaimsWithExtra(t, issue, nil,
		`{"id":"`+fixtureIssue+`#c1","text":"Oldest first.","source":"file",`+
			`"file":"`+design+`","span":"drains oldest first"}`)
	require.NoError(t, err)

	require.Len(t, stored.Recorded, 1)
	assert.Equal(t, design, stored.Recorded[0].File)
	text, held, err := intent.StoredIssueText(layout, fixtureOwner, fixtureProject, fixturePRNumber, 1)
	require.NoError(t, err)
	assert.True(t, held)
	assert.Equal(t, rerecordIssue+separatorFor(design)+extraDesign, text,
		"the configured path is appended exactly as the flag's is")
}

// §3.3 and §3.3.1 through `cr claims record`, over one round reading one extra
// intent file: what a claim drawn from it must carry, and every way of getting
// it wrong.
//
// The accepted claim is asserted whole on the way out, because `file` is a row
// cr stores and §8.1.6 discloses from it. Each refusal is checked for §11.2's
// code 1 and for the field it names, so a rejection that fired for another
// reason fails the case it was written for.
func TestClaimsRecordHoldsAFileSourcedClaimToTheFileItNames(t *testing.T) {
	_, issue, _ := rerecordHome(t)
	design := writeOutside(t, "design.md", extraDesign)
	extra := []string{design}

	stored, err := recordClaimsWithExtra(t, issue, extra,
		`{"id":"`+fixtureIssue+`#c1","text":"Oldest first.","source":"file",`+
			`"file":"`+design+`","span":"drains oldest first"}`)
	require.NoError(t, err)
	require.Len(t, stored.Recorded, 1)
	assert.Equal(t, intent.ClaimFromFile, stored.Recorded[0].Source)
	assert.Equal(t, design, stored.Recorded[0].File)

	refused := []struct {
		name, field, line, problem string
	}{
		{
			name: "a file the round did not read", field: "file",
			line: `{"id":"` + fixtureIssue + `#c1","text":"Oldest first.","source":"file",` +
				`"file":"other.md","span":"drains oldest first"}`,
			problem: `names "other.md", which is not an extra intent file of this round`,
		},
		{
			name: "no file at all", field: "file",
			line: `{"id":"` + fixtureIssue + `#c1","text":"Oldest first.","source":"file",` +
				`"span":"drains oldest first"}`,
			problem: "is required by §3.3 when source is file",
		},
		{
			name: "a file on a claim of the tracker's text", field: "file",
			line: `{"id":"` + fixtureIssue + `#c1","text":"Load parses.","source":"description",` +
				`"file":"` + design + `","span":"load parses the file"}`,
			problem: "names an extra intent file, and §3.3 draws a claim sourced from description " +
				"out of another text",
		},
		{
			name: "a span of another part", field: "span",
			line: `{"id":"` + fixtureIssue + `#c1","text":"Load parses.","source":"file",` +
				`"file":"` + design + `","span":"load parses the file"}`,
			problem: "does not occur in the extra intent file " + design,
		},
		{
			name: "a span across the separator line", field: "span",
			line: `{"id":"` + fixtureIssue + `#c1","text":"Both.","source":"description",` +
				`"span":"Retries back off.\n--- cr intent file: "}`,
			problem: "does not occur in the issue text before the first §3.1.5 separator line",
		},
	}
	for _, claim := range refused {
		t.Run(claim.name, func(t *testing.T) {
			_, err := recordClaimsWithExtra(t, issue, extra, claim.line)
			var rejected *intent.RejectedClaimError
			require.ErrorAs(t, err, &rejected)
			assert.Equal(t, ExitValidation, exitCodeFor(err), "§3.3.1 rejects with exit code 1")
			assert.Equal(t, claim.field, rejected.Field)
			assert.Contains(t, rejected.Problem, claim.problem,
				"the refusal says which of §3.3's rules the claim broke")
		})
	}
}

// §8.1.6 through the commands: a record resting on a claim with `source: file`
// reaches the draft with a provenance region naming the claim and the extra
// intent file of §3.1.5 it was drawn from, exactly as one resting on a note
// names the note and its source.
//
// The claim is written into the round's state rather than recorded through the
// command, as the note-sourced case is: what is under test is the disclosure,
// and the round the draft fixture carries reads its issue text from no file.
func TestAClaimDrawnFromAnExtraIntentFileReachesTheDraftNamingTheFile(t *testing.T) {
	layout := detectedHome(t)
	meta, err := layout.ReadMeta(fixtureOwner, fixtureProject, fixturePRNumber)
	require.NoError(t, err)
	meta.IssueKey = fixtureIssue
	held, err := layout.LockPR(fixtureOwner, fixtureProject, fixturePRNumber)
	require.NoError(t, err)
	require.NoError(t, held.WriteMeta(&meta))
	require.NoError(t, held.Write(state.FileClaims, []byte(`{"id":"`+fixtureIssue+`#c1",`+
		`"text":"Load may not panic.","source":"file","file":"docs/design.md",`+
		`"span":"Load may not panic.","head":"`+meta.Head+`","round":1}`+"\n")))
	require.NoError(t, held.Unlock())

	rests := confirming("f1", 4)
	delete(rests, "rule")
	delete(rests, "citations")
	rests["claim"] = fixtureIssue + "#c1"
	_, err = runRecord(t, fixturePR, writeRecordFile(t, "merged.ndjson", rests), "--repo", fixtureSlug)
	require.NoError(t, err)

	assert.Contains(t, blockOf(t, draftedFixture(t, layout), "f1"), "<!-- cr:provenance -->\n"+
		"claim: "+fixtureIssue+"#c1 (source: file)\n"+
		"intent file: docs/design.md\n"+
		"<!-- cr:/provenance -->")
}

// A round whose claims name neither a note nor an extra intent file draws no
// provenance region from either, and a note-sourced claim beside a
// file-sourced one still names its own note: the two lookups are separate
// disclosures over one read of the round's claims.
func TestANoteSourcedClaimKeepsItsOwnRegionBesideAFileSourcedOne(t *testing.T) {
	layout := detectedHome(t)
	meta, err := layout.ReadMeta(fixtureOwner, fixtureProject, fixturePRNumber)
	require.NoError(t, err)
	meta.IssueKey = fixtureIssue
	recorded, err := note.Append(layout, fixtureIssue, "Load may panic during start-up only.",
		note.SourceMeeting, fixturePRNumber, time.Now())
	require.NoError(t, err)
	held, err := layout.LockPR(fixtureOwner, fixtureProject, fixturePRNumber)
	require.NoError(t, err)
	require.NoError(t, held.WriteMeta(&meta))
	require.NoError(t, held.Write(state.FileClaims, []byte(
		`{"id":"`+fixtureIssue+`#c1","text":"Load may not panic.","source":"file",`+
			`"file":"docs/design.md","span":"Load may not panic.",`+
			`"head":"`+meta.Head+`","round":1}`+"\n"+
			`{"id":"`+fixtureIssue+`#c2","text":"Start-up only.","source":"note",`+
			`"note_id":"`+recorded.ID+`","span":"Load may panic during start-up only.",`+
			`"head":"`+meta.Head+`","round":1}`+"\n")))
	require.NoError(t, held.Unlock())

	records := make([]map[string]any, 0, 2)
	for id, claim := range map[string]string{"f1": "#c1", "f2": "#c2"} {
		rests := confirming(id, 4)
		delete(rests, "rule")
		delete(rests, "citations")
		rests["claim"] = fixtureIssue + claim
		records = append(records, rests)
	}
	_, err = runRecord(t, fixturePR, writeRecordFile(t, "merged.ndjson", records...), "--repo", fixtureSlug)
	require.NoError(t, err)

	drafted := draftedFixture(t, layout)
	assert.Contains(t, blockOf(t, drafted, "f1"), "<!-- cr:provenance -->\n"+
		"claim: "+fixtureIssue+"#c1 (source: file)\n"+
		"intent file: docs/design.md\n"+
		"<!-- cr:/provenance -->")
	assert.Contains(t, blockOf(t, drafted, "f2"), "<!-- cr:provenance -->\n"+
		"claim: "+fixtureIssue+"#c2 (source: note)\n"+
		"note: "+recorded.ID+" (source: meeting)\n"+
		"<!-- cr:/provenance -->")
}

// An extra intent file the command cannot read fails the run with §11.2's code
// 3, naming `--intent-extra` and the path, and stores no issue text: a part
// silently dropped would leave every claim drawn from it unreachable while the
// round looked complete.
func TestAnUnreadableExtraIntentFileRefusesTheCommand(t *testing.T) {
	_, issue, _ := rerecordHome(t)
	missing := writeOutside(t, "absent.md", "")
	require.NoError(t, os.Remove(missing))

	_, err := runCLIPrinting(t, "brief", fixturePR, "--repo", fixtureSlug,
		"--issue", fixtureIssue, "--intent-file", issue, "--intent-extra", missing)

	var refused *intent.FileError
	require.ErrorAs(t, err, &refused)
	assert.Equal(t, ExitFile, exitCodeFor(err), "§11.2 codes a file cr cannot read 3")
	assert.Equal(t, intent.ExtraFlag, refused.Flag)
	assert.Contains(t, err.Error(), "--intent-extra "+missing)
}
