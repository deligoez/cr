package draft

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/deligoez/cr/internal/finding"
	"github.com/deligoez/cr/internal/render"
)

// §7.1.6: a preserved body replaces the agent region and nothing else. The
// question's label is cr's and is built again from the record; the body is the
// reviewer's and is carried exactly as they left it.
func TestAPreservedBodyReplacesOnlyTheAgentRegion(t *testing.T) {
	question := aRecord("f1")
	question.Kind, question.Grade = finding.KindQuestion, finding.GradeArgued
	question.Summary = "Does the caller ever see the error Decode returns?"
	kept := "The reviewer's own question: does anything downstream see this error?"

	rendered, err := Render([]*finding.Finding{question}, render.LangEN, nil, map[string]string{"f1": kept})
	require.NoError(t, err)

	label, err := render.QuestionLabelRegion(render.LangEN, finding.GradeArgued)
	require.NoError(t, err)
	assert.Contains(t, rendered, label, "the owned region is regenerated from the record")
	assert.NotContains(t, rendered, question.Summary, "cr's own body is not rendered beside the kept one")
	_, comment, _ := strings.Cut(rendered, "\n")
	assert.Equal(t, kept, render.AgentRegion(comment), "the agent region is the kept body, byte for byte")
}

// A preserved body is held to §8.1.3 exactly as a rendered one is. A reviewer
// who deleted half of an owned region leaves its other marker in the agent
// region, and the regeneration stops naming the record rather than writing a
// draft whose regions no longer pair.
func TestAPreservedBodyIsHeldToSection813(t *testing.T) {
	kept := "A body that swallowed half a region.\n\n" + render.Reserved + "/label -->"

	_, err := Render([]*finding.Finding{aRecord("f1")}, render.LangEN, nil, map[string]string{"f1": kept})

	var refused *render.BodyError
	require.ErrorAs(t, err, &refused)
	assert.Equal(t, "f1", refused.Record)
}
