package finding

import (
	"reflect"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// §5.5.2: a finding MUST reference at most one probe, by id.
//
// Both halves are structural rather than validated, and this is what says so —
// there is no checker to point at, because a record that could break the rule
// could not be decoded in the first place.
//
// "At most one" is the field's arity. A `probe` that were a slice would let a
// record rest on two experiments, and nothing in §6.2 says how to grade that: §6.2.1
// names "the referenced probe record" in the singular and §6.2.2 asks whether
// "its `target`" falls inside the record's anchor, which two probes could answer
// two ways. "At most" is §6.1's Optional row: a record resting on no experiment
// references none, and is graded `cited` or `argued` instead.
//
// "By id" is the field's type. A reference that carried a copy of the probe
// record would be a second copy of a line §5.5.1 declares immutable, free to
// disagree with the one on disk — and §5.5.3's same-head check is read off the
// stored record, never off what a finding says about it.
func TestAFindingReferencesAtMostOneProbeByID(t *testing.T) {
	field, declared := reflect.TypeFor[Finding]().FieldByName("Probe")
	require.True(t, declared, "§5.5.2: a finding references a probe")
	assert.Equal(t, reflect.String, field.Type.Kind(),
		"§5.5.2: one probe id, so not a slice and not the record itself")
	assert.Equal(t, "probe,omitempty", field.Tag.Get("json"),
		"§6.1's row, absent when the record rests on no experiment")
	assert.Contains(t, Fields(), Field{Name: "probe", Requirement: Optional},
		"§6.1 makes the reference optional, which is §5.5.2's \"at most\"")
}
