package glob

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

// `**` is the reason this package exists rather than a call to path.Match, so
// it is what the table spends most of its rows on. `tests/**/*Test.php` is the
// glob the shipped laravel-pest profile actually writes, and the three depths
// below are the three ways a Laravel suite lays a test out.
func TestMatchTreatsDoubleStarAsAnyRunOfSegments(t *testing.T) {
	for _, c := range []struct {
		pattern, target string
		matched         bool
	}{
		{pattern: "tests/**/*Test.php", target: "tests/OrderTest.php", matched: true},
		{pattern: "tests/**/*Test.php", target: "tests/Feature/OrderTest.php", matched: true},
		{pattern: "tests/**/*Test.php", target: "tests/Feature/Api/OrderTest.php", matched: true},
		{pattern: "tests/**/*Test.php", target: "app/Models/Order.php", matched: false},
		{pattern: "tests/**/*Test.php", target: "tests/Order.php", matched: false},
		{pattern: "app/**", target: "app/Models/Order.php", matched: true},
		{pattern: "app/**", target: "app", matched: true},
		{pattern: "app/**", target: "apple/Order.php", matched: false},
		{pattern: "**", target: "a/b/c/d.php", matched: true},

		// A single star stays inside one segment, which is what makes
		// `**` a different thing rather than a longer spelling.
		{pattern: "app/*.php", target: "app/Order.php", matched: true},
		{pattern: "app/*.php", target: "app/Models/Order.php", matched: false},

		// A pattern with more segments than the path, and the reverse.
		{pattern: "app/Models/Order.php", target: "app/Models", matched: false},
		{pattern: "app/Models", target: "app/Models/Order.php", matched: false},
	} {
		t.Run(c.pattern+" vs "+c.target, func(t *testing.T) {
			assert.Equal(t, c.matched, Match(c.pattern, c.target))
		})
	}
}

// A pattern path.Match cannot compile matches nothing rather than panicking or
// matching everything. Neither §2.4 nor §2.6 gives its globs a syntax rule, so
// the matcher invents no rejection — and of the two ways it could go wrong,
// matching nothing is the one that attaches no file and enforces no rule cr
// never established belonged.
func TestAnUncompilablePatternMatchesNothing(t *testing.T) {
	assert.False(t, Match("tests/[Order", "tests/[Order"))
	assert.False(t, Match("tests/[Order", "tests/OrderTest.php"))
}

// MatchAny over no pattern is false, and each caller gives that its own
// meaning: §2.4's absent `tests.globs` recognises no test file, while §2.6's
// empty `globs` means all source and is therefore asked the other way round.
func TestMatchAnyOverNoPatternIsFalse(t *testing.T) {
	assert.False(t, MatchAny(nil, "app/Models/Order.php"))
	assert.False(t, MatchAny([]string{}, "app/Models/Order.php"))
	assert.True(t, MatchAny([]string{"database/**", "app/**"}, "app/Models/Order.php"))
	assert.False(t, MatchAny([]string{"database/**", "config/**"}, "app/Models/Order.php"))
}
