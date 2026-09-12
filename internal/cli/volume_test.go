package cli

import (
	"io/fs"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/deligoez/cr/internal/finding"
)

// §7.3.6: a class discarded as `not-here` far more often than it is kept is a
// volume candidate and not a demotion candidate.
//
// The two classes are the same history told twice, differing only in the
// disposition of the discards, because that is the whole of what separates the
// two lists. `unwelcome` is accurate — nothing was ever marked `wrong` — and
// merely not worth posting, so §7.3.6 puts it on its own list with its own
// remedy: stop raising it. `imprecise` is the other case, and each class
// appears on exactly one list.
//
// A `not-here` rate folded into §7.3.4's numerator would put `unwelcome` up for
// demotion, and the remedy would then be to soften it — turning a true finding
// nobody wanted into a question nobody wanted, on a class that has never once
// been wrong.
func TestAnAccurateButUnwantedClassIsAVolumeCandidateAndNotADemotionOne(t *testing.T) {
	layout := statsHome(t)

	notHere := append(
		repeated(7, finding.OutcomeDiscardedNotHere), repeated(3, finding.OutcomeKept)...)
	seedTriage(t, layout, statsFirst, "unwelcome", notHere)
	wrong := append(repeated(7, finding.OutcomeDiscardedWrong), repeated(3, finding.OutcomeKept)...)
	seedTriage(t, layout, statsLater, "imprecise", wrong)

	report := statsReport(t)

	assert.Equal(t, []finding.VolumeCandidate{
		{
			Class: "unwelcome", Raised: 10, NotHereRate: 0.7,
			Remedy: finding.VolumeRemedy,
		},
	}, report.Volume, "§7.3.6: accurate, rarely worth posting, reported separately")
	assert.Equal(t, []finding.DemotionCandidate{
		{Class: "imprecise", Raised: 10, RateLowerBound: 0.7},
	}, report.Demotion, "and the imprecise class is on the other list and not on this one")
}

// §7.3.6 draws its candidacy over the same sample §7.3.4 does, so a class under
// the minimum is on neither list however lopsided its dispositions.
//
// Reading "the same sample" as the same two settings rather than as a second
// number arrived at separately is what keeps a project that moves either knob
// from having cr change its mind about one list and not the other.
func TestAVolumeCandidacyIsDrawnOverTheSameSampleAsADemotion(t *testing.T) {
	layout := statsHome(t)
	seedTriage(t, layout, statsFirst, "rare", repeated(7, finding.OutcomeDiscardedNotHere))

	report := statsReport(t)
	assert.Empty(t, report.Volume, "7 raises is under §7.3.4's default minimum of 8")
	assert.Empty(t, report.Demotion)

	byClass := map[string]finding.ClassTriage{}
	for _, class := range report.Classes {
		byClass[class.Class] = class
	}
	assert.InDelta(t, 1.0, byClass["rare"].NotHereRate(), 0.0001,
		"the rate is computed and simply not acted on")
}

// §7.3.7: both candidacies are reports to the user and not automatic changes,
// and cr alters no rule's kind by itself.
//
// The state tree is fingerprinted whole, byte for byte, across a run that
// produced both a demotion candidate and a volume candidate — the run that
// would act on them if any run did. A rule file is planted first, holding the
// `kind` §7.3.7 names, so the assertion has the thing it is about to protect
// and not only the absence of one: `cr stats` has every reason it will ever
// have to rewrite that file, and the file comes out unchanged along with
// everything else under `~/.cr`.
//
// The whole tree rather than the rule file alone is deliberate. §7.3.7 forbids
// the automatic change, and a command that demoted nothing while quietly
// rewriting the ledger it had just read would satisfy a narrower guard and
// break the same sentence.
func TestStatsChangesNothingIncludingTheRuleItProposesDemoting(t *testing.T) {
	layout := statsHome(t)
	notHere := append(
		repeated(7, finding.OutcomeDiscardedNotHere), repeated(3, finding.OutcomeKept)...)
	seedTriage(t, layout, statsFirst, "unwelcome", notHere)
	wrong := append(repeated(7, finding.OutcomeDiscardedWrong), repeated(3, finding.OutcomeKept)...)
	seedTriage(t, layout, statsLater, "imprecise", wrong)

	planted := filepath.Join(layout.RulesDir(), "no-dropped-error.json")
	require.NoError(t, os.WriteFile(planted, []byte(
		`{"id":"no-dropped-error","class":"imprecise","kind":"finding",`+
			`"summary":"The error is dropped.","pattern":"_ ="}`+"\n"), 0o600))

	before := treeFingerprint(t, layout.Root())

	report := statsReport(t)
	require.Len(t, report.Demotion, 1, "the run had a demotion candidate to act on")
	require.Len(t, report.Volume, 1, "and a volume candidate")

	assert.Equal(t, before, treeFingerprint(t, layout.Root()),
		"§7.3.7: both candidacies are reports, so the run changed nothing at all")
}

// treeFingerprint is every file under root with its contents, so a comparison
// says which file changed rather than only that one did.
func treeFingerprint(t *testing.T, root string) map[string]string {
	t.Helper()
	seen := map[string]string{}
	require.NoError(t, filepath.WalkDir(root, func(path string, entry fs.DirEntry, err error) error {
		if err != nil || entry.IsDir() {
			return err
		}
		body, readErr := os.ReadFile(path)
		if readErr != nil {
			return readErr
		}
		rel, relErr := filepath.Rel(root, path)
		if relErr != nil {
			return relErr
		}
		seen[filepath.ToSlash(rel)] = string(body)
		return nil
	}))
	require.NotEmpty(t, seen, "an empty state tree cannot show that a run left it alone")
	return seen
}
