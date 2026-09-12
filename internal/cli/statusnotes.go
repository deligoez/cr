package cli

import (
	"slices"
	"strconv"
	"strings"

	"github.com/deligoez/cr/internal/coverage"
	"github.com/deligoez/cr/internal/finding"
	"github.com/deligoez/cr/internal/intent"
	"github.com/deligoez/cr/internal/note"
	"github.com/deligoez/cr/internal/state"
)

// unstandingNote is §3.6.6's report for one note the round rests on that no
// longer stands: the note, what a citation of it may now do, and what is
// resting on it.
//
// §3.6.6 makes a note revocable and asks that any coverage cell or record
// citing a retracted one be reported as needing re-evaluation rather than
// silently retained. `cr note --remove` says so to whoever retracted it; this
// is the same sentence reaching whoever reads the round afterwards, which is
// the only place a reviewer who did not perform the retraction can learn what
// it landed on.
//
// The report lands in the round the retraction happened in rather than in the
// next one, which is note.Standing's reading of §3.6.6 and round 9's
// retracted-provenance-still-posts: v0.1 ends at posting per §9, so for this
// pull request the next round may never arrive, and a consequence deferred to
// it would not be delayed but lost.
type unstandingNote struct {
	// Note is the note id the round's state cites.
	Note string `json:"note"`
	// Standing is what a citation of it may now do. It is carried rather
	// than implied, because the two values that reach here are different
	// facts: `retracted` is a human withdrawing a fact they had recorded,
	// and `dangling` is an id the store holds no note for at all. Neither
	// may be asserted on, and only the first is a retraction.
	Standing note.Standing `json:"standing"`
	// Cells are the round's coverage cells citing it, each named by its
	// §4.5.6 key — the `(unit, role)` a cell sits at — since a cell has no
	// id of its own.
	Cells []string `json:"cells"`
	// Records are the ids of the round's findings and questions citing it,
	// which they do through the claim they name: §3.3.1 gives a claim
	// `source: note` a `note_id`, and §8.1.6 discloses that provenance in
	// the posted body.
	Records []string `json:"records"`
}

// unstandingNotesOf is §3.6.6 over one round: every note this round's coverage
// cells or records cite that no longer stands, with what rests on it.
//
// The store is read whole, as note.StandingOf requires: an id missing from a
// narrowed slice is reported dangling, so a store trimmed by round or by pull
// request would report notes nobody retracted. §9.3.5 exempts the context store
// from round scoping for the same reason.
//
// A round that resolved no issue key cites no note: §3.6.1 forms every note id
// against a key, and §4.5.5's `note_id` and §3.3.1's are ids from that store.
func unstandingNotesOf(
	l state.Layout, owner, repo string, pr int, round *state.Meta,
) ([]unstandingNote, error) {
	report := make([]unstandingNote, 0)
	if round.IssueKey == "" {
		return report, nil
	}
	stored, err := note.Load(l, round.IssueKey)
	if err != nil {
		return nil, err
	}
	cells, err := state.ReadStamped[coverage.Cell](
		l, owner, repo, pr, state.FileCoverage, round.Round)
	if err != nil {
		return nil, err
	}
	claims, err := state.ReadStamped[intent.Claim](
		l, owner, repo, pr, state.FileClaims, round.Round)
	if err != nil {
		return nil, err
	}
	records, err := roundFindingsOf(l, owner, repo, pr, round.Round)
	if err != nil {
		return nil, err
	}
	return citedNotes(stored, cells, claims, records), nil
}

// citedNotes collects one entry per note that no longer stands and that the
// round's cells or records cite, in the order those citations were first met.
//
// A cell with no `note_id` at all is passed over rather than reported dangling.
// §4.5.5 asks for the field only where §4.1.5's decision rests on a note, so an
// empty one is a cell that cited nothing — which is not a citation needing
// re-evaluation, and reporting it as one would put a line in this report for
// every ordinary cell of the round.
func citedNotes(
	stored []note.Note, cells []coverage.Cell,
	claims []intent.Claim, records []*finding.Finding,
) []unstandingNote {
	report := make([]unstandingNote, 0)
	at := make(map[string]int)
	entry := func(id string) *unstandingNote {
		if id == "" {
			return nil
		}
		standing := note.StandingOf(stored, id)
		if standing.Stands() {
			return nil
		}
		if _, held := at[id]; !held {
			at[id] = len(report)
			report = append(report, unstandingNote{
				Note: id, Standing: standing,
				Cells: make([]string, 0), Records: make([]string, 0),
			})
		}
		return &report[at[id]]
	}
	for i := range cells {
		if held := entry(cells[i].NoteID); held != nil {
			held.Cells = append(held.Cells, cells[i].Unit+"/"+cells[i].Role)
		}
	}
	behind := claimNotes(claims)
	for _, record := range records {
		if held := entry(behind[record.Claim]); held != nil {
			held.Records = append(held.Records, record.ID)
		}
	}
	return report
}

// claimNotes is the note behind each claim of the round that was drawn from the
// context store, by claim id.
//
// It is how a record reaches a note. §6.1's record carries no `note_id` of its
// own — it names a claim, and §3.3.1 gives a claim with `source: note` the id
// — which is the same path readNoteClaims walks for §8.1.6's provenance region,
// so what this reports as needing re-evaluation is what that region would
// disclose.
func claimNotes(claims []intent.Claim) map[string]string {
	behind := make(map[string]string, len(claims))
	for i := range claims {
		if claims[i].Source == intent.ClaimFromNote {
			behind[claims[i].ID] = claims[i].NoteID
		}
	}
	return behind
}

// unstandingLine renders one §3.6.6 entry for the terminal: the note, its
// standing, and how much rests on it.
func unstandingLine(held *unstandingNote) string {
	return "  " + held.Note + " " + string(held.Standing) + ": " +
		strconv.Itoa(len(held.Cells)) + " cell(s), " +
		strconv.Itoa(len(held.Records)) + " record(s) need re-evaluation" +
		restingOn(held)
}

// restingOn names what rests on the note, and names nothing when nothing does:
// a note this report lists with no dependant is one the round cited and then
// stopped citing, which is worth saying without a trailing empty list.
func restingOn(held *unstandingNote) string {
	resting := slices.Concat(held.Cells, held.Records)
	if len(resting) == 0 {
		return ""
	}
	return " (" + strings.Join(resting, ", ") + ")"
}
