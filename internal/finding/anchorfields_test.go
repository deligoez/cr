package finding

import (
	"reflect"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// AnchorFields names every key Anchor decodes, in Anchor's own order, read off
// the struct's JSON tags so a key the struct gains is a key the table has to
// name.
func TestAnchorFieldsNameEveryKeyAnAnchorDecodes(t *testing.T) {
	anchor := reflect.TypeFor[Anchor]()
	decoded := make([]string, 0, anchor.NumField())
	for field := range anchor.Fields() {
		name, _, _ := strings.Cut(field.Tag.Get("json"), ",")
		decoded = append(decoded, name)
	}
	named := make([]string, 0, len(decoded))
	for _, field := range AnchorFields() {
		named = append(named, field.Name)
	}

	assert.Equal(t, decoded, named)
}

// AnchorFields' Required column is exactly the decoder's: a line whose anchor
// omits a required key is refused naming the anchor, and one omitting an
// optional key is accepted. The prompt tells a role which keys it must write by
// this column (§4.6.2), so a column wider than the decoder would demand keys
// nothing needs, and a narrower one would let a role leave out a key its line
// is then refused for.
func TestAnAnchorKeyIsRequiredExactlyWhenALineWithoutItIsRefused(t *testing.T) {
	for _, field := range AnchorFields() {
		t.Run(field.Name, func(t *testing.T) {
			anchor := map[string]any{
				"path": "app/Models/User.php", "side": "RIGHT", "start_line": 12, "line": 14,
				"content_hash": "0123456789abcdef", "context_before": []string{"a"}, "context_after": []string{"b"},
			}
			delete(anchor, field.Name)
			record := aRecord()
			record["anchor"] = anchor

			_, err := DecodePerRole(FanOutFile("test"), onLineThree(t, record), roundUnits)

			if field.Requirement != Required {
				assert.NoError(t, err)
				return
			}
			var rejected *RejectedRecordError
			require.ErrorAs(t, err, &rejected)
			assert.Equal(t, "anchor", rejected.Field)
			assert.Equal(t, 3, rejected.Line)
		})
	}
}
