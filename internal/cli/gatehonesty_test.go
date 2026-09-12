package cli

import (
	"bytes"
	"encoding/json"
	"maps"
	"os"
	"regexp"
	"slices"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/deligoez/cr/internal/state"
)

// readClaiming is wording that would present §8.5's gate as evidence that
// somebody looked at the review: a reading, a reader, an approval, a human, or
// a draft that changed.
//
// It is matched on whole words, case-insensitively, with an underscore counted
// as a separator so a key spelled `draft_edited` is caught as surely as the
// sentence — and a payload hash or a count cannot trip it, and "already" is
// not "read".
var readClaiming = regexp.MustCompile(
	`(?i)(?:^|[^a-z])(read|reads|reader|reading|reviewed|approved|approval|looked|seen|saw|` +
		`inspected|human|person|someone|edited|changed|modified|draft)(?:[^a-z]|$)`)

// gateOutputs are the parts of a `cr post` run that describe the gate: the
// terminal's closing line and the JSON document without the payload, which is
// the author's content rather than cr's account of the gate.
type gateOutputs struct {
	line     string
	document map[string]json.RawMessage
}

// gateOutputsOf runs `cr post` with args and returns what it said about the
// gate, in both of §12.1's shapes.
func gateOutputsOf(t *testing.T, args ...string) gateOutputs {
	t.Helper()
	printed, err := runPost(t, args...)
	require.NoError(t, err)

	document := map[string]json.RawMessage{}
	require.NoError(t, json.Unmarshal([]byte(printed), &document))
	delete(document, "payload")

	var reported struct{ posting }
	require.NoError(t, json.Unmarshal([]byte(printed), &reported))
	var rendered bytes.Buffer
	out := &writer{out: &rendered, mode: ModeText}
	return gateOutputs{line: reported.line(out), document: document}
}

// §8.5.4 over a real run, dry and confirmed: every output describing the gate
// says whether `--confirm` was given and claims nothing about a reading, and
// the round summary's share of the gate is the confirmation and the payload
// hash beside §10.3's posted count, and nothing more.
//
// The draft is rewritten before either run, which is what §8.1.2 has the agent
// do to every body. A summary or a line that reported that edit would be
// offering the one signal §8.5.4 says measures nothing, and readClaiming would
// catch it by the word it would have to use.
func TestTheGateReportsConfirmationAndNeverAReading(t *testing.T) {
	first := aCitedRecord("f1")
	layout := draftedHome(t, first, aCitedRecord("f2"))
	redraft(t)
	rewritten := strings.Replace(readDraft(t, layout), first.Summary+"\n\n"+first.Evidence,
		"The agent's rewrite in render.lang: Decode's error is dropped on this line.", 1)
	require.NotEqual(t, readDraft(t, layout), rewritten, "§8.1.2's rewrite reached the body")
	require.NoError(t, os.WriteFile(
		layout.RoundFile(draftOwner, draftRepo, draftPRNum, draftRound, state.FileDraft),
		[]byte(rewritten), 0o600))
	redraft(t)

	dry := gateOutputsOf(t, draftPR, "--repo", draftSlug)
	ghShimming(t, builtPayload(t))
	confirmed := gateOutputsOf(t, draftPR, "--repo", draftSlug, "--confirm")

	assert.Equal(t, "not posted: --confirm was not given, and nothing was sent to GitHub", dry.line)
	assert.Equal(t, "posted: --confirm was given, and the review was sent to GitHub", confirmed.line,
		"§8.5.4: the output describing the gate says that confirmation was given")
	assert.JSONEq(t, "false", string(dry.document["confirm_given"]))
	assert.JSONEq(t, "true", string(confirmed.document["confirm_given"]),
		"§8.5.4: the JSON document says it too, not only the terminal")

	for name, run := range map[string]gateOutputs{"dry run": dry, "confirmed run": confirmed} {
		assert.Emptyf(t, readClaiming.FindAllString(run.line, -1),
			"§8.5.4: the %s's line claims a reading: %q", name, run.line)
		for key, raw := range run.document {
			assert.Emptyf(t, readClaiming.FindAllString(key+" "+string(raw), -1),
				"§8.5.4: the %s's %q claims a reading: %s", name, key, raw)
		}
	}

	body, err := layout.ReadRound(draftOwner, draftRepo, draftPRNum, draftRound, state.FileSummary)
	require.NoError(t, err)
	var summary map[string]json.RawMessage
	require.NoError(t, json.Unmarshal(body, &summary))
	share := map[string]json.RawMessage{}
	for key, owner := range summaryOwners {
		if owner == ownerPost {
			share[key] = summary[key]
		}
	}
	assert.Equal(t, []string{"confirm_given", "payload_hash", "posted"}, slices.Sorted(maps.Keys(share)),
		"§8.5.4: the confirmation and the payload hash, beside §10.3's posted count, and nothing more")
	assert.JSONEq(t, "true", string(share["confirm_given"]))
	for key, raw := range share {
		assert.Emptyf(t, readClaiming.FindAllString(key+" "+string(raw), -1),
			"§8.5.4: the round summary's %q claims a reading: %s", key, raw)
	}
}

// readClaiming is only worth what it catches, so it is shown catching the
// sentences §8.5.4 forbids and passing the ones it requires.
func TestReadClaimingCatchesTheForbiddenWording(t *testing.T) {
	for _, forbidden := range []string{
		"posted: the review was read and sent to GitHub",
		"a human reviewed the draft",
		"draft.md changed since it was rendered",
		`"draft_edited": true`,
		"approved by the reviewer",
	} {
		assert.NotEmptyf(t, readClaiming.FindAllString(forbidden, -1), "%q", forbidden)
	}
	for _, allowed := range []string{
		"posted: --confirm was given, and the review was sent to GitHub",
		`"payload_hash": "273f8e23bae911fc"`,
		"0 finding(s) dropped as already posted",
	} {
		assert.Emptyf(t, readClaiming.FindAllString(allowed, -1), "%q", allowed)
	}
}
