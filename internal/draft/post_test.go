package draft

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/deligoez/cr/internal/finding"
	"github.com/deligoez/cr/internal/render"
)

// §8.1.3 at post time: every cr-owned region of a posted body is the one the
// draft carried, byte for byte, because both are built by commentOf.
//
// The three records are one of each grade, so all three owned regions are in
// play: §8.1.4's label on the question, §8.1.7's citations on the cited record
// and its probe on the probed one. The comparison is the whole comment against
// the whole block, which is what makes it an identity rather than a spot check
// — a second renderer that agreed about the evidence and differed by a blank
// line would fail here.
func TestAPostedBodyIsTheDraftedBlockThroughTheSamePath(t *testing.T) {
	records := assertingRecords()
	drafted, err := Render(records, render.LangEN, withProbe(), nil)
	require.NoError(t, err)

	bodies, err := PostBodies(records, render.LangEN, withProbe(), nil)
	require.NoError(t, err)

	require.Len(t, bodies, len(records))
	for _, record := range records {
		assert.Contains(t, drafted, markerOf(record).String()+"\n\n"+bodies[record.ID]+"\n",
			"%s: the posted comment is the drafted block's, byte for byte", record.ID)
	}
	cited, err := render.CitedEvidence("f1", records[0].Citations)
	require.NoError(t, err)
	assert.Contains(t, bodies["f1"], cited,
		"§8.1.7's region is regenerated at post time rather than dropped")
	probed, err := render.ProbeEvidence("f2", aProbe(), 4096)
	require.NoError(t, err)
	assert.Contains(t, bodies["f2"], probed)
}

// A body the reviewer edited is the body that is posted, and the owned regions
// around it are still cr's.
//
// This is §8.1.2 and §8.1.3 meeting: the agent region reaches the author as the
// reviewer left it in draft.md, and the label above it is regenerated from the
// record whatever was typed inside its markers.
func TestAPreservedBodyIsWhatReachesTheAuthor(t *testing.T) {
	asked := aRecord("f1")
	asked.Kind, asked.Grade = finding.KindQuestion, finding.GradeArgued
	rewritten := "Does the error Decode returns ever reach the caller?"

	bodies, err := PostBodies(
		[]*finding.Finding{asked}, render.LangEN, nil, map[string]string{"f1": rewritten})
	require.NoError(t, err)

	label, err := render.QuestionLabelRegion(render.LangEN, finding.GradeArgued)
	require.NoError(t, err)
	assert.Equal(t, label+"\n\n"+rewritten, bodies["f1"],
		"§8.1.3: the reviewer's region, beneath the label cr regenerates")
}

// §8.1.5 is applied when the body is posted and not when it is drafted.
//
// The draft is where the rewrite happens, so refusing it there would refuse the
// only place a question can be written; refusing it here is the last moment a
// forced question whose body still asserts can be stopped, and it names the
// record so the reviewer knows which block to open.
func TestAQuestionWrittenAsAStatementIsRefusedOnlyAtPostTime(t *testing.T) {
	asked := aRecord("f1")
	asked.Kind, asked.Grade = finding.KindQuestion, finding.GradeArgued

	drafted, err := Render([]*finding.Finding{asked}, render.LangEN, nil, nil)
	require.NoError(t, err, "§8.1.2's initial body is a statement by construction")
	require.Contains(t, drafted, asked.Summary)

	_, err = PostBodies([]*finding.Finding{asked}, render.LangEN, nil, nil)

	require.Error(t, err)
	assert.Contains(t, err.Error(), "f1", "§8.1.5's refusal names the record id")
}
