package rule

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// §2.6 keeps rules and roles apart: a role is a lens and supplies prose, a rule
// is a specific standard and is data. A rule file is project-owned and
// hand-edited, so the fault worth defending against is not a typo but a rule
// that tries to be something else — a `prompt` or `instructions` beside its
// `rationale`, a `grade` or `state` §6.1.4 reserves for cr, an `output_path`
// beside the one §4.6.2 names.
//
// Ignoring such a key is the wrong answer twice over. It leaves the author
// believing the key took effect, and for a rule that means believing a standard
// is enforced while nothing enforces it. And it makes the boundary a convention
// rather than a fence: with unknown keys tolerated, no reader can tell a
// supported field from a decorative one. So an unknown top-level key aborts on
// §2.6.5's code, naming the key.
//
// TestARuleCarriesExactlyTheSpecFields is the other half. That one fixes what
// the struct may hold; this one fixes what a file may say. Neither implies the
// other: a struct held to the table still decodes an unknown key into nothing,
// and an allowlist over a struct that grew a field would let it through.
func TestARuleFileCannotCarryAKeyCrDoesNotDefine(t *testing.T) {
	for _, key := range []string{
		"instructions", "focus", "prompt", "output_path", "schema", "grade", "state", "probe",
	} {
		t.Run(key, func(t *testing.T) {
			path := ruleFile(t, "handle-every-error", map[string]any{key: "mine"})

			_, err := Load(path)

			var malformed *MalformedError
			require.ErrorAs(t, err, &malformed)
			assert.Equal(t, key, malformed.Field)
			assert.Contains(t, err.Error(), path)
			assert.Contains(t, err.Error(), "is not a rule field")
		})
	}

	// A file carrying two unknown keys must report the same one on every run.
	// The map that finds them iterates in no order, so without the sort this
	// would pass most of the time and fail the rest — the shape of flake that
	// gets rerun rather than read.
	t.Run("two unknown keys report the same one every run", func(t *testing.T) {
		path := ruleFile(t, "handle-every-error", map[string]any{"zeta": "1", "alpha": "2"})

		for range 20 {
			_, err := Load(path)

			var malformed *MalformedError
			require.ErrorAs(t, err, &malformed)
			assert.Equal(t, "alpha", malformed.Field)
		}
	})
}

// The two objects §2.6's table points at are closed too. §2.6.1 names
// `detect.pattern` and `detect.mode` and nothing else; §2.6.2 names
// `fix.replace` and `fix.with` and nothing else.
//
// The inner sets are where a tolerated key would do the most damage, because a
// detect block is the half of a rule that runs. A `detect.exclude` or a
// `fix.apply` decoded into nothing would leave its author believing the rule
// skips a path, or — worse, against §2.6.2.3 — that cr edits a file for them.
// The rejected key is reported under its full path, so the message names the
// line the user has to open.
func TestTheDetectAndFixBlocksCannotCarryAKeyCrDoesNotDefine(t *testing.T) {
	for _, c := range []struct{ block, key string }{
		{"detect", "exclude"},
		{"detect", "flags"},
		{"detect", "replace"},
		{"fix", "apply"},
		{"fix", "pattern"},
	} {
		t.Run(c.block+"."+c.key, func(t *testing.T) {
			path := ruleFile(t, "handle-every-error", map[string]any{
				c.block: map[string]any{c.key: "x"},
			})

			_, err := Load(path)

			var malformed *MalformedError
			require.ErrorAs(t, err, &malformed)
			assert.Equal(t, c.block+"."+c.key, malformed.Field)
			assert.Contains(t, err.Error(), "is not a "+c.block+" field")
		})
	}
}

// The allowlists are the message. A check that read one list and printed
// another could refuse a key while naming a set that contains it, which is the
// answer a user cannot act on. Holding the message to the same literal keeps
// the refusal and the remedy in one place.
func TestTheRefusalNamesTheSetItCheckedAgainst(t *testing.T) {
	t.Run("rule", func(t *testing.T) {
		_, err := Load(ruleFile(t, "handle-every-error", map[string]any{"prompt": "x"}))

		require.Error(t, err)
		for _, field := range fields {
			assert.Contains(t, err.Error(), field)
		}
	})

	t.Run("detect", func(t *testing.T) {
		_, err := Load(ruleFile(t, "handle-every-error", map[string]any{
			"detect": map[string]any{"exclude": "x"},
		}))

		require.Error(t, err)
		for _, field := range detectFields {
			assert.Contains(t, err.Error(), "detect."+field)
		}
	})

	t.Run("fix", func(t *testing.T) {
		_, err := Load(ruleFile(t, "handle-every-error", map[string]any{
			"fix": map[string]any{"apply": true},
		}))

		require.Error(t, err)
		for _, field := range fixFields {
			assert.Contains(t, err.Error(), "fix."+field)
		}
	})
}
