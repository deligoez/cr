package render

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/deligoez/cr/internal/finding"
)

// lens is one §4.5.4 entry as its own package renders it, which is all
// ReviewBody knows about any of the four kinds.
type lens string

func (l lens) Disclosure() string { return string(l) }

// §8.4.3's two halves, in the order the section fixes: the disclosure of
// §4.5.4, and beneath it the payload hash as an HTML comment.
//
// The body is asserted whole, which asserts the order too: a body with the
// comment above the disclosure still holds both strings, and the author reading
// it would meet cr's bookkeeping before cr's statement about what it examined.
func TestTheReviewBodyCarriesTheDisclosureAboveTheHash(t *testing.T) {
	body := ReviewBody(
		[]string{"correctness", "convention"},
		[]finding.HonestyDisclosure{
			lens("axis test disabled, per §4.5.2: the profile declares no tests.cmd"),
			lens("lens convention/reinvention unavailable, per §4.5.4: no symbol index"),
			lens("role security skipped, per §4.6.4: its axis did not run"),
		},
		"420012ebffc2b992")

	assert.Equal(t, strings.Join([]string{
		"**cr — review coverage**",
		"",
		"Axes reviewed: correctness, convention",
		"",
		"Lenses that did not run, and why:",
		"- axis test disabled, per §4.5.2: the profile declares no tests.cmd",
		"- lens convention/reinvention unavailable, per §4.5.4: no symbol index",
		"- role security skipped, per §4.6.4: its axis did not run",
	}, "\n")+regionSeparator+PayloadHashComment("420012ebffc2b992"), body)
}

// The hash written and the hash read back are one value, which is what §8.4.4
// rests on: it matches the embedded hash to decide between adopting a posted
// review and posting a second one, and a disagreement between the two spellings
// would turn that guard into a duplicate post.
func TestThePayloadHashReadsBackOutOfTheBodyItWasWrittenInto(t *testing.T) {
	body := ReviewBody([]string{"correctness"}, nil, "420012ebffc2b992")

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
	assert.Equal(t, strings.Join([]string{
		"**cr — review coverage**",
		"",
		"No axis ran this round.",
		"",
		"No lens was left unexamined.",
	}, "\n")+regionSeparator+PayloadHashComment("420012ebffc2b992"),
		ReviewBody(nil, nil, "420012ebffc2b992"))
}
