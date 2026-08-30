package render

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// codesOf renders a set of languages as the codes render.lang writes, so a
// whole enumeration can be asserted at once and in order.
func codesOf(set []Lang) []string {
	codes := make([]string, 0, len(set))
	for _, lang := range set {
		codes = append(codes, lang.String())
	}
	return codes
}

// §8.1.1 configures the language of every author-facing body and defaults it to
// tr; §8.1.4 has cr build the question label in per language and forbids
// configuring it. The two together close the domain, which is round 8's finding
// render-lang-domain-unbounded: a free-form value admits a language cr has no
// built-in label for, and §6.3's forcing — which reaches the reader through that
// line alone — would then reach them through nothing at all.
//
// The enumeration is asserted whole and in order rather than membership by
// membership, so a third language cannot be added without this test being read,
// and the label table is what it then has to satisfy.
func TestV01RendersInExactlyTurkishAndEnglish(t *testing.T) {
	assert.Equal(t, []string{"tr", "en"}, codesOf(Langs()),
		"§8.1.1 defaults render.lang to tr, so tr leads the set")

	for _, lang := range Langs() {
		parsed, err := ParseLang(lang.String())
		require.NoErrorf(t, err, "%s is enumerated and must parse", lang)
		assert.Equal(t, lang, parsed, "ParseLang is the only door in, so it must round-trip")
		assert.True(t, parsed.Valid())
	}

	_, err := ParseLang("de")
	var unknown *UnknownLangError
	require.ErrorAs(t, err, &unknown, "a language outside the two is refused")
	assert.Equal(t, "de", unknown.Value, "the refusal carries what was rejected")
	assert.Contains(t, unknown.Error(), Setting,
		"§12.4: the refusal names the setting the user has to edit")
	assert.Contains(t, unknown.Error(), "tr and en", "and the two values it may hold")

	assert.False(t, Lang{}.Valid(),
		"a value nothing parsed names no language, and no label is built in for it")

	widened := Langs()
	widened[0] = LangEN
	assert.Equal(t, []string{"tr", "en"}, codesOf(Langs()),
		"the set handed out is a copy: a caller can neither widen it nor reorder it")
}
