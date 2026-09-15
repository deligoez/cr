package cli

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/deligoez/cr/internal/finding"
	"github.com/deligoez/cr/internal/git"
	"github.com/deligoez/cr/internal/review"
)

// contractOf reads the record contract the prompt names (§4.6.2), which is
// where §6.1's schema is since 0.3.0.
func contractOf(t *testing.T, prompt *review.Prompt) string {
	t.Helper()
	_, named, found := strings.Cut(prompt.Text, "read it before writing a record:\n\n    ")
	require.Truef(t, found, "%s on %s names its contract file", prompt.Role, prompt.Unit)
	path, _, _ := strings.Cut(named, "\n")
	body, err := os.ReadFile(path)
	require.NoError(t, err, "the file the prompt names is written")
	return string(body)
}

// promptValues reads the closed set a text names on the line that starts with
// lead and goes on " one of ...", unquoted and in the order it is written.
func promptValues(t *testing.T, text, lead string) []string {
	t.Helper()
	_, rest, found := strings.Cut(text, "\n"+lead+" one of ")
	require.Truef(t, found, "the prompt states the values of %q", lead)
	line, _, _ := strings.Cut(rest, "\n")
	values := make([]string, 0)
	for part := range strings.SplitSeq(line, ", ") {
		value, err := strconv.Unquote(part)
		if err != nil {
			break
		}
		values = append(values, value)
	}
	return values
}

// spelled writes a closed set of a defined string type as plain strings.
func spelled[T ~string](values []T) []string {
	out := make([]string, 0, len(values))
	for _, value := range values {
		out = append(out, string(value))
	}
	return out
}

// §4.6.2 end to end: every prompt `cr review` emits names every value the
// decoder accepts for kind, severity, suggestion_origin and side, and every key
// of the anchor object; and records written from nothing but what each prompt
// states — its unit's path, every kind, severity and side it names, the lowest
// start_line it allows, and for LEFT a line its hunk shows removed — to the
// output path it names, numbered from the first id of the block it names, pass
// `cr merge` (audit round 14's list-25-2, where the prompt carried field names
// only and a role could not write a valid anchor).
func TestRecordsWrittenFromWhatCrReviewStatesPassCrMerge(t *testing.T) {
	statusHome(t)
	printed, err := runCLIPrinting(t, "review", fixturePR, "--repo", fixtureSlug)
	require.NoError(t, err)
	var fanout review.Fanout
	require.NoError(t, json.Unmarshal([]byte(printed), &fanout))
	require.NotEmpty(t, fanout.Prompts)

	files := make([]string, 0, len(fanout.Prompts))
	written := 0
	for _, prompt := range fanout.Prompts {
		text := prompt.Text
		schema := contractOf(t, &prompt)
		id, numbered := finding.IDSuffix(prompt.FirstID)
		require.True(t, numbered, "the prompt names the first id of its block")
		id--
		kinds := promptValues(t, schema, "- kind:")
		severities := promptValues(t, schema, "- severity:")
		sides := promptValues(t, schema, "  - side: required;")
		assert.Equal(t, spelled(finding.Kinds()), kinds)
		assert.Equal(t, spelled(finding.Severities()), severities)
		assert.Equal(t, spelled(finding.Origins()), promptValues(t, schema, "- suggestion_origin:"))
		assert.Equal(t, spelled(git.Sides()), sides)
		for _, field := range finding.AnchorFields() {
			assert.Contains(t, schema, "\n  - "+field.Name+": ", "the contract names the anchor's %s", field.Name)
		}

		_, unitLine, found := strings.Cut(text, "\n## Unit "+prompt.Unit+" (§3.4)\n\n")
		require.True(t, found)
		path, _, _ := strings.Cut(unitLine, ",")
		_, startLine, found := strings.Cut(schema, "\n  - start_line: required; ")
		require.True(t, found)
		var first int
		_, err := fmt.Sscanf(startLine, "an integer, %d or greater", &first)
		require.NoError(t, err)
		// §9.2.1 lets a LEFT anchor name a removed line only, which cr merge
		// refuses otherwise, and the prompt shows the unit's hunks: a LEFT
		// record anchors on the first line the first of them removes, and a
		// RIGHT one on the lowest start_line the prompt allows.
		_, block, found := strings.Cut(text, "```diff\n")
		require.True(t, found, "the prompt shows the unit's hunks")
		block, _, _ = strings.Cut(block, "\n```")
		shown, err := git.ParseHunks(block + "\n")
		require.NoError(t, err)
		require.NotEmpty(t, shown)
		require.NotEmpty(t, shown[0].Removed, "the fixture's hunk removes a line")
		lineOn := map[string]int{string(git.Right): first, string(git.Left): shown[0].Removed[0]}

		var body strings.Builder
		for _, kind := range kinds {
			for _, severity := range severities {
				for _, side := range sides {
					id++
					written++
					line, err := json.Marshal(map[string]any{
						"id": "f" + strconv.Itoa(id), "kind": kind, "role": prompt.Role,
						"class": "unchecked-error", "severity": severity, "unit": prompt.Unit,
						"anchor":  map[string]any{"path": path, "side": side, "start_line": lineOn[side], "line": lineOn[side]},
						"summary": "The error Load returns is dropped.", "evidence": "Nothing reads the result.",
					})
					require.NoError(t, err)
					body.Write(line)
					body.WriteByte('\n')
				}
			}
		}
		require.NoError(t, os.MkdirAll(filepath.Dir(prompt.Output), 0o750))
		require.NoError(t, os.WriteFile(prompt.Output, []byte(body.String()), 0o600))
		files = append(files, prompt.Output)
	}

	out := mergedOut(t)
	args := append([]string{"merge"}, files...)
	merged, err := runCLIPrinting(t, append(args, "-o", out, "--repo", fixtureSlug, "--pr", fixturePR)...)
	require.NoError(t, err, "a record written from the prompt's stated values passes cr merge")
	assert.Equal(t, written, decodeMergeResult(t, merged).Merged)
}
