package cli

import (
	"os"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/deligoez/cr/internal/draft"
	"github.com/deligoez/cr/internal/finding"
	"github.com/deligoez/cr/internal/state"
)

// The three tests below close mutants that survived the mutation run of
// 2026-09-12. Each names the boundary it pins, because a boundary test whose
// reason is "a mutant lived here" is one nobody can judge later.

// `cr draft` says what it kept only when it kept something.
//
// The line is built from `len(r.Preserved) > 0`, and a run that printed it
// unconditionally would tell a reviewer, after every draft, that it had kept
// the edited body of nothing at all — a sentence that is false and that the
// reviewer would learn to skip, taking the true one with it. No fixture reached
// the empty case through the terminal rendering.
func TestATerminalDraftThatKeptNothingSaysNothingAboutKeeping(t *testing.T) {
	draftedHome(t, aStoredRecord("f1", finding.StateDraft))

	out := throughATerminal(t, "draft", draftPR, "--repo", draftSlug)

	assert.NotContains(t, out, "kept the edited body of")
}

// The triage's two report helpers size their buffers from the lists they hold,
// and the sizes have to survive a round whose lists are uneven.
//
// `make([]T, 0, len(a)+len(b))` is a capacity hint until the arithmetic turns
// negative, and then it panics `makeslice: cap out of range` — the failure
// mode scripts/known-survivors.json warns about at the top. A round that
// deletes nothing and calls two records wrong is an ordinary round, and it is
// the one that separates a hint from a crash.
func TestTheTriageReportsARoundThatDeletedNothing(t *testing.T) {
	wrong := []*finding.Finding{
		{ID: "f1", Kind: finding.KindFinding},
		{ID: "f2", Kind: finding.KindFinding},
	}
	softened := []*finding.Finding{
		{ID: "f3", Kind: finding.KindFinding},
		{ID: "f4", Kind: finding.KindFinding},
	}
	for _, c := range []struct {
		name     string
		triage   draft.Triage
		outcomes map[string]finding.Outcome
	}{
		{
			name:   "two wrong and one softened, none deleted",
			triage: draft.Triage{Wrong: wrong, Softened: softened[:1]},
			outcomes: map[string]finding.Outcome{
				"f1": finding.OutcomeDiscardedWrong,
				"f2": finding.OutcomeDiscardedWrong,
				"f3": finding.OutcomeSoftened,
			},
		},
		{
			name:   "two softened, none deleted and none wrong",
			triage: draft.Triage{Softened: softened},
			outcomes: map[string]finding.Outcome{
				"f3": finding.OutcomeSoftened,
				"f4": finding.OutcomeSoftened,
			},
		},
	} {
		t.Run(c.name, func(t *testing.T) {
			read := append(append([]*finding.Finding{}, c.triage.Wrong...), c.triage.Softened...)
			triage := triaged{Triage: c.triage, read: read}

			discarded := triage.discarded()
			report := triage.report()

			assert.ElementsMatch(t, c.triage.Wrong, discarded,
				"a round with no deletion discards its wrong records and no others")
			require.Len(t, report, len(c.outcomes))
			for _, entry := range report {
				assert.Equal(t, c.outcomes[entry.ID], entry.Outcome, entry.ID)
				assert.True(t, entry.CountsAgainstClass,
					"§7.3.4 counts both a wrong and a softening against the class")
			}
		})
	}
}

// A round whose every record is graded `probed` still reads its probes.
//
// The draft loads probes.ndjson when some queued record is graded `probed`, and
// a run that asked the opposite question would load them for every round but
// this one — the round where §8.1.7's evidence region has the most to say. It
// fails loudly rather than quietly, with §11.2's code 3 for a probe the state
// tree does not hold, which is what makes the case worth a fixture: the two
// readings agree on every round that mixes grades, and the fixtures all did.
func TestADraftOfNothingButProbedRecordsStillReadsTheProbes(t *testing.T) {
	layout := gradedHome(t)
	probed := aGradedRecord("f1")
	probed["probe"], probed["severity"] = "p1", "high"
	_, err := runRecord(t, fixturePR,
		writeRecordFile(t, "merged.ndjson", probed), "--repo", fixtureSlug)
	require.NoError(t, err)

	_, err = runDraft(t, fixturePR, "--repo", fixtureSlug)
	require.NoError(t, err)

	written, err := os.ReadFile(
		layout.RoundFile(fixtureOwner, fixtureProject, fixturePRNumber, 2, state.FileDraft))
	require.NoError(t, err)
	assert.Contains(t, blockOf(t, string(written), "f1"), "kind: mutation")
}
