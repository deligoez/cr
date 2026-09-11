package render

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/deligoez/cr/internal/finding"
	"github.com/deligoez/cr/internal/probe"
)

// §8.1.5: cr refuses to post a `kind=question` body containing no `?`
// character, naming the record id.
//
// The bodies refused are the ones a question actually arrives as when nobody
// rewrote it: §8.1.2's English summary and evidence, which are statements. A
// question mark that is not the ASCII character is refused too, since §8.1.5
// names the character and both of `render.lang`'s languages write it.
func TestADeclarativeQuestionBodyIsRefused(t *testing.T) {
	for name, body := range map[string]string{
		"the initial body, never rewritten": "The error Decode returns is dropped.\n\n" +
			"The call's second result is assigned to the blank identifier.",
		"a statement ending in a full stop": "Decode's error does not reach the caller.",
		"a full-width question mark only":   "Does Decode's error reach the caller？",
		"an inverted question mark only":    "¿Decode's error reaches the caller.",
	} {
		t.Run(name, func(t *testing.T) {
			err := ValidatePostBody("f5", finding.KindQuestion, body)

			var refused *BodyError
			require.ErrorAs(t, err, &refused)
			assert.Equal(t, "f5", refused.Record, "§8.1.5: the refusal names the record id")
			assert.Contains(t, err.Error(), "f5")
			assert.Contains(t, err.Error(), `"?"`, "and says what the body lacks")
		})
	}
}

// A question holding the character anywhere passes, and a finding is never
// asked for one: §8.1.5 is about the question register, and an assertion that
// happens to end in a full stop is exactly what a finding should be.
func TestOnlyAQuestionIsAskedForAQuestionMark(t *testing.T) {
	for name, body := range map[string]string{
		"a question at the end":    "The second result is discarded. Is that intended?",
		"a question at the start":  "Is Decode's error dropped? The second result is discarded.",
		"a question inside a code": "Does `v, _ := dec.Decode(&body)` drop an error that matters? See line 42.",
	} {
		t.Run(name, func(t *testing.T) {
			assert.NoError(t, ValidatePostBody("f5", finding.KindQuestion, body))
		})
	}

	assert.NoError(t, ValidatePostBody("f6", finding.KindFinding,
		"Decode's error does not reach the caller."),
		"a finding asserts, and needs no question mark")
}

// §8.1.3 applies §8.1.5 to the agent region alone. A `?` that stands only in a
// cr-owned region — here the runner's own output in the evidence region — is
// not the agent asking anything, and does not let a declarative body through.
func TestAQuestionMarkInAnOwnedRegionDoesNotCount(t *testing.T) {
	label, err := QuestionLabelRegion(LangEN, finding.GradeProbed)
	require.NoError(t, err)
	comment := Comment{
		Label: label,
		Body:  "Decode's error does not reach the caller.",
		Evidence: probeRegion(t, &probe.Record{
			Kind: probe.Mutation, Target: "internal/api/handler.go:42",
			Result: "no-test-failed", OutputTail: "did the suite run? yes: 12 passed\n",
		}),
	}
	require.Contains(t, comment.String(), "?", "the comment as a whole does hold a question mark")

	err = ValidatePostBody("f7", finding.KindQuestion, AgentRegion(comment.String()))
	var refused *BodyError
	require.ErrorAs(t, err, &refused)
	assert.Equal(t, "f7", refused.Record)
}

// §8.1.3's two refusals come first and hold for a finding and a question alike:
// an empty body is not rescued by being a finding, and a body carrying the
// reserved sequence is not rescued by holding a question mark.
func TestThePostCheckKeepsSection813sRefusals(t *testing.T) {
	for _, kind := range []finding.Kind{finding.KindFinding, finding.KindQuestion} {
		for _, body := range []string{"", "Is this reachable? " + Reserved + "label -->"} {
			var refused *BodyError
			require.ErrorAs(t, ValidatePostBody("f8", kind, body), &refused, "%s: %q", kind, body)
			assert.NotContains(t, refused.Problem, `"?"`, "the §8.1.3 refusal, not §8.1.5's")
		}
	}
}
