package finding

import (
	"encoding/json"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/deligoez/cr/internal/state"
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
	_, err := Decode(FanOutFile("test"), onLineThree(t, record), roundUnits, SourceAgent)
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

	records, err := Decode(FanOutFile("test"), onLineThree(t, aRecord()), roundUnits, SourceAgent)
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

// §6.1.3 rejects a record whose unit is not a unit of the current round. It is
// the round's own set that decides, not the id's shape: §9.3.4 recomputes units
// on every new head, so a unit id an earlier round issued names nothing now.
//
// The check proves the unit exists, never that it is the record's own. A record
// anchored in one unit may still declare another, and §6.2.1's containment
// predicate is what has to catch that; nothing here does.
func TestARecordNamingAUnitOfNoCurrentRoundIsRejected(t *testing.T) {
	record := aRecord()
	record["unit"] = "u9"
	rejected := rejects(t, record)
	assert.Equal(t, "unit", rejected.Field)
	assert.Contains(t, rejected.Error(), `"u9" is not a unit of the current round`)

	records, err := Decode(FanOutFile("test"), onLineThree(t, record), []string{"u1", "u9"}, SourceAgent)
	require.NoError(t, err, "the same record passes once u9 is a unit of the round")
	require.Len(t, records, 2)
	assert.Equal(t, "u9", records[1].Unit)
}

// §4.6.2 fixes no form for the path a role writes to, which would leave `role`
// a field the agent writes and cr believes. §6.1's axis is derived from it and
// §6.2 grades `cited` only when the axis is not `test`, so a test-adequacy
// record writing `role: correctness` would buy the assertion register the
// experiment §4.4.2 demands. Pinning the path to review-<role-id>.ndjson gives
// §6.1.3 something to check the field against.
func TestARecordsRoleIsTheOneOfTheFileItArrivedIn(t *testing.T) {
	assert.Equal(t, "review-test.ndjson", FanOutFile("test"))

	role, bound := RoleForFile(filepath.Join(t.TempDir(), FanOutFile("correctness")))
	assert.True(t, bound, "a role may write into any directory; the base name binds it")
	assert.Equal(t, "correctness", role)
	for _, name := range []string{"merged.ndjson", FanOutFile(""), "review-test.json"} {
		_, bound := RoleForFile(name)
		assert.False(t, bound, name)
	}

	borrowed := aRecord()
	borrowed["role"] = "correctness"
	rejected := rejects(t, borrowed)
	assert.Equal(t, "role", rejected.Field)
	assert.Contains(t, rejected.Error(), `"correctness" is not "test"`)

	_, err := DecodePerRole("merged.ndjson", onLineThree(t, aRecord()), roundUnits)
	var unattributable *UnattributableFileError
	require.ErrorAs(t, err, &unattributable, "cr merge refuses input it cannot bind to a role")
	assert.Equal(t, "merged.ndjson", unattributable.File)
	assert.Contains(t, unattributable.Error(), "review-<role-id>.ndjson")

	records, err := DecodePerRole(FanOutFile("test"), onLineThree(t, aRecord()), roundUnits)
	require.NoError(t, err, "and reads one it can bind")
	assert.Len(t, records, 2)

	records, err = Decode("merged.ndjson", onLineThree(t, borrowed), roundUnits, SourceAgent)
	require.NoError(t, err, "cr merge's own output holds several roles and its name binds none")
	require.Len(t, records, 2)
	assert.Equal(t, "correctness", records[1].Role)
}

// oversteps decodes the record as the agent's own and returns the §6.1.4
// rejection it earned. The file it arrives in carries a role, so the record is
// held to the whole of §6.1.3 as well and nothing about the name softens the
// fence.
func oversteps(t *testing.T, record map[string]any) *state.ReservedFieldError {
	t.Helper()
	_, err := Decode(FanOutFile("test"), onLineThree(t, record), roundUnits, SourceAgent)
	var overstep *state.ReservedFieldError
	require.ErrorAs(t, err, &overstep)
	assert.Equal(t, FanOutFile("test"), overstep.File)
	assert.Equal(t, 3, overstep.Line)
	assert.Contains(t, err.Error(), "line 3")
	return overstep
}

// §6.1.4 has cr write the computed fields and rejects a record arriving with
// one. Each is supplied in turn, because a fence that held for seven of eight
// fields would hand the eighth to the agent for the life of the tool: the agent
// that writes its own grade writes itself past §6.3's forcing, the one that
// writes its own axis picks the axis §6.2 grades it on, and the one that writes
// state answers §9.1 for cr.
//
// Presence alone is the fault. A key holding null was still written by the
// agent, and the field has one author whatever value sits under it — unlike a
// required field, where §6.1.3 reads null as nothing supplied.
func TestARecordArrivingWithAComputedFieldIsRejected(t *testing.T) {
	require.Equal(t, []string{
		"axis", "grade", "state", "disposition", "duplicate_of", "thread_id",
		"span_hash", "issue_hash",
	}, reserved, "§6.1.4's fence, in §6.1's table order with §3.3's two last")

	for field, value := range map[string]any{
		"axis":        "correctness",
		"grade":       "cited",
		"state":       "queued",
		"disposition": "wrong",
		// §6.5.1's exemption is on cr merge's output alone, and
		// TestDuplicateOfIsExemptOnlyOnMergeOutput is where that is
		// read both ways round.
		"duplicate_of": "f1",
		"thread_id":    "PRRT_kwDOAbCd",
		// §3.3's computed claim fields. §6.1.4 names them in the same
		// sentence as its own, and no finding has a use for either.
		"span_hash":  "9f2a1c",
		"issue_hash": "4b7e00",
	} {
		t.Run(field, func(t *testing.T) {
			supplied := aRecord()
			supplied[field] = value
			assert.Equal(t, field, oversteps(t, supplied).Field)
			assert.Contains(t, oversteps(t, supplied).Error(), "written by cr")

			empty := aRecord()
			empty[field] = nil
			assert.Equal(t, field, oversteps(t, empty).Field, "null is a key the agent wrote")
		})
	}

	// §6.1's citations row computes two of an entry's four fields, and
	// §6.2.5 rejects a record arriving with any origin at all: cr stamps
	// origin positionally against its own detection output, and an agent
	// that could write `origin: rule` would buy its own record the cited
	// grade §6.2 withholds from a citation inside the record's own unit.
	cited := aRecord()
	cited["citations"] = []any{map[string]any{"path": "app/Models/User.php", "line": 12}}
	records, err := Decode(FanOutFile("test"), onLineThree(t, cited), roundUnits, SourceAgent)
	require.NoError(t, err, "an entry of path and line is the whole of what the agent supplies")
	require.Len(t, records, 2)

	for _, field := range CitationFields() {
		if field.Requirement != Computed {
			continue
		}
		t.Run("citations."+field.Name, func(t *testing.T) {
			stamped := aRecord()
			stamped["citations"] = []any{
				map[string]any{"path": "app/Models/User.php", "line": 12},
				map[string]any{"path": "app/Models/User.php", "line": 20, field.Name: "rule"},
			}
			assert.Equal(t, "citations[1]."+field.Name, oversteps(t, stamped).Field,
				"the entry at fault is named by its index")
		})
	}
}

// §6.1.4's fence under any letter case. encoding/json binds `"Grade"` to the
// grade field exactly as it binds `"grade"`, so a fence that looked keys up by
// their exact spelling would let a record set its own grade by capitalising a
// letter. Every reserved field is supplied upper-cased, one mixed-case and one
// under a Unicode letter the decode folds (U+017F long s), and each refusal
// names the field by §6.1's own spelling. The citations entry is held to the
// same reading for its computed fields and for §2.6.1.3's rule id.
func TestAComputedFieldIsRefusedUnderAnyKeyCase(t *testing.T) {
	values := map[string]any{
		"axis": "correctness", "grade": "cited", "state": "queued",
		"disposition": "wrong", "duplicate_of": "f1", "thread_id": "PRRT_kwDOAbCd",
		"span_hash": "9f2a1c", "issue_hash": "4b7e00",
	}
	require.Len(t, values, len(reserved), "a value for every field of the fence")
	for _, field := range reserved {
		t.Run(field, func(t *testing.T) {
			supplied := aRecord()
			supplied[strings.ToUpper(field)] = values[field]
			assert.Equal(t, field, oversteps(t, supplied).Field)
		})
	}
	for key, field := range map[string]string{
		"Thread_ID":    "thread_id",
		"Duplicate_Of": "duplicate_of",
		"\u017ftate":   "state",
	} {
		t.Run(key, func(t *testing.T) {
			supplied := aRecord()
			supplied[key] = values[field]
			assert.Equal(t, field, oversteps(t, supplied).Field)
		})
	}

	for key, field := range map[string]string{"Content_Hash": "content_hash", "ORIGIN": "origin"} {
		t.Run("citations."+key, func(t *testing.T) {
			stamped := aRecord()
			stamped["citations"] = []any{
				map[string]any{"path": "app/Models/User.php", "line": 12},
				map[string]any{"path": "app/Models/User.php", "line": 20, key: "rule"},
			}
			assert.Equal(t, "citations[1]."+field, oversteps(t, stamped).Field)
		})
	}

	ruled := aRecord()
	ruled["rule"] = "no-panic"
	ruled["citations"] = []any{map[string]any{"path": "app/Models/User.php", "line": 12, "Rule": "no-panic"}}
	assert.Equal(t, "citations[0].rule", rejects(t, ruled).Field,
		"§2.6.1.3 keeps the rule id off a citation under any spelling of it")
}

// A field spelt twice in one line holds the value the decode keeps, the later
// one, and §6.1.3 reads that value: a record whose evidence ends as the empty
// string has no evidence, whatever an earlier spelling of the key held.
func TestARequiredFieldIsReadAsTheDecodeKeepsIt(t *testing.T) {
	first, err := json.Marshal(aRecord())
	require.NoError(t, err)
	spelt := string(first[:len(first)-1])

	records, err := Decode(FanOutFile("test"),
		[]byte(spelt+`,"Evidence":"The guard has no test."}`+"\n"), roundUnits, SourceAgent)
	require.NoError(t, err, "a later spelling that supplies the field supplies it")
	require.Len(t, records, 1)
	assert.Equal(t, "The guard has no test.", records[0].Evidence)

	var rejected *RejectedRecordError
	_, err = Decode(FanOutFile("test"), []byte(spelt+`,"EVIDENCE":""}`+"\n"), roundUnits, SourceAgent)
	require.ErrorAs(t, err, &rejected)
	assert.Equal(t, &RejectedRecordError{
		File: FanOutFile("test"), Line: 1, Field: "evidence",
		Problem: "is required by §6.1 and this record does not supply it",
	}, rejected)
}

// §6.1.4 exempts duplicate_of on `cr merge` output and nowhere else, so the
// same bytes are read two ways and the exemption rides on neither the entry
// point nor the file's name. §6.4.3 keeps a duplicate out of the draft
// entirely: an agent that could set the field on a record of its own would
// silence that record by naming another, without a human seeing either.
func TestDuplicateOfIsExemptOnlyOnMergeOutput(t *testing.T) {
	suppressed := aRecord()
	suppressed["duplicate_of"] = "f1"

	records, err := Decode("merged.ndjson", onLineThree(t, suppressed), roundUnits, SourceMerge)
	require.NoError(t, err, "§6.5.1 lets cr merge's output carry the one computed field")
	require.Len(t, records, 2)
	assert.Equal(t, "f1", records[1].DuplicateOf)

	assert.Equal(t, "duplicate_of", oversteps(t, suppressed).Field,
		"the same record handed to cr record as the agent's own is refused")

	var reservedField *state.ReservedFieldError
	_, err = Decode("merged.ndjson", onLineThree(t, suppressed), roundUnits, SourceAgent)
	require.ErrorAs(t, err, &reservedField)
	assert.Equal(t, "duplicate_of", reservedField.Field,
		"the name is the agent's claim about its own input and buys nothing")

	_, err = DecodePerRole(FanOutFile("test"), onLineThree(t, suppressed), roundUnits)
	require.ErrorAs(t, err, &reservedField)
	assert.Equal(t, "duplicate_of", reservedField.Field,
		"the exemption is on what cr merge writes, never on what it reads")

	graded := aRecord()
	graded["grade"] = "cited"
	_, err = Decode("merged.ndjson", onLineThree(t, graded), roundUnits, SourceMerge)
	require.ErrorAs(t, err, &reservedField)
	assert.Equal(t, "grade", reservedField.Field,
		"duplicate_of is the only computed field §6.5.1 lets that output carry")
}
