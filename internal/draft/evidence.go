package draft

import (
	"cmp"
	"fmt"
	"slices"

	"github.com/deligoez/cr/internal/finding"
	"github.com/deligoez/cr/internal/probe"
	"github.com/deligoez/cr/internal/render"
)

// MissingProbeError refuses a `probed` record whose probe probes.ndjson does
// not hold.
//
// §6.2 grades a record `probed` only on a probe record it resolved, and §5.5.1
// makes probe records immutable, so the pair can only come apart in state cr
// did not write. The record would otherwise reach the author in the assertion
// register with nothing beneath it to check, which is the one outcome §8.1.7's
// region exists to prevent — so the draft stops, naming both ids, instead.
type MissingProbeError struct {
	// Record is the id of the record graded `probed`.
	Record string
	// Probe is the probe id it names.
	Probe string
}

func (e *MissingProbeError) Error() string {
	return fmt.Sprintf(
		"record %s is graded probed on probe %q, which probes.ndjson does not hold: "+
			"§8.1.7's evidence region cannot be rendered for it; record the round again with `cr record`",
		e.Record, e.Probe)
}

// evidence is §8.1.7's region for one record, asked of its grade: a `cited`
// record lists its citations, a `probed` one carries its probe, and an
// `argued` one — which asserts nothing, since §6.3 makes it a question —
// carries only the re-runs of the probe it names, when there are any.
func (p *Provenances) evidence(record *finding.Finding, lang render.Lang) (string, error) {
	reruns := p.reruns(record.Probe)
	switch record.Grade {
	case finding.GradeCited:
		return render.CitedEvidence(lang, record.ID, record.Citations, reruns)
	case finding.GradeProbed:
		var held *probe.Record
		if p != nil {
			held = p.Probes[record.Probe]
		}
		if held == nil {
			return "", &MissingProbeError{Record: record.ID, Probe: record.Probe}
		}
		return render.ProbeEvidence(lang, record.ID, held, p.MaxProbeInput, p.CountPattern, reruns)
	}
	return render.RerunEvidence(lang, record.ID, reruns)
}

// reruns are the probes at the round's head whose `rerun_of` names the probe
// id, in the order of their ids, and none when id is empty.
func (p *Provenances) reruns(id string) []*probe.Record {
	found := make([]*probe.Record, 0)
	if p == nil || id == "" {
		return found
	}
	for _, held := range p.Probes {
		if held.RerunOf == id && held.Head == p.Head {
			found = append(found, held)
		}
	}
	slices.SortFunc(found, func(a, b *probe.Record) int {
		return cmp.Or(cmp.Compare(len(a.ID), len(b.ID)), cmp.Compare(a.ID, b.ID))
	})
	return found
}
