package finding

import (
	"encoding/json"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// roundUnits is the current round's units, which is the set §6.1.3 reads a
// record's unit against.
var roundUnits = []string{"u1", "u2"}

// aRecord is the smallest line §6.1.3 lets through: every required row of
// §6.1's table and nothing else. Each case takes a copy and breaks one thing,
// so what it asserts is the difference from a record that passes.
func aRecord() map[string]any {
	return map[string]any{
		"id":       "f1",
		"kind":     "finding",
		"role":     "test",
		"class":    "missing-test",
		"severity": "high",
		"unit":     "u1",
		"anchor": map[string]any{
			"path":       "app/Models/User.php",
			"side":       "RIGHT",
			"start_line": 12,
			"line":       14,
		},
		"summary":  "The added guard clause has no test.",
		"evidence": "No changed test file references the method.",
	}
}

// onLineThree hands the record in behind a good record and a blank line, so an
// error reporting the record's position among the records rather than its line
// in the file would name 2 and be caught.
func onLineThree(t *testing.T, record map[string]any) []byte {
	t.Helper()
	first, err := json.Marshal(aRecord())
	require.NoError(t, err)
	line, err := json.Marshal(record)
	require.NoError(t, err)
	return append(append(append(first, "\n\n"...), line...), '\n')
}

// rejects decodes the record and returns the §6.1.3 rejection it earned.
func rejects(t *testing.T, record map[string]any) *RejectedRecordError {
	t.Helper()
	_, err := Decode(FanOutFile("test"), onLineThree(t, record), roundUnits)
	var rejected *RejectedRecordError
	require.ErrorAs(t, err, &rejected)
	assert.Equal(t, FanOutFile("test"), rejected.File)
	assert.Equal(t, 3, rejected.Line)
	assert.Contains(t, err.Error(), "line 3")
	return rejected
}

// §6.1.3 rejects a record missing any field §6.1 marks required, naming the
// line and the field. Every required row is omitted in turn, because a
// validator that checked eight of the nine would pass the ninth for the whole
// life of the tool without anything else noticing.
//
// A key holding null or "" is missing too. Presence is read from the wire for
// the reason state.DecodeStamped gives about head and round — a decoded struct
// cannot say which keys were there — and a record whose evidence is the empty
// string has no evidence: letting it through would put a record in the
// assertion register with nothing behind it, which is what §6.1.3 guards.
func TestARecordMissingARequiredFieldIsNamedByLineAndField(t *testing.T) {
	required := make([]string, 0, len(Fields()))
	for _, field := range Fields() {
		if field.Requirement == Required {
			required = append(required, field.Name)
		}
	}
	require.Equal(t, []string{
		"id", "kind", "role", "class", "severity", "unit", "anchor", "summary", "evidence",
	}, required, "§6.1's required rows, in table order")

	records, err := Decode(FanOutFile("test"), onLineThree(t, aRecord()), roundUnits)
	require.NoError(t, err, "a record supplying every required field passes")
	require.Len(t, records, 2)

	for _, field := range required {
		t.Run(field, func(t *testing.T) {
			omitted := aRecord()
			delete(omitted, field)
			assert.Equal(t, field, rejects(t, omitted).Field)

			absent := aRecord()
			absent[field] = nil
			assert.Equal(t, field, rejects(t, absent).Field, "null supplies nothing")

			if field == "anchor" {
				// An anchor of "" is not an anchor of any
				// shape, so the decode refuses the line before
				// §6.1.3 reads a key of it.
				return
			}
			blank := aRecord()
			blank[field] = ""
			assert.Equal(t, field, rejects(t, blank).Field, `"" supplies nothing`)
		})
	}
}

// §6.1.2 has every record carry an anchor, and §4.1.3 says why: an item with no
// code location never becomes a record, because v0.1 has no unanchored comment
// channel to post it through. An anchor object that names no path is that item
// wearing the shape of a record.
func TestAnItemWithNoCodeLocationNeverBecomesARecord(t *testing.T) {
	for name, anchor := range map[string]map[string]any{
		"an anchor with no path": {"side": "RIGHT", "start_line": 12, "line": 14},
		"an empty anchor":        {},
	} {
		t.Run(name, func(t *testing.T) {
			record := aRecord()
			record["anchor"] = anchor
			rejected := rejects(t, record)
			assert.Equal(t, "anchor", rejected.Field)
			assert.Contains(t, rejected.Error(), "no code location")
		})
	}
}

