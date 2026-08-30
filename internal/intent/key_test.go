package intent

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/deligoez/cr/internal/config"
)

// specDefaultPattern is §3.2's default, written out once so a test can use it
// without deciding on its own what the default is. TestTheKeyPatternDefaultIsTheOneTheSpecNames
// ties it to the configuration table that actually supplies it.
const specDefaultPattern = `[A-Z][A-Z0-9]+-[0-9]+`

// §3.2 orders four sources and resolves from the first that yields a match, so
// each of the four has to be shown winning. The earlier sources are present and
// keyless rather than absent in every case: a source that is simply empty
// cannot tell a working fall-through from one that skipped it.
func TestEachKeySourceWinsInTurn(t *testing.T) {
	for _, tc := range []struct {
		name    string
		sources KeySources
		want    Key
	}{
		{
			name:    "the flag",
			sources: KeySources{Flag: "CR-1", Branch: "feature/add-a-thing", Title: "add a thing", Body: "no key in here"},
			want:    Key{Value: "CR-1", Origin: KeyFromFlag},
		},
		{
			name:    "the branch",
			sources: KeySources{Branch: "feature/CR-2-add-a-thing", Title: "add a thing", Body: "no key in here"},
			want:    Key{Value: "CR-2", Origin: KeyFromBranch},
		},
		{
			name:    "the title",
			sources: KeySources{Branch: "feature/add-a-thing", Title: "CR-3: add a thing", Body: "no key in here"},
			want:    Key{Value: "CR-3", Origin: KeyFromTitle},
		},
		{
			name:    "the body",
			sources: KeySources{Branch: "feature/add-a-thing", Title: "add a thing", Body: "Closes CR-4, at last."},
			want:    Key{Value: "CR-4", Origin: KeyFromBody},
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			key, err := ResolveKey(tc.sources, specDefaultPattern)

			require.NoError(t, err)
			assert.Equal(t, tc.want, key)
		})
	}
}

// §3.2's order is normative, not a preference, and the case that decides it is
// the one where obeying it looks wrong: a stray key in the branch beats the
// ticket named in the title and the body together. Choosing the better-looking
// match would be cr forming an opinion about which key is meant, which P5 does
// not allow it to do — the pattern and the order are the whole of the rule.
func TestAnEarlierSourceWinsEvenWhenALaterOneReadsBetter(t *testing.T) {
	key, err := ResolveKey(KeySources{
		Branch: "feature/AB-1-spike",
		Title:  "CR-123: the ticket this pull request is actually for",
		Body:   "Closes CR-123.",
	}, specDefaultPattern)

	require.NoError(t, err)
	assert.Equal(t, Key{Value: "AB-1", Origin: KeyFromBranch}, key)
}

// §3.2's default and the resolver that runs it are one claim, the way §3.1.2's
// argv and its reader are: a default edited in the table alone would resolve
// perfectly well and go wrong only where a key is looked for. The branch below
// is the shape the default exists for — the pattern is unanchored because a
// branch is a path and a title is a sentence, and an anchored one would find a
// key in neither.
func TestTheKeyPatternDefaultIsTheOneTheSpecNames(t *testing.T) {
	defaults, err := config.Resolve(config.Sources{})
	require.NoError(t, err)

	pattern := defaults.String("intent.key_pattern")
	assert.Equal(t, specDefaultPattern, pattern)

	key, err := ResolveKey(KeySources{Branch: "feature/CR-123-add-a-thing"}, pattern)
	require.NoError(t, err)
	assert.Equal(t, Key{Value: "CR-123", Origin: KeyFromBranch}, key)
}

