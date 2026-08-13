package profile

import (
	"fmt"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// §2.4.3 requires a malformed profile to abort, but never defines malformed
// exhaustively. cr reads a non-positive tests.timeout_seconds or
// tests.output_tail_bytes as malformed, because neither has a reading under
// which the profile works: §5.2.3 and §5.6 assume a finite kill, so 0 is not
// "no timeout", and retaining no output empties the §8.1.7 evidence region that
// makes a probed assertion checkable by the author.
//
// 0 is the value that decides it. A test using -1 passes whether the bound is
// `<= 0` or `< 0`, so it proves nothing about the rule this profile is judged
// by — which is exactly what mutation testing caught here.
func TestNonPositiveTestBudgetsAreMalformed(t *testing.T) {
	for _, field := range []string{"timeout_seconds", "output_tail_bytes"} {
		for _, value := range []int{0, -1} {
			t.Run(fmt.Sprintf("%s is %d", field, value), func(t *testing.T) {
				path := write(t, "generic", fmt.Sprintf(`{
					"id": "generic",
					"match": {"files": [], "globs": ["**/*"]},
					"axes": {},
					"tests": {%q: %d}
				}`, field, value))

				_, err := Load(path)

				var malformed *MalformedError
				require.ErrorAs(t, err, &malformed)
				assert.Equal(t, "tests."+field, malformed.Field)
				assert.Contains(t, err.Error(), fmt.Sprintf("%d", value))
			})
		}
	}
}

// §2.4 gives tests.count_pattern's arity fault its own words but says nothing
// about a pattern that never compiles. cr treats the two alike: a pattern cr
// cannot compile has no group count to check, so it cannot be shown to satisfy
// the arity §2.4 demands, and §5.2.1 could never run it. §2.6.1.2 already
// settles the shape for cr's other configured regex — a pattern that fails to
// compile aborts with exit code 3, naming what it came from — and a profile
// answering the same fault differently would be the anomaly.
//
// The two faults stay distinguishable in the message, because the fix differs:
// one is a typo in the expression, the other a miscount of its groups.
func TestAnUncompilableCountPatternIsMalformedToo(t *testing.T) {
	path := write(t, "generic", `{
		"id": "generic",
		"match": {"files": [], "globs": ["**/*"]},
		"axes": {"test": true},
		"tests": {"count_pattern": "(\\d+) passed, (\\d+ failed"}
	}`)

	_, err := Load(path)

	var malformed *MalformedError
	require.ErrorAs(t, err, &malformed)
	assert.Equal(t, "tests.count_pattern", malformed.Field)
	assert.Contains(t, err.Error(), path)
	// Not the arity message: an uncompilable pattern has no group count.
	assert.NotContains(t, err.Error(), "capture groups")
}

// A profile cr cannot read or parse has no offending field to name, so its
// message must not leave a gap where one would go. Nothing else distinguishes
// the two forms of MalformedError.Error, so nothing else notices if they merge.
func TestMalformedErrorNamesAFieldOnlyWhenItHasOne(t *testing.T) {
	withField := &MalformedError{File: "p.json", Field: "axes", Problem: "is required"}
	assert.Equal(t, "p.json: axes is required", withField.Error())

	withoutField := &MalformedError{File: "p.json", Problem: "is not valid JSON"}
	assert.Equal(t, "p.json: is not valid JSON", withoutField.Error())
}
