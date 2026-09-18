package coverage

import (
	"strconv"
	"strings"

	"github.com/deligoez/cr/internal/finding"
)

// Conditions are the four answers §10.2 asks about one round, each read from
// the state that holds it.
//
// They are supplied rather than gathered here for the reason Lenses collects
// its four kinds rather than deriving them: §10.2's conditions live in four
// different files of §2.3's table, and a package that read them itself would
// be a second reader of each — one that could disagree with the report §10.1
// prints from the first.
type Conditions struct {
	// HeadMoved is §10.2.1's condition, negated: §9.3.1's comparison,
	// true when the pull request's current head is no longer the head the
	// round was recorded against.
	HeadMoved bool
	// Rows is §10.2.2, as RowsOf counts it. Its Gaps is the condition over
	// the units the round formed: a row counts complete only when every
	// active role filled a cell for the unit's current §3.4.6 hash, which
	// is exactly what §10.2.2 asks and is why nothing here recounts it. Its
	// Units is read too, because a round that formed none has no row to
	// count and rowReason refuses to call it complete.
	Rows Rows
	// Unsettled is §10.2.3's blocking set: the round's claims that its
	// mapping maps to no unit and that §4.1.8 has not set aside.
	//
	// The two halves are one list because §10.2.3 makes them one
	// condition. A claim mapped to no unit blocks; a claim carrying
	// §4.1.3's unimplemented entry goes on blocking until it is mapped or
	// set aside; and §10.2.3 says in as many words that this condition is
	// the only thing that makes an unimplemented claim block, because the
	// entry is not a record and §10.2.4 never reaches it.
	Unsettled []string
	// Records are the round's findings and questions, whole. §10.2.4 asks
	// whether any of them remains in `draft` or `queued`, which is
	// §9.1.2's open set — derived from the terminal list rather than
	// written out, so this condition and §9.1's table cannot drift.
	Records []*finding.Finding
}

// Completeness is §10.2's verdict over one round: whether all four conditions
// hold, and the exact reason for each that does not.
//
// The reasons are a list rather than the first failure. §10.2 asks for "the
// exact reason" when the verdict is false, and a round can fail several
// conditions at once — a reviewer shown only the first would fix it, run again,
// and be told about the second, which is the same report delivered one round at
// a time.
//
// It is never nil, per the §12.3 convention: a complete round has no reasons
// rather than a null list.
type Completeness struct {
	// Complete is §10.2's verdict.
	Complete bool `json:"complete"`
	// Reasons are the conditions that did not hold, in §10.2's own order,
	// each naming the item it comes from.
	Reasons []string `json:"reasons"`
}

// Reason is the exact reason §10.2 requires beside a false verdict, as one
// sentence, and the empty string when the round is complete: §10.2 asks for a
// reason only when the verdict is false, and there is none for a complete round
// beyond the four conditions all holding.
func (c Completeness) Reason() string {
	return strings.Join(c.Reasons, "; ")
}

// Complete answers §10.2 over one round.
//
// The four checks run in §10.2's own numbering and each contributes at most one
// reason, so the reasons a reader is given are the items of §10.2 they can go
// and read.
func Complete(c *Conditions) Completeness {
	reasons := make([]string, 0, 4)
	for _, check := range []func(*Conditions) string{
		headReason, rowReason, claimReason, recordReason,
	} {
		if said := check(c); said != "" {
			reasons = append(reasons, said)
		}
	}
	return Completeness{Complete: len(reasons) == 0, Reasons: reasons}
}

// headReason is §10.2.1: the current head equals the head the round was
// recorded against, per §9.3.1.
//
// It names no commit. §9.3.1's own report names both heads and §11.1 exempts it
// from `--quiet`, so a reader of a status report already has them; repeating
// them here would be a second place for the pair to be stated and a second
// place for it to be stated wrongly.
func headReason(c *Conditions) string {
	if !c.HeadMoved {
		return ""
	}
	return "§10.2.1: the pull request's head is no longer the head this round was recorded against"
}

// rowReason is §10.2.2: every unit has a complete row of cells for every active
// role, each cell filled for that unit's current unit hash per §3.4.6.
//
// A round that formed no unit is not held to it vacuously. "Every unit has a
// complete row" is true of an empty set, so a round whose diff yielded nothing
// would otherwise pass §10.2.2 without a single cell having been filled — and
// `cr status` would print that a review was complete over a review of nothing,
// the stronger claim than the round supports that §10.2's verdict exists to
// refuse. So an empty round carries §10.2.2's reason, naming why.
func rowReason(c *Conditions) string {
	if c.Rows.Units == 0 {
		return "§10.2.2: this round formed no unit from its diff, so no cell was filled " +
			"and there is no row of cells its coverage could be complete over"
	}
	if c.Rows.Gaps == 0 {
		return ""
	}
	return "§10.2.2: " + strconv.Itoa(c.Rows.Gaps) + " of " + strconv.Itoa(c.Rows.Units) +
		" unit(s) hold no complete row of cells for all " + strconv.Itoa(c.Rows.Roles) +
		" active role(s) at their current unit hash"
}

// claimReason is §10.2.3: every claim is mapped to at least one unit, and an
// unimplemented-claim entry blocks until it is mapped or set aside.
func claimReason(c *Conditions) string {
	if len(c.Unsettled) == 0 {
		return ""
	}
	return "§10.2.3: " + strconv.Itoa(len(c.Unsettled)) +
		" claim(s) are mapped to no unit and not set aside: " + strings.Join(c.Unsettled, ", ")
}

// recordReason is §10.2.4: no record of the current round remains in `draft` or
// `queued`.
func recordReason(c *Conditions) string {
	open := make([]string, 0, len(c.Records))
	for _, record := range c.Records {
		if record.State.Open() {
			open = append(open, record.ID)
		}
	}
	if len(open) == 0 {
		return ""
	}
	return "§10.2.4: " + strconv.Itoa(len(open)) + " record(s) are still in " +
		openStates() + ": " + strings.Join(open, ", ")
}

// openStates names §9.1.2's open states the way the reason says them, read out
// of internal/finding rather than written here.
//
// §10.2.4 spells the two — `draft` and `queued` — and finding.OpenStates
// derives them from the terminal list, so a state added to §9.1 as non-terminal
// would both block completeness and be named. A sentence that spelled the two
// itself would go on saying `draft or queued` about a round blocked by a third.
func openStates() string {
	held := finding.OpenStates()
	names := make([]string, 0, len(held))
	for _, state := range held {
		names = append(names, state.String())
	}
	return strings.Join(names, " or ")
}
