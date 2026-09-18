package draft

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/deligoez/cr/internal/finding"
	"github.com/deligoez/cr/internal/render"
)

// suggesting is a queued record carrying a suggestion of the given origin.
func suggesting(origin finding.Origin) *finding.Finding {
	record := aRecord("f1")
	record.Suggestion = "\tif err := dec.Decode(&body); err != nil {"
	record.SuggestionOrigin = origin
	return record
}

// §7.1.3: a suggestion is rendered as a fenced `suggestion` block inside the
// body.
//
// The language tag is the whole of what makes it a suggestion. GitHub turns a
// fence tagged `suggestion` into a one-click replacement of the comment's own
// lines and treats every other tag as a quotation, so the tag is the difference
// between an offer the author can accept and a code sample they have to retype.
func TestASuggestionIsRenderedAsAFencedSuggestionBlock(t *testing.T) {
	rendered := renderOf(t, suggesting(finding.OriginAgent))

	assert.Contains(t, rendered, "```suggestion\n\tif err := dec.Decode(&body); err != nil {\n```")
	assert.Greater(t, strings.Index(rendered, "```suggestion"),
		strings.Index(rendered, "The error Decode returns is dropped."),
		"§7.1.3 puts the block inside the body, beneath the prose it belongs to")
}

// §7.1.3: a suggestion with `suggestion_origin: rule` is labelled as machine
// generated, and one the agent wrote is not.
//
// Both directions are asserted, because a label on every block would say
// nothing about any of them. What §2.6.2.4 is disclosing is that this
// particular replacement had no author — cr cannot establish it compiles,
// parses, or preserves behaviour — and a reviewer who reads the same sentence
// above every suggestion stops reading it.
func TestOnlyAMachineGeneratedSuggestionIsLabelled(t *testing.T) {
	for _, c := range []struct {
		name     string
		origin   finding.Origin
		labelled bool
	}{
		{name: "produced by a rule's fix", origin: finding.OriginRule, labelled: true},
		{name: "written by the agent", origin: finding.OriginAgent, labelled: false},
		{name: "carrying no origin at all", origin: "", labelled: false},
	} {
		t.Run(c.name, func(t *testing.T) {
			rendered := renderOf(t, suggesting(c.origin))

			require.Contains(t, rendered, "```suggestion", "every case renders the block")
			if c.labelled {
				assert.Contains(t, rendered, machineGenerated)
				assert.Less(t, strings.Index(rendered, machineGenerated),
					strings.Index(rendered, "```suggestion"),
					"the label introduces the block rather than trailing it")
				return
			}
			assert.NotContains(t, rendered, machineGenerated)
		})
	}
}

// The label says what cr does not know, which is §2.6.2.4's own sentence rather
// than a paraphrase of where the text came from. A reader who is told only
// "machine generated" has to supply the consequence themselves.
func TestTheLabelNamesWhatCrCannotEstablish(t *testing.T) {
	for _, claim := range []string{"compiles", "parses", "preserves behaviour"} {
		assert.Contains(t, machineGenerated, claim)
	}
}

// A record with no suggestion renders no fence, so the block is a consequence
// of the field and not of the renderer. §6.1's table makes `suggestion`
// optional, and most records carry none.
func TestARecordWithoutASuggestionRendersNoBlock(t *testing.T) {
	rendered := renderOf(t, aRecord("f1"))

	assert.NotContains(t, rendered, "```suggestion")
	assert.NotContains(t, rendered, machineGenerated)
}

// The block sits in the agent's own region and carries no `cr`-owned delimiter.
//
// §8.1.3 refuses a body containing any `<!-- cr:` sequence with exit code 1, so
// a label written as an owned region here would make every record carrying a
// machine suggestion unpostable — and §7.1.6 would preserve the agent's edit of
// a region §8.1.3 says cr regenerates.
//
// The comment as a whole does carry an owned region for such a record: §8.1.6
// discloses `suggestion_origin: rule` in the provenance region. So the claim
// is read off the agent region AgentRegion recovers, and the label and the
// fence are asserted to be in it — a label moved into an owned region would
// be stripped with that region and fail here rather than pass unseen.
func TestTheSuggestionBlockOpensNoCrOwnedRegion(t *testing.T) {
	rendered := renderOf(t, suggesting(finding.OriginRule))

	body, found := strings.CutPrefix(rendered, "<!-- cr:record ")
	require.True(t, found)
	_, body, found = strings.Cut(body, "-->\n\n")
	require.True(t, found)
	agent := render.AgentRegion(body)
	assert.NotContains(t, agent, "<!-- cr:")
	assert.Contains(t, agent, machineGenerated)
	assert.Contains(t, agent, "```suggestion")
}
