package cli

import (
	"fmt"

	"github.com/deligoez/cr/internal/finding"
	"github.com/deligoez/cr/internal/state"
)

// readProbedEvidence reads the round's evidence and refuses, before anything
// grades from it, a record whose `probe` names nothing that evidence holds at
// this head.
func readProbedEvidence(
	l state.Layout, owner, repo string, pr int, head, file string, body []byte, records []*finding.Finding,
) (*roundEvidence, error) {
	evidence, err := readRoundEvidence(l, owner, repo, pr)
	if err != nil {
		return nil, err
	}
	if err := refuseAbsentProbes(file, body, head, evidence, records); err != nil {
		return nil, err
	}
	return evidence, nil
}

// refuseAbsentProbes is §6.2.2's rejection: a record whose `probe` names no
// probe record the pull request holds, or one recorded at another head, is
// refused with exit code 1, naming the line and the field.
//
// The line is drawn between evidence that does not exist and evidence that
// does not support. A probe that exists at this head and supports nothing — a
// §5.4.5 result, a failed mutation per §5.3.7, a target outside the record's
// anchor range, a LEFT anchor — is kept and graded: §5.4.4 and §5.4.5 leave
// such a record argued and ask it as a question, and refusing it would make
// deleting the reference to a negative result the cheapest repair. What is
// refused here is a reference to an experiment no record of this head holds,
// which grades nothing whatever else the record says. The reading follows the
// suggestion round 7's reject-vs-downgrade-conflict made.
func refuseAbsentProbes(
	file string, body []byte, head string, found *roundEvidence, records []*finding.Finding,
) error {
	at := state.RecordLines(body)
	for i, record := range records {
		if record.Probe == "" {
			continue
		}
		referenced := found.referenced(record.Probe)
		switch {
		case referenced == nil:
			return &finding.RejectedRecordError{
				File: file, Line: at[i], Field: "probe",
				Problem: fmt.Sprintf("%q names no probe record this pull request holds; §6.2.2 refuses a record "+
					"resting on an experiment that does not exist, so run the probe or drop the reference",
					record.Probe),
			}
		case !referenced.Grades(head):
			return &finding.RejectedRecordError{
				File: file, Line: at[i], Field: "probe",
				Problem: fmt.Sprintf("%q was recorded at head %s and this round's head is %s; §5.5.3 lets no probe "+
					"from another head grade this round and §6.2.2 refuses the reference, so run the probe again at this head",
					record.Probe, referenced.Head, head),
			}
		}
	}
	return nil
}
