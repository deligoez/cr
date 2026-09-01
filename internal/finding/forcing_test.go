package finding

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/deligoez/cr/internal/probe"
)

// argued is a record graded argued that the agent wrote as an assertion, which
// is what §6.3.1 exists to catch.
func argued() *Finding {
	record := aGradedRecord()
	record.Kind, record.Grade = KindFinding, GradeArgued
	return record
}

// Invariant 4 and §6.3.1: a record graded `argued` is forced to
// `kind: question`.
//
// `argued` is §6.2's "neither of the above" — no experiment cr ran, and no
// location cr resolved outside the record's own unit. Such a record may still
// be right and is still worth sending, but it may not be sent as an assertion,
// so P2 re-shapes the uncertainty rather than suppressing the record.
//
// A record the agent already wrote as a question is not forced, and that is not
// a detail: §6.3.2 counts the forcings per class, and counting a record nobody
// moved would report cr restraining an agent that restrained itself.
func TestARecordGradedArguedIsForcedToAQuestion(t *testing.T) {
	record := argued()

	assert.True(t, ForceQuestion(record), "the record was in the assertion register")
	assert.Equal(t, KindQuestion, record.Kind, "§6.3.1: an argued record is a question")

	assert.False(t, ForceQuestion(record), "and the second application moves nothing")
	assert.Equal(t, KindQuestion, record.Kind)

	asked := argued()
	asked.Kind = KindQuestion
	assert.False(t, ForceQuestion(asked), "an agent that asked was not forced to")
	assert.Equal(t, KindQuestion, asked.Kind)
}

// §6.3.1's three moments: the forcing is applied at record time, again at draft
// time, and again at post time immediately before the payload is built, and it
// re-applies at each.
//
// The record is flipped back to `kind: finding` before each application, which
// is the fault the repetition exists to catch. It is not hypothetical: §6.2.1's
// inputs move within a round — §4.1.6 replaces the mapping and §3.3.1 clears it
// — so Regrade can lower a record to `argued` after it was recorded, and §7.2's
// triage rewrites records between the draft and the post. A forcing applied
// once would leave such a record in the assertion register for the rest of the
// round, and the last moment is the one that matters most, because it is the
// only one that sees the payload the author will actually receive.
//
// The moments are named by the actors §9.1 gives them, so that a reader can
// find the command each one belongs to. `cr record` applies it today;
// §6.3.1's other two are applied by the commands that build the draft and the
// payload, and each applies this same function rather than a rule of its own.
func TestTheForcingReAppliesAtEachOfSection631sThreeMoments(t *testing.T) {
	record := argued()

	for _, moment := range []struct {
		at Actor
		is string
	}{
		{ActorRecord, "record time"},
		{ActorDraft, "draft time"},
		{ActorPostConfirm, "post time, immediately before the payload is built"},
	} {
		t.Run(moment.is, func(t *testing.T) {
			// Whatever moved it back — a recomputation, a triage
			// edit, or a hand-edited draft — the register is the
			// agent's assertion again when this moment starts.
			record.Kind = KindFinding

			assert.True(t, ForceQuestion(record),
				"%s: §6.3.1 applies the forcing again", moment.at)
			assert.Equal(t, KindQuestion, record.Kind)
		})
	}
}

// The forcing reads the grade and nothing else, and it never raises a register.
//
// A `probed` or `cited` record keeps the kind the agent wrote — §6.2 lets those
// two assert and does not require them to, and an agent that chose to ask is
// making the judgement §2.1.3 gives it. A forcing that also turned questions
// into findings would be cr forming the opinion P5 denies it.
func TestTheForcingLeavesEveryGradeButArguedAlone(t *testing.T) {
	for _, grade := range []Grade{GradeProbed, GradeCited} {
		for _, kind := range []Kind{KindFinding, KindQuestion} {
			record := argued()
			record.Grade, record.Kind = grade, kind

			assert.False(t, ForceQuestion(record),
				"§6.3.1 forces the argued grade and no other")
			assert.Equal(t, kind, record.Kind, "%s %s is left as the agent wrote it", grade, kind)
		}
	}
}

// The forcing follows a grade the ratchet lowered, which is what makes the
// second and third moments necessary rather than defensive.
//
// The record is recorded on the strength of an experiment, graded `probed`, and
// left as a finding. The probe then stops standing behind it — a re-run at a
// moved head, a baseline that no longer resolves, a mapping §4.1.6 replaced —
// and Regrade lowers it to `argued`. Nothing about the record's own text
// changed, and it is now an assertion with nothing behind it; the next moment
// of §6.3.1 is what catches that.
func TestAGradeTheRatchetLoweredIsForcedAtTheNextMoment(t *testing.T) {
	proven, passing := provenProbe(t, gradedHead)
	record := aGradedRecord()
	record.Kind, record.Probe = KindFinding, proven.ID

	Regrade(record, Resolved(theUnit, proven, gradedHead, passing, theMapping()))
	require.Equal(t, GradeProbed, record.Grade)
	require.False(t, ForceQuestion(record), "an experiment stands behind it, so it may assert")
	require.Equal(t, KindFinding, record.Kind)

	Regrade(record, Resolved(theUnit, nil, gradedHead, probe.Baseline{}, theMapping()))
	require.Equal(t, GradeArgued, record.Grade, "the ratchet lowers a grade that stopped standing up")

	assert.True(t, ForceQuestion(record), "and the next moment of §6.3.1 forces it")
	assert.Equal(t, KindQuestion, record.Kind)
}
