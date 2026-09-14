package cli

import (
	"encoding/json"
	"os"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/deligoez/cr/internal/review"
	"github.com/deligoez/cr/internal/state"
)

// hitLines is the lines a prompt writes under one rule hit: the hit's own line
// and every line after it up to the next hit or the next section. It fails the
// test when the prompt carries no such hit, so an absent hit cannot pass as a
// hit with nothing under it.
func hitLines(t *testing.T, prompt, hit string) []string {
	t.Helper()
	lines := strings.Split(prompt, "\n")
	for at, line := range lines {
		if line != hit {
			continue
		}
		end := at + 1
		for end < len(lines) && !strings.HasPrefix(lines[end], "- rule ") && !strings.HasPrefix(lines[end], "## ") {
			end++
		}
		return lines[at:end]
	}
	require.Failf(t, "hit not in prompt", "no line %q in:\n%s", hit, prompt)
	return nil
}

// reviewPrompts activates the convention role on the fixture's round, the role
// §2.6's default axis names, runs `cr review` and returns every prompt it
// emitted, failing when it emitted none.
func reviewPrompts(t *testing.T, layout state.Layout) []review.Prompt {
	t.Helper()
	meta, err := layout.ReadMeta(fixtureOwner, fixtureProject, fixturePRNumber)
	require.NoError(t, err)
	meta.ActiveRoles = []string{"convention"}
	held, err := layout.LockPR(fixtureOwner, fixtureProject, fixturePRNumber)
	require.NoError(t, err)
	require.NoError(t, held.WriteMeta(&meta))
	require.NoError(t, held.Unlock())
	var fan review.Fanout
	require.NoError(t, json.Unmarshal(runReview(t), &fan))
	require.NotEmpty(t, fan.Prompts)
	return fan.Prompts
}

// QA S10 rule-12 through `cr review` and `cr record`: a prompt attaching a hit
// whose rule carries a fix shows the replacement that fix gives for the hit's
// line, so the agent sees the suggestion before it names the rule — and the
// suggestion `cr record` then attaches to a record confirming the hit is that
// same text, character for character.
func TestAHitsPromptShowsTheSuggestionItsRulesFixAttaches(t *testing.T) {
	layout := fixingHome(t)

	for _, prompt := range reviewPrompts(t, layout) {
		under := hitLines(t, prompt.Text, `- rule no-panic at lib.go:4: panic("one")`)
		require.GreaterOrEqual(t, len(under), 4, "%s on %s: %q", prompt.Role, prompt.Unit, under)
		assert.Equalf(t, []string{
			"  fix: replacing `panic\\((.*)\\)` with `return fmt.Errorf($1)` gives the suggestion below. " +
				"`cr record` attaches it, marked suggestion_origin: rule and labelled machine generated " +
				"in the draft (§2.6.2.4), to a record that names rule no-panic, cites lib.go:4, carries no " +
				"suggestion of its own, and is anchored where §8.2 admits a suggestion. " +
				"A record carrying its own suggestion keeps that one.",
			"```suggestion",
			"\treturn fmt.Errorf(\"one\")",
			"```",
		}, under[len(under)-4:], "%s on %s: the next hit follows the block", prompt.Role, prompt.Unit)
	}

	_, err := runRecord(t, fixturePR,
		writeRecordFile(t, "merged.ndjson", confirming("f1", 4)), "--repo", fixtureSlug)
	require.NoError(t, err)
	stored := storedFindings(t, layout)
	require.Len(t, stored, 1)
	assert.Equal(t, "\treturn fmt.Errorf(\"one\")", stored[0].Suggestion,
		"the suggestion recorded is the one the prompt showed")
}

// A fix whose pattern does not match the hit's line produces no suggestion at
// `cr record`, and the prompt says so instead of showing a block; a rule with
// no fix block says nothing about a fix at all.
func TestAHitsPromptSaysWhenItsRulesFixGivesNoSuggestion(t *testing.T) {
	t.Run("a fix that changes nothing", func(t *testing.T) {
		layout := reviewedHome(t)
		unmatched := strings.Replace(panicFixRule, `"replace":"panic\\((.*)\\)"`, `"replace":"recover\\(\\)"`, 1)
		require.NotEqual(t, panicFixRule, unmatched, "the fixture's fix.replace was not rewritten")
		require.NoError(t, os.WriteFile(layout.Rule("no-panic"), []byte(unmatched), 0o600))

		for _, prompt := range reviewPrompts(t, layout) {
			under := hitLines(t, prompt.Text, `- rule no-panic at lib.go:4: panic("one")`)
			assert.Equalf(t, []string{
				"  fix: replacing `recover\\(\\)` with `return fmt.Errorf($1)` changes nothing on this line, " +
					"so `cr record` attaches no suggestion to a record confirming this hit (§2.6.2.1).",
			}, under[len(under)-1:], "%s on %s", prompt.Role, prompt.Unit)
		}
	})
	t.Run("no fix block", func(t *testing.T) {
		layout := reviewedHome(t)

		for _, prompt := range reviewPrompts(t, layout) {
			for _, line := range hitLines(t, prompt.Text, `- rule no-panic at lib.go:4: panic("one")`) {
				assert.Falsef(t, strings.HasPrefix(line, "  fix: "), "%s on %s: %q", prompt.Role, prompt.Unit, line)
				assert.NotEqualf(t, "```suggestion", line, "%s on %s", prompt.Role, prompt.Unit)
			}
		}
	})
}
