package cli

import (
	"github.com/deligoez/cr/internal/finding"
	"github.com/deligoez/cr/internal/state"
)

// sentRecord finds one record of the pull request by id, in whichever round
// holds it.
//
// §9.5 and §9.6 act on a record after it reached GitHub, and a posted record
// stays in the round that posted it: §9.3.4's sweep leaves it alone, and
// nothing copies it into the round a moved head opens. A read scoped to the
// current round would therefore lose every posted concern the moment the
// author pushed — which is the moment these commands exist for — so §9.3.5
// exempts them, and the read is declared in crossRoundReaders. An id is stable
// for the life of the pull request (§6.1), so one id names one line.
//
// It takes no lock, per §2.3.2; finding.MoveSent checks the state again under
// the lock before it writes.
func sentRecord(l state.Layout, owner, repo string, pr int, id string) (*finding.Finding, error) {
	stored, err := state.ReadRecords[finding.Finding](l, owner, repo, pr, state.FileFindings)
	if err != nil {
		return nil, err
	}
	for i := range stored {
		if stored[i].ID == id {
			return &stored[i], nil
		}
	}
	return nil, &unknownRecordError{ID: id}
}

// postedConcerns are the pull request's records still in `posted`, from every
// round, in file order: what §9.5.2 reports on, for sentRecord's reason.
func postedConcerns(l state.Layout, owner, repo string, pr int) ([]*finding.Finding, error) {
	stored, err := state.ReadRecords[finding.Finding](l, owner, repo, pr, state.FileFindings)
	if err != nil {
		return nil, err
	}
	posted := make([]*finding.Finding, 0)
	for i := range stored {
		if stored[i].State == finding.StatePosted {
			posted = append(posted, &stored[i])
		}
	}
	return posted, nil
}
