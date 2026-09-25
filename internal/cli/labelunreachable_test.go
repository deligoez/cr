package cli

import (
	"os"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/deligoez/cr/internal/config"
	"github.com/deligoez/cr/internal/finding"
	"github.com/deligoez/cr/internal/render"
	"github.com/deligoez/cr/internal/state"
)

// recordedQuestion runs `cr record` over the one argued record the grade
// fixture holds, which §6.3 will force into the question register at draft.
//
// It is recorded before any layer is spoiled, so a refusal the test then sees
// is `cr draft`'s own and not a record-time one.
func recordedQuestion(t *testing.T) state.Layout {
	t.Helper()
	layout := gradedHome(t)
	_, err := runRecord(t, "7", writeRecordFile(t, "merged.ndjson", aGradedRecord("f1")), "--repo", fixtureSlug)
	require.NoError(t, err)
	return layout
}

// draftedQuestion runs `cr draft` over it and returns the draft.
func draftedQuestion(t *testing.T, layout state.Layout, flags ...string) string {
	t.Helper()
	_, err := runDraft(t, append([]string{"7", "--repo", fixtureSlug}, flags...)...)
	require.NoError(t, err)
	body, err := os.ReadFile(
		layout.RoundFile(fixtureOwner, fixtureProject, fixturePRNumber, 2, state.FileDraft))
	require.NoError(t, err)
	return string(body)
}

// labelFor is the region the built-in table gives an argued question in lang.
func labelFor(t *testing.T, lang render.Lang) string {
	t.Helper()
	label, err := render.QuestionLabelRegion(lang, finding.GradeArgued)
	require.NoError(t, err)
	return label
}

// refusedDraft runs `cr draft` expecting §2.7's refusal of a name addressing
// the label, and asserts that nothing was drafted.
func refusedDraft(t *testing.T, layout state.Layout) {
	t.Helper()
	_, err := runDraft(t, "7", "--repo", fixtureSlug)
	require.Error(t, err)

	var protected *config.ProtectedError
	require.ErrorAs(t, err, &protected)
	assert.Contains(t, protected.Subject, "§8.1.4")
	assert.Equal(t, ExitFile, exitCodeFor(err))

	// `cr record` already wrote its §10.3 counts, and a round summary is
	// written into a round directory §2.3 creates whole, empty draft.md
	// included — so what a refused layer leaves is an empty draft, and a
	// rendering would have given it a header at the very least.
	body, readErr := os.ReadFile(
		layout.RoundFile(fixtureOwner, fixtureProject, fixturePRNumber, 2, state.FileDraft))
	require.NoError(t, readErr)
	assert.Empty(t, string(body), "a refused layer drafts nothing")
}

// §8.1.4 and §2.7: the question label is built in per `render.lang`, it is not
// a setting, and it is readable from none of §2.7's five layers — yet it
// appears on every question.
//
// Each layer gets the attempt someone reaching for the label would make. The
// command line has no flag naming it and drafting under every global flag
// leaves the line as built; the environment, the per-repository config and the
// global config each carry a name addressing it, and `cr draft` refuses before
// rendering; and the built-in defaults hold no key for it at all. The one thing
// a layer may say is `render.lang`, and it chooses a row of the table rather
// than supplying text — the English row by default, the Turkish one when asked.
func TestTheQuestionLabelIsUnreachableFromEveryConfigurationLayer(t *testing.T) {
	t.Run("command-line flags", func(t *testing.T) {
		for _, name := range everyFlagInTheTree(newRootCmd()) {
			assert.NotContains(t, strings.ToLower(name), "label",
				"§2.7: --%s would address the question label", name)
		}
		for _, flags := range [][]string{{}, {"--quiet"}, {"--json"}, {"--compact"}, {"--no-color"}} {
			layout := recordedQuestion(t)
			drafted := draftedQuestion(t, layout, flags...)
			assert.Contains(t, drafted, `id="f1" kind="question"`)
			assert.Contains(t, drafted, labelFor(t, render.LangEN),
				"cr draft %s carries the built-in label", flagsNamed(flags))
		}
	})

	t.Run("environment variables", func(t *testing.T) {
		for _, name := range []string{"CR_RENDER_LABEL", "CR_QUESTION_LABEL", "CR_POST_LABELS"} {
			t.Run(name, func(t *testing.T) {
				layout := recordedQuestion(t)
				t.Setenv(name, "Not a question")
				refusedDraft(t, layout)
			})
		}
	})

	t.Run("per-repository config", func(t *testing.T) {
		layout := recordedQuestion(t)
		require.NoError(t, os.WriteFile(layout.RepoConfig(fixtureOwner, fixtureProject),
			[]byte(`{"render":{"label":"Not a question"}}`), 0o600))
		refusedDraft(t, layout)
	})

	t.Run("global config", func(t *testing.T) {
		layout := recordedQuestion(t)
		require.NoError(t, os.WriteFile(layout.Config(),
			[]byte(`{"render":{"question_label":{"tr":"Not a question"}}}`), 0o600))
		refusedDraft(t, layout)
	})

	t.Run("built-in defaults", func(t *testing.T) {
		defaults, err := config.Resolve(config.Sources{})
		require.NoError(t, err)
		keys := defaults.Map()
		require.NotEmpty(t, keys)
		for key := range keys {
			assert.NotContains(t, key, "label", "§2.7: the label is not a setting, so no default holds it")
		}
	})

	t.Run("render.lang chooses the row and never the text", func(t *testing.T) {
		layout := recordedQuestion(t)
		assert.Contains(t, draftedQuestion(t, layout), labelFor(t, render.LangEN),
			"§8.1.1: en is the default, and its row is the one drafted")

		require.NoError(t, os.WriteFile(layout.RepoConfig(fixtureOwner, fixtureProject),
			[]byte(`{"render":{"lang":"tr"}}`), 0o600))
		drafted := draftedQuestion(t, layout)
		assert.Contains(t, drafted, labelFor(t, render.LangTR))
		assert.NotContains(t, drafted, labelFor(t, render.LangEN),
			"one question, one label, in the language the layer chose")
	})
}
