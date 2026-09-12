package finding

import (
	"encoding/json"
	"maps"
	"slices"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/deligoez/cr/internal/git"
	"github.com/deligoez/cr/internal/state"
)

// specMergedFields is §6.5.1 read independently: §6.1's rows a merged line may
// carry, which is every row the Required column does not mark computed, minus
// §6.1.4's three Optional additions, plus `duplicate_of` — "its output MUST
// carry no computed field except `duplicate_of`" — and minus §2.3.3's head and
// round, which state.DecodeStamped rejects by presence on any file handed to a
// recording command.
//
// It is a second transcription of the same table for the reason
// finding_test.go's specFields is: the two being identical is the point.
// Collapsing them would leave §6.5.1 checked against the list that implements
// it.
var specMergedFields = []string{
	"id", "kind", "role", "class", "rule", "severity", "unit", "claim",
	"anchor", "summary", "evidence", "citations", "probe", "suggestion",
	"suggestion_origin", "duplicate_of", "suppressed_by",
}

// §6.5.1's field set, in both directions: what a merged line keeps is §6.1's
// table minus §6.1.4's fence plus the one exception, and nothing about it is
// decided by hand.
func TestAMergedLineCarriesNoComputedFieldButDuplicateOf(t *testing.T) {
	assert.Equal(t, specMergedFields, mergedFields,
		"§6.5.1 fixes what `cr merge` may write, and this list is read off §6.1's table")

	for _, field := range Reserved() {
		if field == "duplicate_of" {
			assert.Contains(t, mergedFields, field,
				"§6.5.1 names `duplicate_of` as the one computed field a merged line carries")
			continue
		}
		assert.NotContainsf(t, mergedFields, field,
			"§6.1.4 reserves %s, so §6.5.1 keeps it out of `cr merge`'s output", field)
	}
	for _, stamped := range []string{"head", "round"} {
		assert.NotContainsf(t, mergedFields, stamped,
			"§2.3.3 puts %s on a stored record, and state.DecodeStamped rejects it on the wire",
			stamped)
	}
}

// mergedRecord is a record carrying every computed field `cr merge` writes in
// memory, so what MergedRecords takes back out is measured rather than assumed.
func mergedRecord() *Finding {
	record := &Finding{
		ID: "f1", Kind: KindFinding, Role: "correctness",
		Class: "unchecked-error", Severity: SeverityHigh, Unit: "u1",
		Anchor: Anchor{
			Path: "lib.go", Side: git.Right, StartLine: 4, Line: 4,
			ContentHash: "0123456789abcdef",
		},
		Summary:  "The returned error is dropped.",
		Evidence: "The call's second result is assigned to the blank identifier.",
		Citations: []Citation{
			{Path: "other.go", Line: 12, ContentHash: "fedcba9876543210", Origin: OriginAgent},
		},
		// Everything below is cr's, written during the merge and never
		// on the wire.
		Axis: "correctness", Grade: GradeCited, State: StateDraft,
		DuplicateOf: "f2",
	}
	record.Stamp = state.Stamp{Head: "0f1e2d3", Round: 3}
	return record
}

// A merged line holds §6.1's rows and no computed field but `duplicate_of` —
// head and round among the ones it does not hold.
//
// The stamp is the half found by command-surface-stubs: state.Stamp carries
// head and round without `omitempty`, deliberately, because §2.3.3 requires
// both on a stored record. Marshalling a decoded Finding therefore emits
// `"head":""` and `"round":0`, and state.DecodeStamped rejects those keys by
// presence — so a merge that wrote them would produce a file `cr record`
// refuses on every line.
func TestMergedRecordsDropTheStampAndEveryComputedField(t *testing.T) {
	body, err := MergedRecords([]*Finding{mergedRecord()})
	require.NoError(t, err)

	lines := strings.Split(strings.TrimSuffix(string(body), "\n"), "\n")
	require.Len(t, lines, 1, "MergedRecords writes one JSON document per line")
	var held map[string]json.RawMessage
	require.NoError(t, json.Unmarshal([]byte(lines[0]), &held))

	for name := range held {
		assert.Containsf(t, mergedFields, name,
			"§6.5.1: a merged line carries no field but §6.1's own, and it carries %s", name)
	}
	assert.NotContains(t, held, "head", "§2.3.3's pair is `cr record`'s to stamp")
	assert.NotContains(t, held, "round")
	assert.NotContains(t, held, "grade", "§6.5.1: the grade is computed for the counts and not written")
	assert.NotContains(t, held, "axis")
	assert.NotContains(t, held, "state", "§6.5.1 leaves `state` to `cr record`")
	assert.JSONEq(t, `"f2"`, string(held["duplicate_of"]),
		"§6.4.3's mark is the one computed field §6.5.1 lets a merged line carry")

	var cited []map[string]json.RawMessage
	require.NoError(t, json.Unmarshal(held["citations"], &cited))
	require.Len(t, cited, 1)
	assert.NotContains(t, cited[0], "content_hash",
		"§6.2.3's hash is cr's, and §6.1.4 rejects a record arriving with it")
	assert.NotContains(t, cited[0], "origin", "§6.2.5 says the same of the origin")
	assert.JSONEq(t, `"other.go"`, string(cited[0]["path"]),
		"what the agent wrote about the citation survives the merge")

	// §6.1's order, so a merged file reads beside the table.
	var order []string
	for _, name := range mergedFields {
		if _, carried := held[name]; carried {
			order = append(order, name)
		}
	}
	assert.Equal(t, order, keysInOrder(t, lines[0]))
}

// keysInOrder reads one JSON object's keys in the order the document writes
// them, which json.Unmarshal into a map cannot report.
func keysInOrder(t *testing.T, line string) []string {
	t.Helper()
	decoder := json.NewDecoder(strings.NewReader(line))
	opening, err := decoder.Token()
	require.NoError(t, err)
	require.Equal(t, json.Delim('{'), opening)

	var names []string
	for decoder.More() {
		key, err := decoder.Token()
		require.NoError(t, err)
		names = append(names, key.(string))
		// The value is consumed whole, so a nested object's keys are
		// not read as the outer object's.
		var skipped json.RawMessage
		require.NoError(t, decoder.Decode(&skipped))
	}
	return names
}

// `cr merge`'s output goes back through the door `cr record` reads it with, and
// is accepted.
//
// This is the assertion the round finding asks for in as many words: the test
// drives the output back through the decoder rather than asserting on the file
// alone. Asserting on the fields is a claim about a list; decoding is a claim
// about the two commands agreeing, which is the one that fails when either side
// moves.
func TestCRRecordAcceptsWhatCRMergeWrites(t *testing.T) {
	body, err := MergedRecords([]*Finding{mergedRecord()})
	require.NoError(t, err)

	// SourceMerge is §6.5.1's exemption and covers `duplicate_of` alone;
	// every other computed field is refused whatever the source says.
	read, err := Decode("merged.ndjson", body, []string{"u1"}, SourceMerge)
	require.NoError(t, err, "§6.5.1 hands `cr merge`'s output to `cr record`, which must accept it")
	require.Len(t, read, 1)
	assert.Equal(t, "f2", read[0].DuplicateOf)
	assert.Empty(t, read[0].Grade, "the grade `cr record` computes is its own")
	assert.Equal(t, state.Stamp{}, read[0].Stamp, "§2.3.3's pair arrives from the writer")

	// And the same bytes through the door that refuses the exemption: what
	// `cr record` accepts here it accepts because of `duplicate_of` and
	// nothing else.
	_, err = Decode("merged.ndjson", body, []string{"u1"}, SourceAgent)
	var reserved *state.ReservedFieldError
	require.ErrorAs(t, err, &reserved,
		"§6.1.4 reserves `duplicate_of` everywhere but §6.5.1's one input")
	assert.Equal(t, "duplicate_of", reserved.Field)
}

// A record with no citations writes no `citations` key, rather than an empty
// array cr would then have to explain.
func TestAMergedLineOmitsWhatTheRecordDoesNotHold(t *testing.T) {
	record := mergedRecord()
	record.Citations = nil
	record.DuplicateOf = ""

	body, err := MergedRecords([]*Finding{record})
	require.NoError(t, err)

	var held map[string]json.RawMessage
	require.NoError(t, json.Unmarshal(body, &held))
	assert.NotContains(t, held, "citations")
	assert.NotContains(t, held, "duplicate_of",
		"a record nothing suppressed is not marked as suppressed")
	assert.NotContains(t, held, "claim")
	for _, field := range Fields() {
		if field.Requirement != Required {
			continue
		}
		assert.Containsf(t, slices.Collect(maps.Keys(held)), field.Name,
			"§6.1 requires %s of every record, and a merged line carries it", field.Name)
	}
}
