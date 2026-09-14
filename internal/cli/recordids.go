package cli

import (
	"fmt"

	"github.com/deligoez/cr/internal/finding"
	"github.com/deligoez/cr/internal/review"
	"github.com/deligoez/cr/internal/role"
	"github.com/deligoez/cr/internal/state"
)

// idInput is one file whose record ids refuseHeldIDs holds to §6.1: the path
// the agent named, its body, and the records decoded from it.
type idInput struct {
	file    string
	body    []byte
	records []*finding.Finding
}

// refuseHeldIDs refuses inputs whose record ids repeat within or across them,
// or collide with a record already stored for the pull request, naming the id
// and both lines or the line and the stored record, and a free id to use.
//
// §6.1 makes a record id stable for the life of the pull request, and every
// store that names a record — a draft marker, a waiver, the posted index, a
// triage event — names it by that id alone. Two records holding one id would
// let any of them reach either.
//
// It lives at the commands rather than inside finding.Decode for the reason
// the second half needs: the stored ids are the pull request's state, and the
// decoder is handed a file and the round's units, never the store. `cr record`
// is the only command that adds a record to findings.ndjson, so no route to
// the store passes around it; `cr merge` asks the same question of its
// per-role files, so §6.4.3's `duplicate_of` never names an id two records
// hold. The repeat is checked beside the collision so the one refusal answers
// both. The store is read whole, across rounds, because an id an earlier
// round's record holds is held still: §9.3.4 moves such a record to `stale`
// and keeps it.
//
// Every prompt of round's fan-out is given its own block of ids (§4.6.2), so
// the free id a refusal names is one inside the block of the refused record's
// own prompt — its role over its unit — and one neither stored nor carried by
// any line of any input. A round-wide next id would sit inside another prompt's
// block, where that prompt's role may already have written it. When exactly one
// of two lines sharing an id lies outside its prompt's block, that line is the
// one refused, whichever was read first: the other role did what its prompt
// said.
func refuseHeldIDs(l state.Layout, owner, repo string, pr, round int, units []string, inputs []idInput) error {
	stored, err := state.ReadRecords[finding.Finding](l, owner, repo, pr, state.FileFindings)
	if err != nil {
		return err
	}
	held := make(map[string]*finding.Finding, len(stored))
	for i := range stored {
		held[stored[i].ID] = &stored[i]
	}
	free := &freeIDs{l: l, owner: owner, repo: repo, round: round, units: units, stored: stored, inputs: inputs}
	seen := make(map[string]idPlace)
	for _, input := range inputs {
		at := state.RecordLines(input.body)
		for i, record := range input.records {
			here := idPlace{file: input.file, line: at[i], record: record}
			if first, repeated := seen[record.ID]; repeated {
				return free.repeat(here, first)
			}
			seen[record.ID] = here
			if prior, taken := held[record.ID]; taken {
				hint, err := free.hint(record)
				if err != nil {
					return err
				}
				return &finding.RejectedRecordError{
					File: input.file, Line: at[i], Field: "id",
					Problem: fmt.Sprintf(
						"%q is already held by the record stored for this pull request in round %d at head %s; "+
							"§6.1 makes a record id stable for the life of the pull request, so give this record an id no stored record holds; "+
							"%s",
						record.ID, prior.Round, prior.Head, hint),
				}
			}
		}
	}
	return nil
}

// idPlace is one record of an input and the file line it sits on.
type idPlace struct {
	file   string
	line   int
	record *finding.Finding
}

// freeIDs answers which ids a refusal may name, over the store, the inputs, and
// the blocks `cr review` gave the round's prompts. The corpus the blocks are
// laid out by is resolved once, and only when a refusal needs it.
type freeIDs struct {
	l            state.Layout
	owner, repo  string
	round        int
	units        []string
	stored       []finding.Finding
	inputs       []idInput
	corpus       []role.Resolved
	corpusLoaded bool
}

// block is the block of the record's own prompt, and false when no prompt of
// the round was given one for its role and unit.
func (f *freeIDs) block(record *finding.Finding) (review.IDs, bool, error) {
	if !f.corpusLoaded {
		corpus, err := role.Resolve(f.l.RepoRolesDir(f.owner, f.repo), f.l.RolesDir())
		if err != nil {
			return review.IDs{}, false, err
		}
		f.corpus, f.corpusLoaded = corpus, true
	}
	block, ok := review.BlockOf(f.stored, f.round, f.corpus, f.units, record.Role, record.Unit)
	return block, ok, nil
}

// departed reports whether the record's id lies outside its own prompt's block.
// A record with no block to measure has not been shown to depart from one.
func (f *freeIDs) departed(record *finding.Finding) (bool, error) {
	block, ok, err := f.block(record)
	if err != nil || !ok {
		return false, err
	}
	n, spelled := finding.IDSuffix(record.ID)
	return !spelled || !block.Holds(n), nil
}

// repeat is the refusal of two lines sharing one id: here, read second, and
// first. The line refused is here unless first alone departed from its block.
func (f *freeIDs) repeat(here, first idPlace) error {
	refused, other := here, first
	firstOut, err := f.departed(first.record)
	if err != nil {
		return err
	}
	hereOut, err := f.departed(here.record)
	if err != nil {
		return err
	}
	if firstOut && !hereOut {
		refused, other = first, here
	}
	where := fmt.Sprintf("line %d", other.line)
	if other.file != refused.file {
		where = fmt.Sprintf("%s line %d", other.file, other.line)
	}
	hint, err := f.hint(refused.record)
	if err != nil {
		return err
	}
	return &finding.RejectedRecordError{
		File: refused.file, Line: refused.line, Field: "id",
		Problem: fmt.Sprintf(
			"%q repeats the id of %s; §6.1 makes a record id stable for the life of the pull request, "+
				"so give each record an id of its own; %s", refused.record.ID, where, hint),
	}
}

// hint names the id to give the record instead: the first id of its prompt's
// block past the stored ones that no stored record and no line of any input
// holds, or that the block has none left. A record whose role and unit were
// given no block is named the next id over the store and every input.
func (f *freeIDs) hint(record *finding.Finding) (string, error) {
	taken := make([]finding.Finding, 0, len(f.stored))
	taken = append(taken, f.stored...)
	for _, input := range f.inputs {
		for _, other := range input.records {
			taken = append(taken, *other)
		}
	}
	block, ok, err := f.block(record)
	if err != nil {
		return "", err
	}
	if !ok {
		return "the next free id is " + finding.NextID(taken), nil
	}
	used := make(map[string]bool, len(taken))
	for i := range taken {
		used[taken[i].ID] = true
	}
	span := fmt.Sprintf("%s..%s, the block cr review gave the %s prompt on unit %s,",
		finding.IDOf(block.Start()), finding.IDOf(block.Last), record.Role, record.Unit)
	for n := block.First; n <= block.Last; n++ {
		if !used[finding.IDOf(n)] {
			return fmt.Sprintf("the next free id in %s is %s", span, finding.IDOf(n)), nil
		}
	}
	return fmt.Sprintf("%s holds no id that is neither stored nor carried by the input, so that block is full", span), nil
}
