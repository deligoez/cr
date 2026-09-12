package render

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/deligoez/cr/internal/finding"
)

// lens is one §4.5.4 entry as its own package renders it, which is all
// ReviewBody knows about any of the four kinds.
type lens string

func (l lens) Disclosure() string { return string(l) }

// §8.4.3's two halves, in the order the section fixes: the disclosure of
// §4.5.4, and beneath it the payload hash as an HTML comment.
//
// The order is asserted by position rather than by presence, because presence
// is the half that cannot go wrong silently. A body with the comment above the
// disclosure still holds both strings, and the author reading it would meet
// cr's bookkeeping before cr's statement about what it examined.
func TestTheReviewBodyCarriesTheDisclosureAboveTheHash(t *testing.T) {
	body, err := ReviewBody(LangEN,
		[]string{"correctness", "convention"},
		[]finding.HonestyDisclosure{
			lens("axis test disabled, per §4.5.2: the profile declares no tests.cmd"),
			lens("lens convention/reinvention unavailable, per §4.5.4: no symbol index"),
			lens("role security skipped, per §4.6.4: its axis did not run"),
		},
		"420012ebffc2b992")
	require.NoError(t, err)

	assert.Contains(t, body, "Axes reviewed: correctness, convention")
	for _, entry := range []string{
		"- axis test disabled, per §4.5.2: the profile declares no tests.cmd",
		"- lens convention/reinvention unavailable, per §4.5.4: no symbol index",
		"- role security skipped, per §4.6.4: its axis did not run",
	} {
		assert.Contains(t, body, entry)
	}
	comment := PayloadHashComment("420012ebffc2b992")
	assert.Contains(t, body, comment)
	assert.Less(t, strings.Index(body, "Lenses that did not run"), strings.Index(body, comment),
		"§8.4.3 puts the disclosure above the hash comment")
	assert.True(t, strings.HasSuffix(body, comment),
		"and the comment is the last thing in the body, so nothing of cr's follows it")
}

// The hash written and the hash read back are one value, which is what §8.4.4
// rests on: it matches the embedded hash to decide between adopting a posted
// review and posting a second one, and a disagreement between the two spellings
// would turn that guard into a duplicate post.
func TestThePayloadHashReadsBackOutOfTheBodyItWasWrittenInto(t *testing.T) {
	body, err := ReviewBody(LangTR, []string{"correctness"}, nil, "420012ebffc2b992")
	require.NoError(t, err)

	read, found := PayloadHashIn(body)
	assert.True(t, found)
	assert.Equal(t, "420012ebffc2b992", read)

	assert.True(t, strings.HasPrefix(PayloadHashComment("x"), Reserved),
		"§8.1.3 refuses an agent body carrying the reserved sequence, which is what stops one counterfeiting this line")

	for _, body := range []string{"", "no marker here", Reserved + "payload-hash 420012eb"} {
		_, found := PayloadHashIn(body)
		assert.False(t, found, "%q", body)
	}
}

// A round where every lens looked says so, rather than leaving the reader to
// read an absent list as an absent obligation. The same for a round where no
// axis ran at all: §4.5.4 is about what cr did not do, and the one thing that
// cannot convey it is silence.
func TestAnEmptyDisclosureIsAStatementRatherThanAnAbsence(t *testing.T) {
	for _, lang := range Langs() {
		body, err := ReviewBody(lang, nil, nil, "420012ebffc2b992")
		require.NoError(t, err)

		text, known := reviewBodyOf(lang)
		require.True(t, known)
		assert.Contains(t, body, text.noAxes)
		assert.Contains(t, body, text.noLenses)
		assert.NotContains(t, body, text.axes)
		assert.NotContains(t, body, text.lenses)
	}
}

// Every language of the enumeration has a row, and a Lang outside it is
// refused rather than rendered through a default — the same fence §8.1.4's
// label table stands behind, for the same reason: a body the author cannot
// read is a disclosure that did not reach them.
func TestEveryLanguageCarriesABuiltInReviewBodyFraming(t *testing.T) {
	for _, lang := range Langs() {
		text, known := reviewBodyOf(lang)
		require.Truef(t, known, "%s has no built-in framing", lang)
		for _, line := range []string{text.heading, text.axes, text.noAxes, text.lenses, text.noLenses} {
			assert.NotEmptyf(t, line, "%s leaves a line of the framing empty", lang)
		}
	}

	_, err := ReviewBody(Lang{}, nil, nil, "420012ebffc2b992")
	var unknown *UnknownLangError
	require.ErrorAs(t, err, &unknown)
}
