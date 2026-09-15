package draft

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/deligoez/cr/internal/render"
)

// verbMarker is one record's marker line for the verb tests.
func verbMarker(id, kind string) string {
	return Marker{ID: id, Kind: kind, Path: "a.go", Side: "RIGHT", StartLine: 1, Line: 2,
		Severity: "high", Grade: "cited"}.String()
}

// The owned regions the body tests place around an agent region.
const (
	verbLabel    = "<!-- cr:label -->\nlabel\n<!-- cr:/label -->"
	verbEvidence = "<!-- cr:evidence -->\n- a.go:1\n<!-- cr:/evidence -->"
)

// Each shape a block's text can take around its agent region, and the block
// Triaged leaves with a body replaced: the exact bytes, and the region
// render.AgentRegion reads back out of them, which is what §7.2's reading of
// the draft sees.
func TestABodyReplacesTheAgentRegionWhereverItStands(t *testing.T) {
	const body = "New body?"
	for _, tc := range []struct {
		name string
		text string
		want string
	}{
		{"a rendered block with no owned region", "\n\nOld body.\n\n", "\n\nNew body?\n\n"},
		{"between a label and the evidence", "\n\n" + verbLabel + "\n\nOld.\n\n" + verbEvidence + "\n",
			"\n\n" + verbLabel + "\n\nNew body?\n\n" + verbEvidence + "\n"},
		{"prose split around the evidence", "\n\nOne.\n\n" + verbEvidence + "\n\nTwo.\n",
			"\n\nNew body?\n\n" + verbEvidence + "\n\n\n"},
		{"an emptied region above the evidence", "\n\n" + verbEvidence + "\n",
			"\n\nNew body?\n\n" + verbEvidence + "\n"},
		{"an emptied region beneath a label", "\n\n" + verbLabel + "\n\n",
			"\n\n" + verbLabel + "\n\nNew body?\n\n"},
		{"an emptied block in the middle", "\n\n\n\n", "\n\nNew body?\n\n"},
		{"an emptied last block", "\n\n\n", "\n\nNew body?\n"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			next := verbMarker("f2", "finding") + "\n\nNext.\n"
			file := "header\n\n" + verbMarker("f1", "finding") + tc.text + next
			replaced := body + "\n"

			got, err := Triaged(file, "f1", VerbKeep, &replaced)

			require.NoError(t, err)
			assert.Equal(t, "header\n\n"+verbMarker("f1", "finding")+tc.want+next, got)
			bodies, err := Bodies(got)
			require.NoError(t, err)
			assert.Equal(t, map[string]string{"f1": body, "f2": "Next."}, bodies)
			assert.Equal(t, render.AgentRegion(tc.want), body)
		})
	}
}

// A marker line is edited in its one value and nowhere else, and a deletion
// takes the block from its marker to the next one, the last block included.
func TestTheVerbsEditOnlyWhatTheyName(t *testing.T) {
	first := verbMarker("f1", "finding") + "\n\nOne.\n\n"
	last := verbMarker("f2", "finding") + "\n\nTwo.\n"
	file := "header\n\n" + first + last

	for _, tc := range []struct {
		name string
		id   string
		verb Verb
		want string
	}{
		{"not-here on the first block", "f1", VerbNotHere, "header\n\n" + last},
		{"not-here on the last block", "f2", VerbNotHere, "header\n\n" + first},
		{"wrong", "f2", VerbWrong, "header\n\n" + first +
			strings.Replace(last, `disposition=""`, `disposition="wrong"`, 1)},
		{"soften", "f1", VerbSoften, "header\n\n" +
			strings.Replace(first, `kind="finding"`, `kind="question"`, 1) + last},
		{"keep", "f1", VerbKeep, file},
	} {
		t.Run(tc.name, func(t *testing.T) {
			got, err := Triaged(file, tc.id, tc.verb, nil)
			require.NoError(t, err)
			assert.Equal(t, tc.want, got)
		})
	}
}

// A draft that cannot be split into one block per record is refused as
// `cr draft` refuses it, and a record id with no block, or `soften` on a
// question, is refused naming the record.
func TestTriagedRefusesWhatItCannotName(t *testing.T) {
	block := verbMarker("f1", "finding") + "\n\nOne.\n"
	question := verbMarker("f2", "question") + "\n\nTwo?\n"

	_, err := Triaged(block+block, "f1", VerbKeep, nil)
	var malformed *MalformedMarkerError
	require.ErrorAs(t, err, &malformed)
	assert.Equal(t, 4, malformed.At, "the second block for one record")

	_, err = Triaged(block+"<!-- cr:record id=\"f2\" -->\n", "f1", VerbKeep, nil)
	require.ErrorAs(t, err, &malformed)
	assert.Equal(t, 4, malformed.At)

	_, err = Triaged(block, "f9", VerbKeep, nil)
	assert.Equal(t, &NoBlockError{ID: "f9"}, err)

	_, err = Triaged(block+question, "f2", VerbSoften, nil)
	assert.Equal(t, &SoftenQuestionError{ID: "f2", At: 4}, err)
}

// §7.2.4's four verbs are the only words ParseVerb admits, and `--body-file` is
// admitted beside the two that keep the block.
func TestTheVerbsAreSection724s(t *testing.T) {
	for word, keeps := range map[string]bool{"not-here": false, "wrong": false, "soften": true, "keep": true} {
		verb, err := ParseVerb(word)
		require.NoError(t, err)
		assert.Equal(t, Verb(word), verb)
		assert.Equal(t, keeps, verb.KeepsBlock(), word)
	}
	_, err := ParseVerb("discard")
	assert.EqualError(t, err, `"discard" is not a triage verb: §7.2.4 names not-here, wrong, soften, keep`)
}
