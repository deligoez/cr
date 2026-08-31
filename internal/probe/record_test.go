package probe

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

// §5.5's id space, allocated over every record probes.ndjson holds.
//
// It is read from the whole file rather than the current round's records, and
// the two ids below say why: §5.5.2 has a finding reference a probe by this id
// and §5.5.3 keeps records from earlier heads on disk, so an id an earlier round
// spent still names the probe a stored finding points at. Reusing it would
// silently repoint that finding at a different experiment.
func TestTheNextProbeIDIsAllocatedAboveEveryOneOnFile(t *testing.T) {
	for name, tc := range map[string]struct {
		existing []Record
		want     string
	}{
		"an empty file starts at one": {
			existing: nil,
			want:     "p1",
		},
		"the highest is taken, not the last": {
			existing: []Record{{ID: "p1"}, {ID: "p7"}, {ID: "p3"}},
			want:     "p8",
		},
		"an id from an earlier round is still spent": {
			existing: []Record{{ID: "p4"}},
			want:     "p5",
		},
		"an id cr did not write contributes nothing": {
			existing: []Record{{ID: "r2"}, {ID: "p"}, {ID: "p0"}, {ID: "p-1"}, {ID: "p01"}, {ID: ""}},
			want:     "p1",
		},
		"a run id is not a probe id": {
			existing: []Record{{ID: "r9"}},
			want:     "p1",
		},
	} {
		t.Run(name, func(t *testing.T) {
			assert.Equal(t, tc.want, NextID(tc.existing))
		})
	}
}
