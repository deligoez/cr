package cli

import (
	"fmt"

	"github.com/deligoez/cr/internal/finding"
	"github.com/deligoez/cr/internal/git"
	"github.com/deligoez/cr/internal/probe"
	"github.com/deligoez/cr/internal/proposal"
	"github.com/deligoez/cr/internal/state"
)

// UnknownProposalError reports a `--proposal` naming no proposal the pull
// request holds. The cli layer maps it onto §11.2's code 1: the command line
// named something that is not there, and the id is the agent's to correct.
type UnknownProposalError struct {
	// ID is the id that was given.
	ID string
}

func (e *UnknownProposalError) Error() string {
	return fmt.Sprintf("--proposal %q names no proposal this pull request holds; "+
		"§5.7.1 stores one per line of the file `cr proposals record` read, and `cr status` lists "+
		"the round's open proposals", e.ID)
}

// StaleProposalError reports a proposal of another round or another head, which
// §5.7.3 refuses with exit code 4.
//
// It is a state conflict rather than bad input, for the reason a moved head is:
// the file parsed, the id exists, and what refuses is where the proposal
// stands. §5.5.3 says the same from the probe's end — a record produced against
// another head grades nothing here — so running it would write a probe the
// round could not use.
type StaleProposalError struct {
	// ID is the proposal.
	ID string
	// Round and Head are the proposal's; At and On are the round's.
	Round, At    int
	Head, OnHead string
}

func (e *StaleProposalError) Error() string {
	if e.Round != e.At {
		return fmt.Sprintf("proposal %s was proposed in round %d and this pull request is in round %d; "+
			"§5.7.3 runs a proposal of the current round alone", e.ID, e.Round, e.At)
	}
	return fmt.Sprintf("proposal %s was proposed at head %s and the round is at head %s; "+
		"§5.7.3 runs a proposal of the current head alone", e.ID, e.Head, e.OnHead)
}

// ProposalSpentError reports a proposal `cr probe run --proposal` has already
// executed, which carries the probe it produced.
//
// §5.5.1 makes a probe record immutable and §5.7.4 has the execution write the
// proposal's `probe` once; running it again would write a second probe for one
// ask and leave the proposal naming whichever finished last.
type ProposalSpentError struct {
	// ID is the proposal, and Probe the probe it produced.
	ID, Probe string
}

func (e *ProposalSpentError) Error() string {
	return fmt.Sprintf("proposal %s was already run and produced probe %s; "+
		"§5.7.4 writes a proposal's probe once, so run a proposal that is still open",
		e.ID, e.Probe)
}

// UnrunnableProposalError reports a proposal §5.7.5 stored `unrunnable`, which
// carries the reason cr cannot execute it.
//
// It is a state conflict for ProposalSpentError's reason: the command line is
// well-formed and names a proposal that exists, and what refuses is where that
// proposal stands.
type UnrunnableProposalError struct {
	// ID is the proposal, and Reason the one §5.7.5 stored with it.
	ID, Reason string
}

func (e *UnrunnableProposalError) Error() string {
	return fmt.Sprintf("proposal %s is unrunnable: %s", e.ID, e.Reason)
}

// proposedRun is §5.7.3's whole invocation: no input flag beside `--proposal`,
// the stored proposal loaded into the request, and the kind it names returned
// for the switch that runs it.
func proposedRun(
	owner, repo string, pr int, id string, request *probeRequest,
	kind, patchFile, testFile, target, filter string, paths []string,
) (string, error) {
	if err := onlyTheProposal(kind, patchFile, testFile, target, filter, paths); err != nil {
		return "", err
	}
	proposed, err := loadProposal(owner, repo, pr, id, request)
	if err != nil {
		return "", err
	}
	request.proposal = id
	return proposed.Kind, nil
}

// loadProposal fills the request from the proposal id names, and returns it.
//
// Every input of §5.7.3 comes from the stored proposal: its kind decides which
// branch runs, its `input` is the patch or the test file's content, and its
// target, filter and paths are the run's. Nothing is re-read from the command
// line, which is what `onlyTheProposal` has already refused.
func loadProposal(owner, repo string, pr int, id string, request *probeRequest) (*proposal.Proposal, error) {
	if !proposal.ValidID(id) {
		return nil, &UnknownProposalError{ID: id}
	}
	layout, err := state.Default()
	if err != nil {
		return nil, err
	}
	round, err := briefedRound(layout, owner, repo, pr)
	if err != nil {
		return nil, err
	}
	// §9.3.2: running a proposal writes probes.ndjson, findings.ndjson and
	// proposals.ndjson, all stamped with the round's head, so a head that
	// moved under it refuses here rather than after the suite has run.
	if err := round.RefuseStale(); err != nil {
		return nil, err
	}
	stored, err := state.ReadRecords[proposal.Proposal](layout, owner, repo, pr, state.FileProposals)
	if err != nil {
		return nil, err
	}
	found := -1
	for i := range stored {
		if stored[i].ID == id {
			found = i
		}
	}
	if found < 0 {
		return nil, &UnknownProposalError{ID: id}
	}
	held := &stored[found]
	probes, err := state.ReadRecords[probe.Record](layout, owner, repo, pr, state.FileProbes)
	if err != nil {
		return nil, err
	}
	if err := runnableNow(held, &round.Meta, probes); err != nil {
		return nil, err
	}
	return held, fillFromProposal(held, request)
}

// runnableNow is §5.7.3's and §5.7.4's refusals of a proposal this run may not
// execute: one of another round or head, one already run whose probe settled
// something, and one §5.7.5 stored with a reason. probes are the pull
// request's stored probe records, the one a run proposal names among them.
func runnableNow(held *proposal.Proposal, round *state.Meta, probes []probe.Record) error {
	switch {
	case held.Round != round.Round || held.Head != round.Head:
		return &StaleProposalError{
			ID: held.ID, Round: held.Round, At: round.Round,
			Head: held.Head, OnHead: round.Head,
		}
	case held.State == proposal.StateRun && !probeUnsettled(probes, held.Probe):
		return &ProposalSpentError{ID: held.ID, Probe: held.Probe}
	case held.State == proposal.StateUnrunnable:
		return &UnrunnableProposalError{ID: held.ID, Reason: held.Reason}
	}
	return nil
}

// probeUnsettled reports whether the probe id names ended on a result
// §5.7.3 runs a proposal again for.
func probeUnsettled(probes []probe.Record, id string) bool {
	for i := range probes {
		if probes[i].ID == id {
			return probes[i].Result.Unsettled()
		}
	}
	return false
}

// fillFromProposal puts the proposal's inputs where the two kinds read them.
func fillFromProposal(held *proposal.Proposal, request *probeRequest) error {
	request.filter, request.paths = held.Filter, held.Paths
	switch held.Kind {
	case proposal.KindMutation:
		files, err := git.ParsePatch(held.Input)
		if err != nil {
			return err
		}
		if len(files) == 0 {
			return &git.MalformedPatchError{
				File: held.ID, Problem: "holds no hunk: §5.3.1's mutation is a unified diff " +
					"against a sandbox file, and §5.7's `input` carries it whole",
			}
		}
		request.kind, request.patch, request.files = probe.Mutation, held.Input, files
		request.patchFile = held.ID
	case proposal.KindGap:
		request.kind, request.test, request.target = probe.Gap, held.Input, held.Target
	default:
		// The decoder closes `kind` at the two, so this is reachable
		// only through a hand-edited state file.
		return fmt.Errorf("proposal %s names kind %q, and §5.7 admits mutation and gap",
			held.ID, held.Kind)
	}
	return nil
}

// settleProposal is §5.7.4: the executed proposal becomes `run` and names the
// probe, and the record it named is re-graded against that probe — unless the
// probe's result is unsettled, in which case the record keeps its probe and
// grade, and the answer says it did.
//
// The exception is measured. On tarfin-labs/backend#6328 with cr 0.14.0,
// f4201 stood on p3, a `no-test-failed` over a passing baseline; proposal
// x4202, naming it, ran twice with a filter selecting no test and came back
// `inconclusive` both times, and each run moved f4201 onto the new probe,
// dropping it to `argued` and taking its evidence out of the draft. A run that
// established nothing is not evidence against the one that did.
//
// The two writes are one step because they are one fact. A proposal that ran
// and a record that still reads `argued` over the experiment that settled it
// would be the round saying two things, and the reader has no way to tell which
// is current.
func settleProposal(
	l state.Layout, owner, repo string, pr int, round *state.Meta, id, probeID string, result probe.Result,
) (*Regraded, error) {
	// The round's own proposals: loadProposal already refused one of
	// another round, and ReplaceStamped keeps every earlier round's line
	// byte for byte while it rewrites these.
	stored, err := state.ReadStamped[proposal.Proposal](
		l, owner, repo, pr, state.FileProposals, round.Round)
	if err != nil {
		return nil, err
	}
	settled := -1
	for i := range stored {
		if stored[i].ID == id {
			stored[i].State, stored[i].Probe = proposal.StateRun, probeID
			settled = i
		}
	}
	if settled < 0 {
		return nil, &UnknownProposalError{ID: id}
	}
	held, err := l.LockPR(owner, repo, pr)
	if err != nil {
		return nil, err
	}
	lines := make([]*proposal.Proposal, 0, len(stored))
	for i := range stored {
		lines = append(lines, &stored[i])
	}
	stamp := state.Stamp{Head: round.Head, Round: round.Round}
	if err := state.ReplaceStamped(held, state.FileProposals, stamp, lines); err != nil {
		_ = held.Unlock()
		return nil, err
	}
	if err := held.Unlock(); err != nil {
		return nil, err
	}
	if stored[settled].Finding == "" {
		return nil, nil
	}
	if result.Unsettled() {
		return keptProbe(l, owner, repo, pr, round, stored[settled].Finding, probeID, result)
	}
	return regradeFromProbe(l, owner, repo, pr, round, stored[settled].Finding, probeID)
}

// Regraded is what §5.7.4's recomputation did to the record a proposal named.
type Regraded struct {
	// Record is the record id, and Probe the probe that was just written.
	Record string `json:"record"`
	Probe  string `json:"probe"`
	// Was and Now are the grade the record held and the grade it holds.
	// They are reported whether or not they differ: a proposal that ran and
	// changed nothing is an answer, and one the reader has to be told.
	Was finding.Grade `json:"was"`
	Now finding.Grade `json:"now"`
	// Kept says the record kept its probe and grade because the probe just
	// written came back `inconclusive`, `error` or `timeout`, and is absent
	// when §5.7.4's recomputation ran. Holds is the probe the record still
	// names then, absent when it names none.
	Kept  bool   `json:"kept,omitempty"`
	Holds string `json:"holds,omitempty"`
	// result is the unsettled result that kept the record, for Disclosure.
	result probe.Result
}

// Disclosure is the sentence §5.7.4 has cr report when the record kept its
// probe and grade, and empty when the recomputation ran: that one the `was`
// and `now` grades already report.
func (r *Regraded) Disclosure() string {
	if r == nil || !r.Kept {
		return ""
	}
	holds := "no probe"
	if r.Holds != "" {
		holds = "probe " + r.Holds
	}
	return fmt.Sprintf("record %s keeps %s and grade %s, per §5.7.4: probe %s came back %s, "+
		"which establishes nothing, so it moves no record's probe", r.Record, holds, r.Now, r.Probe, r.result)
}

// keptProbe is §5.7.4's other branch: the named record is left exactly as it
// stands, and what it stands on is reported. Nothing is written.
func keptProbe(
	l state.Layout, owner, repo string, pr int, round *state.Meta, recordID, probeID string, result probe.Result,
) (*Regraded, error) {
	records, err := roundFindingsOf(l, owner, repo, pr, round.Round)
	if err != nil {
		return nil, err
	}
	for _, record := range records {
		if record.ID == recordID {
			return &Regraded{
				Record: recordID, Probe: probeID, Was: record.Grade, Now: record.Grade,
				Kept: true, Holds: record.Probe, result: result,
			}, nil
		}
	}
	// As in regradeFromProbe: a round re-recorded since may no longer hold
	// the record, and the probe stands on its own.
	return nil, nil
}

// regradeFromProbe writes the probe onto the record the proposal named and
// recomputes its grade per §6.2, re-applying §6.3's forcing.
//
// The grade is recomputed rather than assumed to rise. §5.3.5 and §5.4.4 decide
// what a result establishes, and a mutation the suite noticed disproves the gap
// — so a proposal that ran can leave a record exactly where it was, and saying
// otherwise would be cr asserting from the fact that an experiment happened.
func regradeFromProbe(
	l state.Layout, owner, repo string, pr int, round *state.Meta, recordID, probeID string,
) (*Regraded, error) {
	records, err := roundFindingsOf(l, owner, repo, pr, round.Round)
	if err != nil {
		return nil, err
	}
	var named *finding.Finding
	for _, record := range records {
		if record.ID == recordID {
			named = record
		}
	}
	if named == nil {
		// §5.7.1 held the `finding` to a record of the round when the
		// proposal was stored; a round re-recorded since may no longer
		// hold it, and the probe still stands on its own.
		return nil, nil
	}
	grading, err := readRoundGrading(l, owner, repo, pr, round)
	if err != nil {
		return nil, err
	}
	was := named.Grade
	named.Probe = probeID
	referenced := grading.referenced(probeID)
	var baseline probe.Baseline
	if referenced != nil {
		baseline, _ = referenced.ResolveBaseline(grading.runs)
	}
	claim := probe.MapClaim(grading.pairs, round.Round, named.Claim, named.Unit)
	finding.RegradeOnProbe(named, finding.Resolved(
		unitOf(grading.formed, named.Unit), &named.Anchor, referenced, round.Head, baseline, claim,
	))
	// §6.3.1 again over the round's records, so a record the experiment
	// left `argued` stays a question and one it lifted stops being forced.
	grading.forceQuestions(records)
	held, err := l.LockPR(owner, repo, pr)
	if err != nil {
		return nil, err
	}
	stamp := state.Stamp{Head: round.Head, Round: round.Round}
	if err := state.ReplaceStamped(held, state.FileFindings, stamp, records); err != nil {
		_ = held.Unlock()
		return nil, err
	}
	if err := held.Unlock(); err != nil {
		return nil, err
	}
	return &Regraded{Record: recordID, Probe: probeID, Was: was, Now: named.Grade}, nil
}
