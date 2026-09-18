package cli

import (
	"regexp"
	"slices"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// §13 has skills/cr/SKILL.md ship with the release as the document an agent
// loads every cycle, so a command it never names is a command no agent learns
// exists. These tests read the command surface off §11's own table rather
// than off a list kept here, and hold the parse to the built tree, so neither
// a command added to the spec nor one lost by the parser passes unnoticed.

// currentSpec is the normative contract §11's table is read out of, and the
// one line a version bump moves. The spec file is this guard's fixture: what
// the skill and the built tree are held to is the table the release ships,
// never a list kept in this package.
const currentSpec = "spec/0.4.0.md"

// specCommands names every command §11's table lists, spelled as typed: the
// words before the first positional or optional argument, with `a|b`
// alternatives expanded and a flag that distinguishes a row (`init
// --eject-roles`, `note --remove`) kept. `--repo` is dropped because §11.1
// makes it a global override on every command rather than part of one.
func specCommands(t *testing.T) []string {
	t.Helper()
	spec := string(repoFile(t, currentSpec))
	start := strings.Index(spec, "\n## 11. ")
	require.GreaterOrEqual(t, start, 0, "%s has no §11 heading", currentSpec)
	end := strings.Index(spec[start:], "\n### 11.1 ")
	require.Positive(t, end, "§11's command table has no §11.1 after it")

	names := make([]string, 0)
	for line := range strings.SplitSeq(spec[start:start+end], "\n") {
		if !strings.HasPrefix(line, "| `cr ") {
			continue
		}
		form := strings.ReplaceAll(strings.SplitN(line, "`", 3)[1], `\|`, "|")
		names = append(names, commandNames(form)...)
	}
	require.NotEmpty(t, names, "a guard over none of §11's commands proves nothing")
	return names
}

// commandNames expands one §11 form into the commands it names.
func commandNames(form string) []string {
	words := make([]string, 0)
	for _, word := range strings.Fields(form)[1:] {
		if strings.HasPrefix(word, "<") || strings.HasPrefix(word, "[") {
			break
		}
		if word != "--repo" {
			words = append(words, word)
		}
	}
	names := []string{""}
	for _, word := range words {
		expanded := make([]string, 0, len(names))
		for _, name := range names {
			for alternative := range strings.SplitSeq(word, "|") {
				expanded = append(expanded, strings.TrimSpace(name+" "+alternative))
			}
		}
		names = expanded
	}
	return names
}

// unnamedCommands returns the commands doc never writes as `cr <command>`
// followed by a space, a backtick, or the end of a line.
func unnamedCommands(doc string, names []string) []string {
	missing := make([]string, 0)
	for _, name := range names {
		named := regexp.MustCompile(`(?m)\bcr ` + regexp.QuoteMeta(name) + "(?:[\\s`]|$)")
		if !named.MatchString(doc) {
			missing = append(missing, name)
		}
	}
	return missing
}

// Every command of §11 appears in SKILL.md, per §13.
func TestTheSkillNamesEveryCommandOfSection11(t *testing.T) {
	names := specCommands(t)
	doc := string(repoFile(t, "skills/cr/SKILL.md"))

	assert.Empty(t, unnamedCommands(doc, names), "§13 has SKILL.md document the loop; these §11 commands are never named in it")

	// The matcher must be able to fail: a document that no longer spells
	// `cr stats` reports exactly that command.
	assert.Equal(t, []string{"stats"}, unnamedCommands(strings.ReplaceAll(doc, "cr stats", "cr xstats"), names))
}

// The table the skill is checked against is the tree cr builds: every command
// §11 names is a command the binary has, and every command the binary has is
// one §11 names. Without this a parse that silently dropped a row would shrink
// what the skill is held to.
func TestSection11sTableIsTheBuiltCommandTree(t *testing.T) {
	commands := make([]string, 0)
	for _, name := range specCommands(t) {
		if !strings.Contains(name, "--") && !slices.Contains(commands, name) {
			commands = append(commands, name)
		}
	}
	assert.ElementsMatch(t, leafCommands(t), commands)
}
