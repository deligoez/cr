package cli

import (
	"bytes"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/spf13/cobra"
	"github.com/spf13/pflag"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// surfaceRow is one row of §11's command table, read as a contract: the command
// as it is typed, the argument shape it advertises, and the flags §11 names for
// it.
type surfaceRow struct {
	// path is the command as it is typed after `cr`.
	path []string
	// use is the Use line, which is where the argument shape lives. It is
	// pinned rather than derived so a positional silently dropped from a
	// command fails here — §11 gives the shape, and a command that stopped
	// naming `<file>` would still resolve, still parse, and answer a
	// different contract.
	use string
	// spec are the long flag names §11's own row names, resolved through
	// the command or inherited from the root. §11.1's `--repo` is a global
	// flag, so a row naming it is asserting that it reaches this command
	// and not that this command declares it.
	spec []string
	// shorthands are the single letters §11 spells, mapped to the long name
	// each must resolve to. §11 writes `-o <out>` on `cr merge` and nothing
	// else, so this is the one row that carries any.
	shorthands map[string]string
	// added are local flags the command registers beyond §11's row, each
	// mapped to the section that requires it. §11 lists flags on some rows
	// and omits them on others, and an omission there is not a prohibition:
	// where another section fixes a flag, the surface follows that section
	// and the addition is recorded here rather than passing unremarked.
	added map[string]string
}

// specSurface is §11's table, one entry per command that can actually be typed.
//
// §11's rows are not quite commands: `cr init` and `cr init --eject-roles` are
// one command with a flag, as are `cr note` and `cr note --remove`, while
// `cr sandbox create|destroy`, `cr waivers list|remove` and
// `cr rules list|check|suggest` are each several. What is listed here is the
// set a user can invoke, because that is the set the tree can be compared
// against in both directions.
var specSurface = []surfaceRow{
	{path: []string{"init"}, use: "init", spec: []string{"eject-roles"}},
	{path: []string{"config"}, use: "config", spec: []string{"resolved"}},
	{path: []string{"brief"}, use: "brief <pr>", spec: []string{"intent-extra"},
		added: map[string]string{
			// §3.1.4 lets a file stand in for the tracker command, and
			// CLAUDE.md's own QA recipe runs `cr brief` that way. §11 names
			// flags on other rows and omits these two, so following §11
			// here would leave the bypass unreachable on the one command
			// that opens a round.
			"intent-file": "§3.1.4",
			"issue":       "§3.2",
		}},
	{path: []string{"review"}, use: "review <pr>", spec: []string{"axis", "units", "shard", "all"}},
	{path: []string{"claims", "record"}, use: "record <pr> <file>",
		spec: []string{"intent-extra"},
		added: map[string]string{
			// §3.3 re-reads the issue text to check every claim's span, so
			// the bypass §3.1.4 gives `cr brief` is needed here too and is
			// not inherited from the brief that opened the round.
			"intent-file": "§3.1.4",
		}},
	{path: []string{"claims", "set-aside"}, use: "set-aside <pr> <claim-id>",
		spec: []string{"note"}},
	{path: []string{"merge"}, use: "merge <files...>",
		spec:       []string{"output", "pr", "repo"},
		shorthands: map[string]string{"o": "output"}},
	{path: []string{"record"}, use: "record <pr> <file>"},
	{path: []string{"map", "record"}, use: "record <pr> <file>"},
	{path: []string{"cells", "record"}, use: "record <pr> <file>"},
	{path: []string{"sandbox", "create"}, use: "create <pr>"},
	{path: []string{"sandbox", "destroy"}, use: "destroy <pr>"},
	{path: []string{"test"}, use: "test <pr>", spec: []string{"filter", "path"}},
	{path: []string{"probe", "run"}, use: "run <pr>", spec: []string{"kind"}, added: map[string]string{
		// §11's row ends in `...`, and these are what it stands for:
		// §5.3.1's patch, §5.4.2's test file and §5.5's target, plus
		// the filter and the paths §5.2.2 scopes a run by.
		"patch":  "§5.3.1",
		"test":   "§5.4.2",
		"target": "§5.5",
		"filter": "§5.2.2",
		"path":   "§5.2.2",
		// §5.7.3's own flag: the proposal supplies every input above,
		// and giving one beside it is refused with code 2.
		"proposal": "§5.7.3",
	}},
	{path: []string{"proposals", "record"}, use: "record <pr> <file>"},
	{path: []string{"draft"}, use: "draft <pr>"},
	{path: []string{"triage"}, use: "triage <pr> <record-id> not-here|wrong|soften|keep",
		spec: []string{"body-file"}},
	{path: []string{"post"}, use: "post <pr>", spec: []string{"confirm", "reconcile"}},
	{path: []string{"answer"}, use: "answer <pr> <record-id> <text>", added: map[string]string{
		// §3.6.1 requires a source on every entry in the context store,
		// and §3.6.2 stores an answer as one of them.
		"source": "§3.6.1",
	}},
	{path: []string{"note"}, use: "note <ISSUE-KEY> <text>", spec: []string{"remove"},
		added: map[string]string{"source": "§3.6.1", "pr": "§3.6.1"}},
	{path: []string{"context"}, use: "context <ISSUE-KEY>"},
	{path: []string{"waivers", "list"}, use: "list", spec: []string{"repo", "pr"}},
	{path: []string{"waivers", "remove"}, use: "remove <id>", spec: []string{"repo", "pr"}},
	{path: []string{"stats"}, use: "stats", spec: []string{"repo"}},
	{path: []string{"rules", "list"}, use: "list", spec: []string{"dead", "repo"}},
	{path: []string{"rules", "check"}, use: "check <pr>"},
	{path: []string{"rules", "suggest"}, use: "suggest", spec: []string{"repo"}},
	{path: []string{"status"}, use: "status <pr>"},
}

// localFlagNames is the set of flags a command registers itself, which is what
// makes "§11's flags and these declared additions, and nothing else" an
// assertion rather than a lower bound.
//
// LocalFlags is asked rather than Flags, because a global flag reaches every
// command in the tree and would otherwise show up as an undeclared extra on all
// twenty-six of them.
func localFlagNames(cmd *cobra.Command) []string {
	names := make([]string, 0)
	cmd.LocalFlags().VisitAll(func(flag *pflag.Flag) {
		names = append(names, flag.Name)
	})
	return names
}

// §11's table is the command surface, in both directions: every row resolves
// with the argument shape and flags it names, and every command in the tree is
// a row.
//
// The second direction is what keeps the first honest. A table checked only
// forwards passes while the binary carries a command §11 never described, and
// the surface is a contract a caller writes scripts against — an undocumented
// command is as much a defect in it as a missing one.
func TestTheCommandSurfaceIsTheSpecTable(t *testing.T) {
	root := newRootCmd()

	for _, row := range specSurface {
		name := strings.Join(row.path, " ")
		t.Run(name, func(t *testing.T) {
			found, _, err := root.Find(row.path)
			require.NoError(t, err, "§11 names `cr %s` and the tree does not resolve it", name)
			require.Equal(t, "cr "+name, found.CommandPath(),
				"`cr %s` resolved to a different command", name)
			assert.Equal(t, row.use, found.Use,
				"§11 fixes the argument shape of `cr %s`", name)

			local := make([]string, 0, len(row.spec)+len(row.added))
			for _, flag := range row.spec {
				assert.NotNilf(t, found.Flag(flag),
					"§11 names --%s on `cr %s` and it does not resolve", flag, name)
				// §11.1's global flags are registered once, on the
				// root, so a row naming one is not a row declaring it.
				if flag != "repo" {
					local = append(local, flag)
				}
			}
			for flag, section := range row.added {
				assert.NotNilf(t, found.Flag(flag),
					"%s requires --%s on `cr %s` and it does not resolve", section, flag, name)
				local = append(local, flag)
			}
			for short, long := range row.shorthands {
				resolved := found.Flags().ShorthandLookup(short)
				require.NotNilf(t, resolved, "§11 spells -%s on `cr %s`", short, name)
				assert.Equal(t, long, resolved.Name)
			}
			assert.ElementsMatch(t, local, localFlagNames(found),
				"`cr %s` registers a flag neither §11 nor this table's additions account for", name)
		})
	}

	typed := make([]string, 0, len(specSurface))
	for _, row := range specSurface {
		typed = append(typed, strings.Join(row.path, " "))
	}
	assert.ElementsMatch(t, typed, leafCommands(t),
		"§11 is the whole command surface, so a command in the tree is a row and a row is in the tree")
}

// `cr config --resolved` changes what the command prints.
//
// It was the one place in the surface where a command that works carried a flag
// that did not, and it refused rather than being ignored — a silently ignored
// flag there is the worst of the three outcomes, since the caller asks which
// layer each setting came from, gets a plain listing, and has no way to tell it
// apart from an annotated one where every setting came from the same layer.
// config-resolved-annotation built it, and what is left to hold is that the
// flag is still not the third thing: a listing that differs from the plain one.
func TestTheResolvedFlagChangesWhatConfigPrints(t *testing.T) {
	root := crHome(t)
	require.NoError(t, os.WriteFile(filepath.Join(root, "config.json"),
		[]byte(`{"post": {"max_comments": 9}}`), 0o600))

	plain := configPrinting(t)
	annotated := configPrinting(t, "--resolved")

	require.NotEqual(t, plain, annotated, "§2.7: the annotated listing is not the plain one")
	assert.Contains(t, annotated, filepath.Join(root, "config.json"),
		"and it names the file the value came from")
}

// configPrinting runs `cr config` against whatever CR_HOME points at and
// returns the document it printed.
func configPrinting(t *testing.T, args ...string) string {
	t.Helper()
	cmd := newRootCmd()
	var out bytes.Buffer
	cmd.SetOut(&out)
	cmd.SetErr(&bytes.Buffer{})
	cmd.SetArgs(append([]string{"config"}, args...))
	require.NoError(t, cmd.Execute())
	return out.String()
}

// Every command §11 marks as taking a pull request rejects an argument that is
// not one, so a stub answers a mistyped command line differently from a correct
// one.
//
// This is what the argument shape buys before the behaviour exists. A stub that
// accepted anything would give the same message to `cr status 42` and to
// `cr status HEAD`, and the second is a usage error §11.2 codes 2 whether or
// not the command is built.
//
// It is asked of every row with a positional, built or not. It began as a
// guard over stubs alone, and `cr waivers remove` was the last stub taking a
// positional: building it would have left the guard measuring nothing, while
// the property it states — §11's arity, refused below it — is one a built
// command owes exactly as a stub does.
func TestAStubStillValidatesItsArguments(t *testing.T) {
	root := newRootCmd()

	checked := 0
	for _, row := range specSurface {
		// The shape is counted off §11's own row, so a command that takes
		// no positional argument has nothing here to get wrong.
		// A word of alternatives, such as §7.2.4's verb, is one
		// positional too.
		positional := strings.Count(row.use, "<")
		for word := range strings.FieldsSeq(row.use) {
			if strings.Contains(word, "|") {
				positional++
			}
		}
		if positional == 0 {
			continue
		}
		checked++
		name := strings.Join(row.path, " ")
		t.Run(name, func(t *testing.T) {
			found, _, err := root.Find(row.path)
			require.NoError(t, err)
			require.NotNil(t, found.Args, "`cr %s` validates no arguments", name)
			assert.NoError(t, found.Args(found, slices.Repeat([]string{"42"}, positional)))
			assert.Error(t, found.Args(found, slices.Repeat([]string{"42"}, positional-1)),
				"`cr %s` accepted fewer arguments than §11 gives it", name)
			// The pull-request check is the sharper half and is asked
			// of the rows that have one. `cr claims set-aside` was the
			// last stub §11 gives a pull request, and building it left
			// no row to ask — so the arity above is what keeps the
			// guard measuring, and this returns with the next such
			// stub rather than being deleted with the last one.
			if strings.Contains(row.use, prPlaceholder) {
				assert.Error(t, found.Args(found, []string{"not-a-pull-request"}),
					"`cr %s` accepted an argument that is not a pull request", name)
			}
		})
	}

	require.NotZero(t, checked,
		"no row of §11 takes a positional argument, so this guard measured nothing")
}
