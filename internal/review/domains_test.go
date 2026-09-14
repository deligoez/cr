package review

import (
	"reflect"
	"strconv"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/deligoez/cr/internal/finding"
	"github.com/deligoez/cr/internal/git"
)

// statedValues reads the closed set a prompt names on the line that starts with
// lead and goes on " one of ...", unquoted and in the order it is written,
// stopping at the first part that is not a quoted value.
func statedValues(t *testing.T, text, lead string) []string {
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

// statedAnchor reads the anchor object's keys as the prompt lists them, each
// with the Required column's word the prompt gives it.
func statedAnchor(t *testing.T, text string) map[string]string {
	t.Helper()
	_, rest, found := strings.Cut(text, "\n- anchor: an object carrying\n")
	require.True(t, found, "the prompt states the anchor object's keys")
	keys := make(map[string]string)
	for line := range strings.SplitSeq(rest, "\n") {
		entry, nested := strings.CutPrefix(line, "  - ")
		if !nested {
			break
		}
		name, clause, _ := strings.Cut(entry, ": ")
		word, _, _ := strings.Cut(clause, ";")
		keys[name] = word
	}
	return keys
}

// asStrings spells a closed set of a defined string type as plain strings.
func asStrings[T ~string](values []T) []string {
	spelled := make([]string, 0, len(values))
	for _, value := range values {
		spelled = append(spelled, string(value))
	}
	return spelled
}

// anchorKeys is every key finding.Anchor decodes, read off its JSON tags rather
// than off AnchorFields, so a key the struct gains and the prompt does not name
// fails here.
func anchorKeys() []string {
	anchor := reflect.TypeFor[finding.Anchor]()
	keys := make([]string, 0, anchor.NumField())
	for i := range anchor.NumField() {
		name, _, _ := strings.Cut(anchor.Field(i).Tag.Get("json"), ",")
		keys = append(keys, name)
	}
	return keys
}

// §4.6.2's schema carries §6.1's value domains, not only the field names: every
// value the decoder accepts for kind, severity, suggestion_origin and the
// anchor's side, class's form, the id's spelling, and every key of §9.2's
// anchor object with its Required column (audit round 14's list-25-2).
//
// The expected sets are the decoder's own exports and the anchor struct's
// tags, so a value the validator comes to accept that the prompt does not
// name, or the reverse, fails here.
func TestEveryPromptStatesTheValuesARecordIsHeldTo(t *testing.T) {
	prompts := Emit(handRound())
	require.NotEmpty(t, prompts)
	for _, prompt := range prompts {
		assert.Equal(t, asStrings(finding.Kinds()), statedValues(t, prompt.Text, "- kind:"))
		assert.Equal(t, asStrings(finding.Severities()), statedValues(t, prompt.Text, "- severity:"))
		assert.Equal(t, asStrings(finding.Origins()), statedValues(t, prompt.Text, "- suggestion_origin:"))
		assert.Equal(t, asStrings(git.Sides()), statedValues(t, prompt.Text, "  - side: required;"))
		assert.Contains(t, prompt.Text, "\n- class: kebab-case, matching [a-z0-9-]+\n")
		assert.Contains(t, prompt.Text, "\n- id: f<n>, numbered from one\n")

		anchor := statedAnchor(t, prompt.Text)
		keys := make([]string, 0, len(anchor))
		for _, key := range anchorKeys() {
			if _, named := anchor[key]; named {
				keys = append(keys, key)
			}
		}
		assert.Equal(t, anchorKeys(), keys, "every key an anchor decodes is named")
		assert.Len(t, anchor, len(anchorKeys()), "and no key it does not")
		assert.Equal(t, map[string]string{
			"path": "required", "side": "required", "start_line": "required", "line": "required",
			"content_hash": "optional", "context_before": "optional", "context_after": "optional",
		}, anchor)
		assert.Contains(t, prompt.Text, "\n  - start_line: required; an integer, 1 or greater, the first line of the range\n")
		assert.Contains(t, prompt.Text, "\n  - context_before: optional; up to 3 lines above the range, "+
			"which cr records from the same tree\n")
	}
}

// A line written from nothing but what a prompt states — every kind, every
// severity and every side it names, on the unit and under the role the prompt
// is for — is one the decoder `cr merge` reads a role's file through accepts.
func TestALineWrittenFromThePromptsStatedValuesDecodes(t *testing.T) {
	r := handRound()
	units := []string{r.Units[0].ID, r.Units[1].ID}
	for _, prompt := range Emit(r) {
		var body strings.Builder
		n := 0
		for _, kind := range statedValues(t, prompt.Text, "- kind:") {
			for _, severity := range statedValues(t, prompt.Text, "- severity:") {
				for _, side := range statedValues(t, prompt.Text, "  - side: required;") {
					n++
					body.WriteString(`{"id":"f` + strconv.Itoa(n) + `","kind":` + strconv.Quote(kind) +
						`,"role":` + strconv.Quote(prompt.Role) + `,"class":"unchecked-error","severity":` +
						strconv.Quote(severity) + `,"unit":` + strconv.Quote(prompt.Unit) +
						`,"anchor":{"path":"a.go","side":` + strconv.Quote(side) + `,"start_line":1,"line":1},` +
						`"summary":"The error is dropped.","evidence":"The result is discarded."}` + "\n")
				}
			}
		}

		records, err := finding.DecodePerRole(prompt.Output, []byte(body.String()), units)

		require.NoError(t, err, "%s on %s", prompt.Role, prompt.Unit)
		assert.Len(t, records, 16, "two kinds, four severities and two sides")
	}
}
