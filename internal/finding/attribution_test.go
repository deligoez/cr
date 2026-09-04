package finding

import (
	"reflect"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// accepts decodes the record and returns what came back, requiring it through.
func accepts(t *testing.T, record map[string]any) *Finding {
	t.Helper()
	records, err := Decode(FanOutFile("test"), onLineThree(t, record), roundUnits, SourceAgent)
	require.NoError(t, err)
	require.Len(t, records, 2)
	return records[1]
}

// §2.6 item 3: a record a rule produced carries the rule id.
//
// `suggestion_origin: rule` is the mark cr can read on the way in. §2.6.2.4
// gives it to a suggestion a rule's `fix` produced and nothing else does, so a
// record wearing it has a rule behind it — and a record that says so without
// saying which rule leaves §7.3.2's per-rule statistics and §2.6.3.4's
// dead-rule report with a hit they cannot attribute.
//
// The record is otherwise the one §6.1.3 lets through, so what is asserted is
// the difference the two rule fields make and not some other fault.
func TestARuleProducedRecordWithoutTheRuleIDIsRejected(t *testing.T) {
	produced := aRecord()
	produced["suggestion"] = "if err != nil {"
	produced["suggestion_origin"] = "rule"

	rejected := rejects(t, produced)
	assert.Equal(t, "rule", rejected.Field)
	assert.Contains(t, rejected.Error(), "§2.6 item 3")

	produced["rule"] = "handle-every-error"
	assert.Equal(t, "handle-every-error", accepts(t, produced).Rule,
		"the id the record named is the id it carries")
}

// The mark is what makes the record answerable, not the presence of a
// suggestion.
//
// Two records are one field apart here. A record with the same suggestion and
// `suggestion_origin: agent` is the agent's own work and §2.6 asks nothing of
// it, and a record naming a rule while wearing no mark at all is a record the
// agent attributed itself, which §6.1 marks optional and admits. Without both
// halves the assertion above would also pass a validator that simply required
// `rule` on every record carrying a suggestion.
func TestOnlyTheRuleMarkRequiresTheID(t *testing.T) {
	agents := aRecord()
	agents["suggestion"] = "if err != nil {"
	agents["suggestion_origin"] = "agent"
	assert.Empty(t, accepts(t, agents).Rule,
		"§2.6 asks nothing of a suggestion the agent wrote")

	attributed := aRecord()
	attributed["rule"] = "handle-every-error"
	assert.Equal(t, "handle-every-error", accepts(t, attributed).Rule,
		"§6.1 marks `rule` optional, so a record may name one with no mark on it")
}

