package rule

import (
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// MalformedError has two sentence shapes and a documented rule for choosing
// between them: Field is "empty when the fault is the file as a whole and no
// single field can be blamed for it". Both shapes were built by the same
// branch and neither was asserted, so the branch could be inverted and every
// test in this package still passed — found as a surviving CONDITIONALS_NEGATION
// mutant on it.
//
// It is worth an assertion rather than a shrug because the message is the whole
// of what a user gets: §2.6.5 aborts, and the sentence is the only instruction
// they have. The field-less shape read with the field-carrying format would
// print the file twice with the problem in the field's place, and the reverse
// would drop the field a user needs to find the line.
func TestTheMessageNamesTheFieldOnlyWhenThereIsOne(t *testing.T) {
	whole := &MalformedError{File: "rules/no-raw-sql.json", Problem: "is not valid JSON"}
	assert.Equal(t, "rules/no-raw-sql.json: is not valid JSON", whole.Error())

	blamed := &MalformedError{
		File: "rules/no-raw-sql.json", Field: "class", Problem: "is required",
	}
	assert.Equal(t, "rules/no-raw-sql.json: class is required", blamed.Error())
}

// The field-less shape is reachable, not hypothetical, and a profile's `rules`
// array is where it turns up: every element of a valid JSON array is valid
// JSON, but nothing makes one an object. An element that is a string or a
// number fails to decode with no field to blame.
//
// locate then adds the array position and nothing else. Its own comment claims
// the sentence never trails a bare dot, which is exactly the claim the branch
// above decides — so this asserts the two together rather than trusting either
// to describe the other.
func TestAProfileRuleThatIsNotAnObjectIsBlamedOnItsPositionAlone(t *testing.T) {
	for _, c := range []struct{ name, element string }{
		{name: "a string", element: `"no-raw-sql"`},
		{name: "a number", element: `42`},
		{name: "an array", element: `["no-raw-sql"]`},
	} {
		t.Run(c.name, func(t *testing.T) {
			_, err := Resolve(t.TempDir(), t.TempDir(), profileFile, []json.RawMessage{
				json.RawMessage(mustMarshal(t, ruleDoc("handle-every-error", nil))),
				json.RawMessage(c.element),
			})

			var malformed *MalformedError
			require.ErrorAs(t, err, &malformed)
			assert.Equal(t, "rules[1]", malformed.Field,
				"there is no field to blame, so the position stands alone")
			assert.Contains(t, err.Error(), "profiles/laravel-pest.json: rules[1] ",
				"the sentence names the position and never trails a bare dot")
			assert.NotContains(t, err.Error(), "rules[1].")
		})
	}
}

// mustMarshal writes a rule document as the bytes a profile carries it in.
func mustMarshal(t *testing.T, doc map[string]any) []byte {
	t.Helper()
	data, err := json.Marshal(doc)
	require.NoError(t, err)
	return data
}
