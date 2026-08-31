package cli

import (
	"bytes"
	"encoding/json"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/deligoez/cr/internal/activation"
	"github.com/deligoez/cr/internal/axis"
	"github.com/deligoez/cr/internal/brief"
	"github.com/deligoez/cr/internal/gh"
	"github.com/deligoez/cr/internal/git"
	"github.com/deligoez/cr/internal/intent"
	"github.com/deligoez/cr/internal/note"
	"github.com/deligoez/cr/internal/state"
	"github.com/deligoez/cr/internal/unit"
)

// briefedPayload is one §3.7 payload carrying something in every one of the six
// items, so a rendering that dropped one has something to drop.
//
// The axes are the half that is easy to get wrong: one axis runs, one is
// disabled by §4.5.2, and one is unavailable by §4.5.3, because §3.7.6 asks for
// all three categories with their reasons and a payload with nothing switched
// off would let a rendering that prints only the active list pass.
func briefedPayload() *brief.Brief {
	recordedAt := time.Date(2026, 8, 30, 9, 0, 0, 0, time.UTC)
	return &brief.Brief{
		Owner: "acme", Repo: "api", PR: 7, Round: 2,
		Head:      "be7e2c75aeb661ba3c96d7a7634f3f8e84bb7b91",
		MergeBase: "50b3dbeb56fdea59761d6818410de5212bdcc02d",
		Profile: brief.ProfileReport{
			ID: "laravel-pest", Selected: true, SelectionLayer: brief.LayerMarkerFiles,
		},
		Issue: brief.IssueReport{
			Key:    "CR-7",
			Origin: intent.KeyFromBranch,
			Text:   "An order exposes a total that includes tax.\n",
		},
		Claims: []intent.Claim{{
			ID: "CR-7#c1", Text: "An order exposes a total that includes tax.",
			Source: intent.ClaimFromAcceptance,
			Span:   "a total that includes tax",
			Stamp:  state.Stamp{Head: "be7e2c7", Round: 2},
		}},
		Drift: intent.Drift{
			Hash:    "f877d5d715d6f96c",
			Drifted: true,
			Claims: []intent.ClaimDrift{{
				ID: "CR-7#c1", ExtractedHash: "0123456789abcdef", SpanOccurs: false,
			}},
		},
		Units: []unit.Unit{{
			ID: "u1", Path: "src/Order.php", Side: git.Right,
			HunkRanges:   []unit.Range{{Start: 27, End: 46}},
			ChangedLines: 16, Hash: "38372bc96eb4010e",
			Formation: unit.ByAdjacency,
		}},
		Threads: []gh.Thread{{
			ID:         "PRRT_1",
			Anchor:     gh.Anchor{Path: "src/Money.php", Side: git.Right, StartLine: 14, Line: 14},
			Comment:    gh.Comment{ID: "PRRC_1", Author: "reviewer", Body: "Can this reuse Rounding::half()?"},
			AuthorType: gh.AuthorHuman,
			Replies:    []gh.Comment{},
		}},
		Notes: []note.Note{{
			ID: "CR-7#n1", Text: "the free-shipping threshold moved to 50.00",
			Source: note.SourceChat, PR: 7, RecordedAt: recordedAt,
		}},
		Axes: activation.Activation{
			Active: []string{axis.Correctness},
			Disabled: []activation.Disabled{{
				Axis: axis.Test, Rule: activation.RuleNoTestCommand,
				Reason: "the resolved profile generic declares no tests.cmd",
			}},
			Unavailable: []intent.Unavailable{{
				Axis: axis.Intent, Reason: "no issue key matched intent.key_pattern",
			}},
		},
	}
}

// rendered returns the payload as one mode prints it.
func rendered(t *testing.T, mode Mode) string {
	t.Helper()
	var printed bytes.Buffer
	out := &writer{out: &printed, mode: mode}
	require.NoError(t, out.emit(newBriefResult(briefedPayload())))
	return printed.String()
}

// §3.7's six items reach the reader in both renderings.
//
// The two are asserted separately and against different things, because they
// fail differently. The JSON document is what an agent parses, so it is decoded
// and each item is looked up by the key §3.7's numbering puts it under; the
// terminal rendering is what a human reads, so it is searched for the values
// themselves. A test that only checked the payload would pass on a command that
// assembled all six and printed two.
func TestTheBriefPrintsAllSixItemsOfSection37(t *testing.T) {
	t.Run("json", func(t *testing.T) {
		var printed map[string]any
		require.NoError(t, json.Unmarshal([]byte(rendered(t, ModeJSON)), &printed))

		// §3.7.1: identity, head, merge base, and the resolved profile
		// with the layer that selected it.
		assert.Equal(t, "acme", printed["owner"])
		assert.Equal(t, float64(7), printed["pr"])
		assert.Equal(t, "be7e2c75aeb661ba3c96d7a7634f3f8e84bb7b91", printed["head"])
		assert.Equal(t, "50b3dbeb56fdea59761d6818410de5212bdcc02d", printed["merge_base"])
		profile, ok := printed["profile"].(map[string]any)
		require.True(t, ok)
		assert.Equal(t, "laravel-pest", profile["id"])
		assert.Equal(t, brief.LayerMarkerFiles, profile["layer"])

		// §3.7.2: the key, its source, and the issue text.
		issue, ok := printed["issue"].(map[string]any)
		require.True(t, ok)
		assert.Equal(t, "CR-7", issue["key"])
		assert.Equal(t, string(intent.KeyFromBranch), issue["origin"])
		assert.Contains(t, issue["text"], "a total that includes tax")

		// §3.7.3: the claims, and whether the issue text has drifted.
		assert.Len(t, printed["claims"], 1)
		drift, ok := printed["drift"].(map[string]any)
		require.True(t, ok)
		assert.Equal(t, true, drift["drifted"])

		// §3.7.4: the units with their paths, hunk ranges, and hashes.
		units, ok := printed["units"].([]any)
		require.True(t, ok)
		require.Len(t, units, 1)
		formed, ok := units[0].(map[string]any)
		require.True(t, ok)
		assert.Equal(t, "src/Order.php", formed["path"])
		assert.Equal(t, "38372bc96eb4010e", formed["hash"])
		assert.Len(t, formed["hunk_ranges"], 1)

		// §3.7.5: the ingested threads and the notes.
		assert.Len(t, printed["threads"], 1)
		assert.Len(t, printed["notes"], 1)

		// §3.7.6: the active, disabled, and unavailable axes with their
		// reasons.
		axes, ok := printed["axes"].(map[string]any)
		require.True(t, ok)
		assert.Equal(t, []any{axis.Correctness}, axes["active"])
		assert.Len(t, axes["disabled"], 1)
		assert.Len(t, axes["unavailable"], 1)
		assert.Len(t, printed["honesty"], 2,
			"§4.5.4: every lens that did not run reaches the reader with its reason")
	})

	t.Run("text", func(t *testing.T) {
		printed := rendered(t, ModeText)

		assert.Contains(t, printed, "pull request acme/api#7 round 2")
		assert.Contains(t, printed, "head       be7e2c75aeb661ba3c96d7a7634f3f8e84bb7b91")
		assert.Contains(t, printed, "merge base 50b3dbeb56fdea59761d6818410de5212bdcc02d")
		assert.Contains(t, printed, "profile    laravel-pest, selected by match.files")

		assert.Contains(t, printed, "issue CR-7, from the branch")
		assert.Contains(t, printed, "| An order exposes a total that includes tax.")

		assert.Contains(t, printed, "claims 1 recorded; issue text has drifted")
		assert.Contains(t, printed, "CR-7#c1  span no longer occurs in the issue text")

		assert.Contains(t, printed, "units 1")
		assert.Contains(t, printed, "u1  src/Order.php RIGHT  27-46  38372bc96eb4010e  by adjacency")

		assert.Contains(t, printed, "threads 1")
		assert.Contains(t, printed, "PRRT_1  src/Money.php:14 RIGHT  by reviewer (human)")
		assert.Contains(t, printed, "notes 1")
		assert.Contains(t, printed, "CR-7#n1  from chat on pr 7")
		assert.Contains(t, printed, "the free-shipping threshold moved to 50.00")

		assert.Contains(t, printed, "axes active: correctness")
		assert.Contains(t, printed, "axis test disabled, per §4.5.2")
		assert.Contains(t, printed, "axis intent unavailable, per §4.5.3")
	})
}

// A round where every axis of §1.5 ran says so, rather than printing an active
// list and nothing after it.
//
// §4.5.4 is a report about the lenses that did not run, and the empty report is
// the one a reader cannot check: an active list of four and silence beneath it
// reads the same whether cr found nothing to disclose or forgot to print what
// it found. Saying it in words is what makes the two distinguishable, and this
// is the case the payload above cannot cover, because it carries two
// disclosures on purpose.
func TestARoundWithNothingDisabledSaysSoInBothRenderings(t *testing.T) {
	complete := briefedPayload()
	complete.Axes = activation.Activation{
		Active:      axis.IDs(),
		Disabled:    []activation.Disabled{},
		Unavailable: []intent.Unavailable{},
	}

	render := func(mode Mode) string {
		t.Helper()
		var printed bytes.Buffer
		out := &writer{out: &printed, mode: mode}
		require.NoError(t, out.emit(newBriefResult(complete)))
		return printed.String()
	}

	var document map[string]any
	require.NoError(t, json.Unmarshal([]byte(render(ModeJSON)), &document))
	assert.Equal(t, []any{}, document["honesty"],
		"§12.3: an empty report serialises as [], never as null")

	printed := render(ModeText)
	assert.Contains(t, printed, "axes active: intent, correctness, convention, test")
	assert.Contains(t, printed, "every axis of §1.5 ran; nothing was disabled or unavailable")
}
