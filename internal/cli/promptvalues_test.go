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

// promptValues reads the closed set a prompt names on the line that starts with
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
// start_line it allows — to the output path it names pass `cr merge` (audit
// round 14's list-25-2, where the prompt carried field names only and a role
// could not write a valid anchor).
func TestRecordsWrittenFromWhatCrReviewStatesPassCrMerge(t *testing.T) {
	statusHome(t)
	printed, err := runCLIPrinting(t, "review", fixturePR, "--repo", fixtureSlug)
	require.NoError(t, err)
	var fanout review.Fanout
	require.NoError(t, json.Unmarshal([]byte(printed), &fanout))
	require.NotEmpty(t, fanout.Prompts)

	files := make([]string, 0, len(fanout.Prompts))
	id := 0
	for _, prompt := range fanout.Prompts {
		text := prompt.Text
		kinds := promptValues(t, text, "- kind:")
		severities := promptValues(t, text, "- severity:")
		sides := promptValues(t, text, "  - side: required;")
		assert.Equal(t, spelled(finding.Kinds()), kinds)
		assert.Equal(t, spelled(finding.Severities()), severities)
		assert.Equal(t, spelled(finding.Origins()), promptValues(t, text, "- suggestion_origin:"))
		assert.Equal(t, spelled(git.Sides()), sides)
		for _, field := range finding.AnchorFields() {
			assert.Contains(t, text, "\n  - "+field.Name+": ", "the prompt names the anchor's %s", field.Name)
		}

		_, unitLine, found := strings.Cut(text, "\n## Unit "+prompt.Unit+" (§3.4)\n\n")
		require.True(t, found)
		path, _, _ := strings.Cut(unitLine, ",")
		_, startLine, found := strings.Cut(text, "\n  - start_line: required; ")
		require.True(t, found)
		var first int
		_, err := fmt.Sscanf(startLine, "an integer, %d or greater", &first)
		require.NoError(t, err)

		var body strings.Builder
		for _, kind := range kinds {
			for _, severity := range severities {
				for _, side := range sides {
					id++
					line, err := json.Marshal(map[string]any{
						"id": "f" + strconv.Itoa(id), "kind": kind, "role": prompt.Role,
						"class": "unchecked-error", "severity": severity, "unit": prompt.Unit,
						"anchor":  map[string]any{"path": path, "side": side, "start_line": first, "line": first},
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
	assert.Equal(t, id, decodeMergeResult(t, merged).Merged)
}
