package review

import (
	"bytes"
	"encoding/json"
	"errors"
	"io/fs"
	"slices"
	"time"

	"github.com/deligoez/cr/internal/finding"
	"github.com/deligoez/cr/internal/intent"
	"github.com/deligoez/cr/internal/mapping"
	"github.com/deligoez/cr/internal/note"
	"github.com/deligoez/cr/internal/state"
)

// PassAll is the pass an emission of the whole fan-out records: `cr review`
// run with no `--axis`. Every other pass is the axis id `--axis` named.
const PassAll = "all"

// Emission is one line of state.FileEmissions: one prompt `cr review` emitted,
// for which round and head, in which pass, when, and the ids of the notes it
// carried.
//
// It exists so a note recorded after a prompt can be told apart from a note the
// prompt carried (field-feedback 1.5). What is compared is two timestamps and a
// list of ids. Whether the note settles what a record raised is left to the
// agent and the human, per §2.1.3, and nothing here is copied onto a record:
// §6.1 fixes a record's fields, so the report is computed when read.
type Emission struct {
	state.Stamp
	// Pass is PassAll, or the axis `--axis` named.
	Pass string `json:"pass"`
	// Axis, Role and Unit are the prompt's, as Prompt carries them.
	Axis string `json:"axis"`
	Role string `json:"role"`
	Unit string `json:"unit"`
	// EmittedAt is when `cr review` emitted the prompt, in UTC. One run
	// stamps every prompt it emits with one reading.
	EmittedAt time.Time `json:"emitted_at"`
	// Notes are the ids of the notes the prompt carried: the issue key's
	// notes that stood when it was emitted.
	Notes []string `json:"notes"`
}

// now is the clock an emission is stamped with.
func (s *Sources) now() time.Time {
	if s.Clock != nil {
		return s.Clock().UTC()
	}
	return time.Now().UTC()
}

// recordEmissions appends one line per prompt to state.FileEmissions, under
// the pull request's lock (§2.3.1). A run that emitted no prompt writes
// nothing.
func (r *Round) recordEmissions(src *Sources, prompts []Prompt) error {
	if len(prompts) == 0 {
		return nil
	}
	carried := make([]string, 0, len(r.Notes))
	for i := range r.Notes {
		carried = append(carried, r.Notes[i].ID)
	}
	pass := src.Axis
	if pass == "" {
		pass = PassAll
	}
	at := src.now()
	lines := make([]Emission, 0, len(prompts))
	for i := range prompts {
		lines = append(lines, Emission{
			Stamp: state.Stamp{Head: r.Head, Round: r.Round},
			Pass:  pass, Axis: prompts[i].Axis, Role: prompts[i].Role, Unit: prompts[i].Unit,
			EmittedAt: at, Notes: carried,
		})
	}
	held, err := src.Layout.LockPR(src.Owner, src.Repo, src.PR)
	if err != nil {
		return err
	}
	if err := state.AppendRecords(held, state.FileEmissions, lines); err != nil {
		_ = held.Unlock()
		return err
	}
	return held.Unlock()
}

// ReadEmissions returns the emission lines of one round, in file order, and
// none when no `cr review` of this version has emitted for the pull request.
// It takes no lock, per §2.3.2.
func ReadEmissions(l state.Layout, owner, repo string, pr, round int) ([]Emission, error) {
	body, err := l.ReadPR(owner, repo, pr, state.FileEmissions)
	if errors.Is(err, fs.ErrNotExist) {
		return []Emission{}, nil
	}
	if err != nil {
		return nil, err
	}
	kept := make([]Emission, 0)
	for _, line := range bytes.Split(body, []byte{'\n'}) {
		if len(bytes.TrimSpace(line)) == 0 {
			continue
		}
		var emitted Emission
		if err := json.Unmarshal(line, &emitted); err != nil {
			return nil, state.FileFailure("read",
				l.PRFile(owner, repo, pr, state.FileEmissions), state.UnusableHint, err)
		}
		if emitted.Round == round {
			kept = append(kept, emitted)
		}
	}
	return kept, nil
}

// Postdated is one run of `cr review` a note was recorded after and whose
// prompts did not carry it.
type Postdated struct {
	Round int    `json:"round"`
	Head  string `json:"head"`
	// Pass is PassAll or the axis the run named.
	Pass string `json:"pass"`
	// EmittedAt is the run's one timestamp.
	EmittedAt time.Time `json:"emitted_at"`
	// Roles are the roles the run emitted prompts for, in emission order.
	Roles []string `json:"roles"`
	// Prompts is how many prompts the run emitted.
	Prompts int `json:"prompts"`
}

// PassesBefore is every run of the emissions given that emitted before the
// note was recorded and did not carry it, one entry per run, in file order.
//
// A run is the lines sharing round, head, pass and timestamp, which is what one
// `cr review` writes.
func PassesBefore(emitted []Emission, recorded *note.Note) []Postdated {
	passes := make([]Postdated, 0)
	for i := range emitted {
		line := &emitted[i]
		if !line.EmittedAt.Before(recorded.RecordedAt) || slices.Contains(line.Notes, recorded.ID) {
			continue
		}
		at := slices.IndexFunc(passes, func(p Postdated) bool {
			return p.Round == line.Round && p.Head == line.Head && p.Pass == line.Pass && p.EmittedAt.Equal(line.EmittedAt)
		})
		if at < 0 {
			passes = append(passes, Postdated{
				Round: line.Round, Head: line.Head, Pass: line.Pass, EmittedAt: line.EmittedAt, Roles: []string{},
			})
			at = len(passes) - 1
		}
		passes[at].Prompts++
		if !slices.Contains(passes[at].Roles, line.Role) {
			passes[at].Roles = append(passes[at].Roles, line.Role)
		}
	}
	return passes
}

// RecordBeforeNotes is one record written from a prompt emitted before a note
// on its claim or unit was recorded, with those notes' ids.
type RecordBeforeNotes struct {
	Record string   `json:"record"`
	Notes  []string `json:"notes"`
}

// NotesAfterPrompts is the report field-feedback 1.5 asks `cr record`,
// `cr status` and `cr draft` for: the records whose prompt a standing note on
// their claim or unit postdates.
type NotesAfterPrompts struct {
	// Count is how many records Records lists.
	Count int `json:"count"`
	// Records are those records, in the order they were given.
	Records []RecordBeforeNotes `json:"records"`
	// Unattributed are the records no emission of their role and unit
	// precedes, so which notes their prompt carried is unknown. It is
	// empty when the issue key holds no standing note, since no note can
	// then postdate a prompt.
	Unattributed []string `json:"unattributed"`
}

// Only narrows the report to the records ids names, keeping its order.
func (n NotesAfterPrompts) Only(ids []string) NotesAfterPrompts {
	narrowed := NotesAfterPrompts{Records: []RecordBeforeNotes{}, Unattributed: []string{}}
	for _, entry := range n.Records {
		if slices.Contains(ids, entry.Record) {
			narrowed.Records = append(narrowed.Records, entry)
		}
	}
	for _, id := range n.Unattributed {
		if slices.Contains(ids, id) {
			narrowed.Unattributed = append(narrowed.Unattributed, id)
		}
	}
	narrowed.Count = len(narrowed.Records)
	return narrowed
}

// PromptNotes is what NotesAfter compares.
type PromptNotes struct {
	// Emissions are the round's emission lines.
	Emissions []Emission
	// Notes is the issue key's store, whole.
	Notes []note.Note
	// Claims are the round's claims, which tie a note to a claim through
	// §3.3.2's `note_id`.
	Claims []intent.Claim
	// Records are the round's records.
	Records []*finding.Finding
	// RecordedAt is when `cr record` stored each record, by id, read off
	// §9.1.1's journal. A record missing from it is compared against every
	// emission of its role and unit.
	RecordedAt map[string]time.Time
	// PR is the pull request, which a note answering a record names.
	PR int
}

// NotesAfter reports every record whose prompt a standing note on its claim or
// unit postdates.
//
// A record's prompt is the latest emission of its round, head, role and unit
// made no later than the record was stored: re-emitting a prompt hands out the
// same id block (§4.6.2), so the ids cannot say which emission a record came
// from, and the latest one before it is the one an agent could have run last.
// A note postdates that prompt when it was recorded after it and the prompt did
// not carry it.
//
// A note is on a record's claim or unit through the links it has as stored: a
// claim of the round drawn from it (§3.3.2) ties it to that claim alone, and
// failing that, a record of the round it answers (§3.6.2) ties it to that
// record's unit alone. A note with neither link is counted against every prompt
// it postdates.
func NotesAfter(in *PromptNotes) NotesAfterPrompts {
	report := NotesAfterPrompts{Records: []RecordBeforeNotes{}, Unattributed: []string{}}
	standing := standingNotes(in.Notes)
	if len(standing) == 0 {
		return report
	}
	for _, record := range in.Records {
		prompt := in.promptOf(record)
		if prompt == nil {
			report.Unattributed = append(report.Unattributed, record.ID)
			continue
		}
		missed := missedNotes(prompt, standing, func(recorded *note.Note) bool {
			return linkOf(recorded, in.Claims, in.Records, in.PR).onRecord(record)
		})
		if len(missed) > 0 {
			report.Records = append(report.Records, RecordBeforeNotes{Record: record.ID, Notes: missed})
		}
	}
	report.Count = len(report.Records)
	return report
}

// promptOf is the emission a record is compared against, and nil when none of
// its role and unit precedes it.
func (in *PromptNotes) promptOf(record *finding.Finding) *Emission {
	stored, dated := in.RecordedAt[record.ID]
	return latestEmission(in.Emissions, record.Stamp, record.Role, record.Unit, func(line *Emission) bool {
		return !dated || !line.EmittedAt.After(stored)
	})
}

// latestEmission is the latest line of emitted for one role and unit at the
// stamp's round and head that within admits, and nil when there is none.
//
// It is the one reading of "the latest line" both of its callers need: a
// record's prompt, bounded by when the record was stored, and §4.6.1's latest
// line of a cell, bounded by nothing.
func latestEmission(emitted []Emission, stamp state.Stamp, roleID, unitID string, within func(*Emission) bool) *Emission {
	var latest *Emission
	for i := range emitted {
		line := &emitted[i]
		if line.Role != roleID || line.Unit != unitID || line.Round != stamp.Round ||
			line.Head != stamp.Head || !within(line) {
			continue
		}
		if latest == nil || line.EmittedAt.After(latest.EmittedAt) {
			latest = line
		}
	}
	return latest
}

// missedNotes is the ids of the standing notes recorded after the line was
// emitted that the line does not carry and that on places on what the line
// was emitted for, in the order standing holds them.
func missedNotes(line *Emission, standing []note.Note, on func(*note.Note) bool) []string {
	missed := make([]string, 0)
	for i := range standing {
		if standing[i].RecordedAt.After(line.EmittedAt) &&
			!slices.Contains(line.Notes, standing[i].ID) && on(&standing[i]) {
			missed = append(missed, standing[i].ID)
		}
	}
	return missed
}

// noteLink is where a note sits by the links it has as stored: the claims of
// the round drawn from it (§3.3.2), or failing any, the record of the round it
// answers (§3.6.2). A note with neither is on everything.
type noteLink struct {
	// claims are the ids of the round's claims drawn from the note.
	claims []string
	// answered is the round's record the note answers, nil when a claim
	// was drawn from the note or it answers no record of the round.
	answered *finding.Finding
}

// linkOf reads a note's links out of the round's claims and records. A note
// answering a record is read together with its pull request, because §11
// scopes a record id to one.
func linkOf(recorded *note.Note, claims []intent.Claim, records []*finding.Finding, pr int) noteLink {
	link := noteLink{claims: make([]string, 0)}
	for i := range claims {
		if claims[i].Source == intent.ClaimFromNote && claims[i].NoteID == recorded.ID {
			link.claims = append(link.claims, claims[i].ID)
		}
	}
	if len(link.claims) > 0 || recorded.Record == "" || recorded.PR != pr {
		return link
	}
	for _, answered := range records {
		if answered.ID == recorded.Record {
			link.answered = answered
			return link
		}
	}
	return link
}

// onRecord reports whether the note is on the record's claim or unit.
func (l noteLink) onRecord(record *finding.Finding) bool {
	if len(l.claims) > 0 {
		return slices.Contains(l.claims, record.Claim)
	}
	if l.answered != nil {
		return l.answered.Unit == record.Unit
	}
	return true
}

// onUnit reports whether the note is on the unit, per §4.6.1: a note a claim
// was drawn from is on the units the round's mapping maps that claim to, a
// note answering a record is on that record's unit, and every other note is on
// every unit.
func (l noteLink) onUnit(unitID string, pairs []mapping.Pair, round int) bool {
	if len(l.claims) > 0 {
		return slices.ContainsFunc(mapping.ClaimsOf(pairs, round, unitID), func(claim string) bool {
			return slices.Contains(l.claims, claim)
		})
	}
	if l.answered != nil {
		return l.answered.Unit == unitID
	}
	return true
}

// NotesAfterOf reads what NotesAfter compares for one round and reports over
// the records given, which are the round's.
func NotesAfterOf(l state.Layout, round *state.Meta, records []*finding.Finding) (NotesAfterPrompts, error) {
	owner, repo, pr := round.Owner, round.Repo, round.PR
	notes, err := notesOf(l, round.IssueKey)
	if err != nil {
		return NotesAfterPrompts{}, err
	}
	emitted, err := ReadEmissions(l, owner, repo, pr, round.Round)
	if err != nil {
		return NotesAfterPrompts{}, err
	}
	claims, err := state.ReadStamped[intent.Claim](l, owner, repo, pr, state.FileClaims, round.Round)
	if err != nil {
		return NotesAfterPrompts{}, err
	}
	journal, err := state.ReadRecords[finding.Transition](l, owner, repo, pr, state.FileTransitions)
	if err != nil {
		return NotesAfterPrompts{}, err
	}
	// §9.1's first row, which only `cr record` makes, is the moment a record
	// was stored; every later move of it has a state to come from.
	stored := make(map[string]time.Time)
	for i := range journal {
		if journal[i].From == nil {
			stored[journal[i].Record] = journal[i].At
		}
	}
	return NotesAfter(&PromptNotes{
		Emissions: emitted, Notes: notes, Claims: claims, Records: records, RecordedAt: stored, PR: pr,
	}), nil
}
