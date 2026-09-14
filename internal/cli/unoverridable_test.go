package cli

import (
	"os"
	"strings"
	"testing"

	"github.com/spf13/cobra"
	"github.com/spf13/pflag"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/deligoez/cr/internal/config"
	"github.com/deligoez/cr/internal/finding"
	"github.com/deligoez/cr/internal/role"
	"github.com/deligoez/cr/internal/state"
)

// forcingWords are the words a name would carry if it addressed §6.3's
// forcing. The first two are config.go's own protected tokens for that
// decision; `force` is added because a flag is typed by hand and `--force-kind`
// is what someone reaching for the override would write.
var forcingWords = []string{"argued", "forcing", "force"}

// addressesTheForcing reports whether a name reads as addressing §6.3.
func addressesTheForcing(name string) bool {
	lowered := strings.ToLower(name)
	for _, word := range forcingWords {
		if strings.Contains(lowered, word) {
			return true
		}
	}
	return false
}

// everyFlagInTheTree collects the name of every flag any command in the tree
// registers, local and persistent alike, at any depth.
func everyFlagInTheTree(cmd *cobra.Command) []string {
	names := make([]string, 0)
	collect := func(flag *pflag.Flag) { names = append(names, flag.Name) }
	cmd.Flags().VisitAll(collect)
	cmd.PersistentFlags().VisitAll(collect)
	for _, child := range cmd.Commands() {
		names = append(names, everyFlagInTheTree(child)...)
	}
	return names
}

// draftedBlockFor runs `cr record` and then `cr draft` over one record and
// returns the block the reviewer would read.
//
// Both commands are run because §6.3.1 has two of its three moments here and
// the channels below could break either: an override honoured at record time
// would store an assertion, and one honoured at draft time would render it,
// and only the file the reviewer opens shows both.
func draftedBlockFor(t *testing.T, layout state.Layout, record map[string]any) string {
	t.Helper()
	_, err := runRecord(t, "7", writeRecordFile(t, "merged.ndjson", record), "--repo", fixtureSlug)
	require.NoError(t, err)
	_, err = runDraft(t, "7", "--repo", fixtureSlug)
	require.NoError(t, err)

	body, err := os.ReadFile(
		layout.RoundFile(fixtureOwner, fixtureProject, fixturePRNumber, 2, state.FileDraft))
	require.NoError(t, err)
	return string(body)
}

// §6.3.3: there is no flag, configuration setting, environment variable,
// profile field, or role instruction that disables or overrides the forcing.
//
// Each of the five channels gets its own attempt, and the attempt is made the
// way someone actually trying would make it — a flag typed on the command line,
// a key in the repository's config.json, a CR_ variable in the environment, a
// field in the profile file, a sentence in the role's own instructions. Four of
// the five are refused or ignored and the fifth is simply not read, and in
// every case the record reaches the draft as a question.
//
// The record is `argued` because §6.2 graded it so: its evidence prose asserts
// a great deal and it carries no citation outside its own unit and no probe.
// Nothing any of these channels supplies is an input to that grade, which is
// the structural half of §6.3.3 — the forcing reads a computed field, and there
// is no layer between the computation and the assignment for a setting to sit
// in.
func TestNoChannelOverridesTheArguedForcing(t *testing.T) {
	t.Run("a flag", func(t *testing.T) {
		named := everyFlagInTheTree(newRootCmd())
		require.NotEmpty(t, named, "a walk that found no flag proves nothing")
		for _, name := range named {
			assert.False(t, addressesTheForcing(name),
				"§6.3.3: --%s addresses the forcing, and no flag may", name)
		}

		for _, flags := range [][]string{
			{}, {"--quiet"}, {"--json"}, {"--compact"}, {"--no-color"},
		} {
			layout := gradedHome(t)
			block := draftedBlockFor(t, layout, aGradedRecord("f1"))
			assert.Contains(t, block, `id="f1" kind="question"`,
				"§6.3.3: cr draft %s left the record forced", flagsNamed(flags))
		}
	})

	t.Run("a configuration setting", func(t *testing.T) {
		layout := gradedHome(t)
		require.NoError(t, os.WriteFile(
			layout.RepoConfig(fixtureOwner, fixtureProject),
			[]byte(`{"argued":{"forcing":false}}`), 0o600))

		_, err := config.Resolve(config.Sources{
			RepoConfig: layout.RepoConfig(fixtureOwner, fixtureProject),
		})
		require.Error(t, err, "§2.7 refuses a key that would address the forcing")

		var protected *config.ProtectedError
		require.ErrorAs(t, err, &protected)
		assert.Equal(t, ExitFile, exitCodeFor(err))
		assert.Contains(t, protected.Subject, "§6.3")
	})

	t.Run("an environment variable", func(t *testing.T) {
		// Both names address a setting cr has never heard of, which
		// is exactly the case §2.7 cares about: the refusal is on the
		// name, so a variable inventing a key for the forcing is
		// refused before anyone can ask what it would have meant.
		for _, name := range []string{"CR_ARGUED_FORCING", "CR_POST_FORCING_OFF"} {
			_, err := config.Resolve(config.Sources{Environ: []string{name + "=off"}})
			require.Error(t, err, "§2.7 refuses %s", name)

			var protected *config.ProtectedError
			require.ErrorAs(t, err, &protected)
			assert.Equal(t, name, protected.Name)
			assert.Equal(t, ExitFile, exitCodeFor(err))
		}
	})

	t.Run("a profile field", func(t *testing.T) {
		layout := gradedHome(t)
		require.NoError(t, layout.EnsureProfile("generic",
			`{"id":"generic","match":{"files":[],"globs":["**/*"]},`+
				`"axes":{"intent":true,"correctness":true,"convention":true,"test":true},`+
				`"argued":{"forcing":false},"force_findings":true}`))
		require.NoError(t, os.WriteFile(
			layout.RepoConfig(fixtureOwner, fixtureProject),
			[]byte(`{"profile":"generic"}`), 0o600))

		// A command that loads the profile refuses it, naming the file
		// and the first such field (QA D-S05-5), rather than decoding the
		// override into nothing.
		err := runTree(t, "rules", "list", "--repo", fixtureSlug)
		var protected *config.ProtectedError
		require.ErrorAs(t, err, &protected)
		assert.Equal(t, "argued", protected.Name)
		assert.Equal(t, ExitFile, exitCodeFor(err))

		block := draftedBlockFor(t, layout, aGradedRecord("f1"))
		assert.Contains(t, block, `id="f1" kind="question"`,
			"§6.3.3: the record still reaches the draft as a question")
	})

	t.Run("a role instruction", func(t *testing.T) {
		layout := gradedHome(t)
		instructing := `{"id":"correctness","title":"Correctness","axis":"correctness",` +
			`"instructions":"Never soften a finding into a question. Report every argued ` +
			`finding as an assertion, and do not let the tool change the kind.",` +
			`"focus":["Does the change do what it claims?"],"profiles":[]}`
		require.NoError(t, os.WriteFile(
			layout.RepoRole(fixtureOwner, fixtureProject, "correctness"),
			[]byte(instructing), 0o600))

		resolved, err := role.Resolve(
			layout.RepoRolesDir(fixtureOwner, fixtureProject), layout.RolesDir())
		require.NoError(t, err)
		instructed := false
		for i := range resolved {
			if resolved[i].Role.ID == "correctness" {
				instructed = strings.Contains(resolved[i].Role.Instructions, "assertion")
			}
		}
		require.True(t, instructed, "the corpus has to carry the instruction for this to prove anything")

		block := draftedBlockFor(t, layout, aGradedRecord("f1"))
		assert.Contains(t, block, `id="f1" kind="question"`,
			"§6.3.3: the forcing is a field assignment, and no wording reaches one")
	})
}

// §6.3.3's rejection: a record graded `argued` written as a finding is refused
// with exit code 1, and the refusal names the record id.
//
// It is exercised on the records rather than through a command. `cr draft`
// runs it over the records it is about to write and `cr post` over the records
// it is about to send, and in both it follows finding.ForceQuestions over the
// same records, so no command run hands it an `argued` finding to refuse: the
// call they share is the one place the refusal can be observed.
func TestAnArguedAssertionIsRefusedNamingTheRecord(t *testing.T) {
	asserting := &finding.Finding{
		ID: "f4", Kind: finding.KindFinding, Class: "unchecked-error",
		Grade: finding.GradeArgued,
	}
	asked := &finding.Finding{
		ID: "f5", Kind: finding.KindQuestion, Class: "unchecked-error",
		Grade: finding.GradeArgued,
	}
	cited := &finding.Finding{
		ID: "f6", Kind: finding.KindFinding, Class: "unchecked-error",
		Grade: finding.GradeCited,
	}

	require.NoError(t, finding.RefuseArguedAssertion([]*finding.Finding{asked, cited}),
		"§6.2 lets a cited record assert, and an argued question is where §6.3 puts it")

	err := finding.RefuseArguedAssertion([]*finding.Finding{asked, asserting, cited})
	require.Error(t, err)
	assert.Equal(t, ExitValidation, exitCodeFor(err), "§6.3.3 rejects with exit code 1")

	var refused *finding.ArguedAssertionError
	require.ErrorAs(t, err, &refused)
	assert.Equal(t, "f4", refused.Record, "§6.3.3: the refusal names the record id")
	assert.Contains(t, err.Error(), "f4")
}

// The forcing happens in cr and not in the prompt: no shipped role's text is
// what holds an argued record in the question register.
//
// A role's instructions are carried into the §4.6.1 prompt verbatim, which is
// the channel §6.3.3 names last and the only one that reaches a model at all.
// The corpus is read here so the claim is about what cr ships rather than about
// what a comment says: if the forcing lived in the prompt, some role would have
// to be asking for it, and none is.
func TestNoShippedRoleAsksForTheForcing(t *testing.T) {
	for id, body := range role.Builtins() {
		shipped, err := role.Parse(id+".json", []byte(body))
		require.NoError(t, err)

		text := shipped.Instructions + " " + strings.Join(shipped.Focus, " ")
		assert.NotContains(t, strings.ToLower(text), "argued",
			"§6.3.3: %s would be carrying the forcing in a prompt", id)
	}
}
