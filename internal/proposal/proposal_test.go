package proposal

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// The file name every refusal in these tests names.
const file = "proposals.ndjson"

// span answers Contains for one path and one inclusive line range, which is the
// containment §6.2.1 asks of a unit reduced to what these tests need.
type span struct {
	path     string
	from, to int
}

func (s span) Contains(path string, line int) bool {
	return path == s.path && line >= s.from && line <= s.to
}

// head reads order.go as a file of twelve lines and holds nothing else, so a
// target outside it fails §6.2.3's resolution and one inside it does not.
func head(path string) (lines []string, exists bool, err error) {
	if path != "order.go" {
		return nil, false, nil
	}
	return make([]string, 12), true, nil
}

// round is a round holding one unit over order.go lines 4 to 9, two active
// roles, and one record.
func round() *Round {
	return &Round{
		Units:   []Unit{{ID: "u1", In: span{path: "order.go", from: 4, to: 9}}},
		Active:  []string{"correctness", "test-adequacy"},
		Records: []string{"f12"},
		Head:    head,
	}
}

// line is a complete proposal, with the named field replaced when one is given.
func line(overrides ...string) string {
	fields := map[string]string{
		"id":         `"x1"`,
		"kind":       `"mutation"`,
		"role":       `"correctness"`,
		"unit":       `"u1"`,
		"target":     `"order.go:5"`,
		"hypothesis": `"No test covers the discount ceiling."`,
		"settles":    `"A suite that stays green under the mutation proves the gap."`,
		"input":      `"--- a/order.go\n+++ b/order.go\n"`,
	}
	for i := 0; i+1 < len(overrides); i += 2 {
		if overrides[i+1] == "" {
			delete(fields, overrides[i])
			continue
		}
		fields[overrides[i]] = overrides[i+1]
	}
	body := "{"
	for _, name := range []string{
		"id", "kind", "role", "unit", "finding", "target",
		"hypothesis", "settles", "input", "filter", "paths",
		"state", "probe", "reason", "head", "round", "nonsense",
	} {
		value, given := fields[name]
		if !given {
			continue
		}
		if len(body) > 1 {
			body += ","
		}
		body += `"` + name + `":` + value
	}
	return body + "}\n"
}

// A complete proposal decodes into §5.7's fields, and cr writes the state.
func TestAProposalCarriesEveryFieldSection57Names(t *testing.T) {
	got, err := DecodeInRound(file, []byte(line()), round())
	require.NoError(t, err)
	require.Len(t, got, 1)

	assert.Equal(t, "x1", got[0].ID)
	assert.Equal(t, KindMutation, got[0].Kind)
	assert.Equal(t, "correctness", got[0].Role)
	assert.Equal(t, "u1", got[0].Unit)
	assert.Equal(t, "order.go:5", got[0].Target)
	assert.Equal(t, StateOpen, got[0].State,
		"§5.7's table computes state, and a stored proposal starts open")
	assert.Equal(t, []string{}, got[0].Paths,
		"a slice serialises as [] and never as null")
	assert.Empty(t, got[0].Probe)
}

// A proposal may name the record it would settle, and only one the round holds.
func TestAProposalMayNameTheRecordItWouldSettle(t *testing.T) {
	got, err := DecodeInRound(file, []byte(line("finding", `"f12"`)), round())
	require.NoError(t, err)
	require.Len(t, got, 1)
	assert.Equal(t, "f12", got[0].Finding)

	_, err = DecodeInRound(file, []byte(line("finding", `"f99"`)), round())
	require.Error(t, err)
	assert.Contains(t, err.Error(), "finding names \"f99\", which is no record of the current round")
}

// §5.7.1's refusals, each on a line the message names.
func TestSection571RefusesEveryLineItNames(t *testing.T) {
	cases := map[string]struct {
		body, contains string
	}{
		"a computed field": {
			line("state", `"run"`),
			"state is computed by cr per §5.7's table",
		},
		"a field of no row": {
			line("nonsense", `"x"`),
			"nonsense is not a field of §5.7's table",
		},
		"an id of the wrong spelling": {
			line("id", `"f1"`),
			`id reads "f1", and §5.7 spells a proposal id x<n>`,
		},
		"a kind outside the two": {
			line("kind", `"benchmark"`),
			`kind reads "benchmark", and §5.7's two kinds are mutation and gap`,
		},
		"a unit of no round": {
			line("unit", `"u9"`),
			`unit reads "u9", which is no unit of the current round`,
		},
		"a role that is not active": {
			line("role", `"security"`),
			`role reads "security", which is no active role of the current round`,
		},
		"a target outside the unit": {
			line("target", `"order.go:11"`),
			`which lies outside unit u1`,
		},
		"a target the head does not hold": {
			line("target", `"missing.go:1"`),
			"which does not resolve at the round's head",
		},
		"an empty hypothesis": {
			line("hypothesis", `"  "`),
			"hypothesis is required by §5.7's table and this line leaves it empty",
		},
		"an empty settles": {
			line("settles", `""`),
			"settles is required by §5.7's table and this line leaves it empty",
		},
		"an empty input": {
			line("input", `""`),
			"input is required by §5.7's table and this line leaves it empty",
		},
		"a supplied head": {
			line("head", `"cafe"`),
			"head",
		},
		"a supplied round": {
			line("round", `2`),
			"round",
		},
	}
	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			got, err := DecodeInRound(file, []byte(tc.body), round())
			require.Error(t, err, "§5.7.1 refuses this line")
			assert.Contains(t, err.Error(), tc.contains)
			assert.Empty(t, got, "§5.7.1 stores nothing when it refuses")
		})
	}
}

// A file naming one id twice is refused naming both lines, because the second
// would otherwise overwrite the first or sit beside it under one id.
func TestARepeatedProposalIDIsRefusedNamingBothLines(t *testing.T) {
	_, err := DecodeInRound(file, []byte(line()+line("target", `"order.go:6"`)), round())

	require.Error(t, err)
	assert.Contains(t, err.Error(), "proposals.ndjson line 2: id \"x1\" is the id of line 1")
}

// The id spellings §5.7 admits and refuses, which is what lets §5.7.2 tell a
// proposal from a record by spelling alone.
func TestAProposalIDIsSpelledXn(t *testing.T) {
	for _, valid := range []string{"x1", "x7", "x100"} {
		assert.True(t, ValidID(valid), valid)
	}
	for _, invalid := range []string{"", "x", "x0", "x01", "f1", "1", "xa", "p1", "X1"} {
		assert.False(t, ValidID(invalid), invalid)
	}

	n, ok := IDSuffix(IDOf(42))
	require.True(t, ok)
	assert.Equal(t, 42, n, "IDOf and IDSuffix are one reading of an id")
}
