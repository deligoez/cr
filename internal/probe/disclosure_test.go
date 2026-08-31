// This file is the package's one external test, and the import is why. §5.4.4's
// severity bounds are enforced where the record lives, so internal/finding
// reads internal/probe; a test in `package probe` that imported finding to
// assert the contract would close that loop. The assertion is the same one
// intent.Unavailable and testadequacy.Unavailable carry, made from outside.
package probe_test

import (
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/deligoez/cr/internal/finding"
	"github.com/deligoez/cr/internal/probe"
)

// §4.5.4 requires a lens that could not look to be reported with its reason,
// and §11.1 exempts that report from `--quiet`. finding.HonestyDisclosure is
// the channel, and implementing it is what lets round 12's
// axis-availability-coupling reach a reader through it rather than through a
// message a flag can silence.
//
// The compile-time assertion comes first because that is when the contract can
// still be repaired: a disclosure that stopped satisfying it would otherwise be
// found by the call site that no longer collects it, which is a report going
// quiet rather than a build going red.
//
// The sentence is then held to three things. It names every probe the coupling
// reaches, because the experiment is what the reader is holding. It names the
// three sections that produce the coupling, because no one of them says it. And
// it names what would lift it, because §4.5.4 asks for the reason a lens did
// not look and a reason with no remedy leaves the reader where they started.
func TestTheUnmappableGapReportIsAnHonestyDisclosure(t *testing.T) {
	var _ finding.HonestyDisclosure = probe.GapUnmappable{}

	disclosed := probe.GapUnmappable{Probes: []string{"p1", "p3"}}.Disclosure()

	assert.Contains(t, disclosed, "p1")
	assert.Contains(t, disclosed, "p3")
	assert.Contains(t, disclosed, "§4.5.3", "the intent axis is unavailable")
	assert.Contains(t, disclosed, "§4.6.6", "so the mapping is empty")
	assert.Contains(t, disclosed, "§5.4.4", "so the support condition cannot be met")
	assert.Contains(t, disclosed, "--issue", "and this is what would make it available")
}
