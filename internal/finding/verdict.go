package finding

import (
	"fmt"
	"strings"
	"time"
)

// Verdict is §9.5.5's three verbs: what an agent concluded about one posted
// record after reading what came back.
//
// It is a type of its own rather than a State because two of the three name a
// state and the third does not. `standing` is a judgement that changes nothing
// — the concern was looked at and is still open — and §9.5.5 records it for
// exactly that reason: a question nobody read and a question somebody read and
// left open are different facts, and before v0.5 both looked like `posted`.
//
// The shape is State's, and for State's reason: the only field is unexported,
// so nothing outside this file can mint a fourth verb.
type Verdict struct{ name string }

// The verbs of §9.5.5, in the order the section writes them.
var (
	// VerdictAnswered is a posted question the author's reply settled.
	VerdictAnswered = Verdict{"answered"}
	// VerdictAddressed is a posted record whose concern is gone at the
	// current head.
	VerdictAddressed = Verdict{"addressed"}
	// VerdictStanding is a posted record that was read and still stands.
	VerdictStanding = Verdict{"standing"}
)

// verdicts is §9.5.5's vocabulary, closed.
var verdicts = []Verdict{VerdictAnswered, VerdictAddressed, VerdictStanding}

// String returns the verb as §9.5.5 writes it.
func (v Verdict) String() string { return v.name }

// State returns the state this verdict moves a record to, and false for
// `standing`, which moves none.
func (v Verdict) State() (State, bool) {
	switch v {
	case VerdictAnswered:
		return StateAnswered, true
	case VerdictAddressed:
		return StateAddressed, true
	default:
		return State{}, false
	}
}

// UnknownVerdictError reports a value that names no verb of §9.5.5.
type UnknownVerdictError struct {
	// Value is what was given.
	Value string
}

func (e *UnknownVerdictError) Error() string {
	names := make([]string, 0, len(verdicts))
	for _, v := range verdicts {
		names = append(names, v.String())
	}
	return fmt.Sprintf("%q is not a verdict; §9.5.5 has exactly %s",
		e.Value, strings.Join(names, ", "))
}

// ParseVerdict resolves one of §9.5.5's verbs.
func ParseVerdict(name string) (Verdict, error) {
	for _, v := range verdicts {
		if v.name == name {
			return v, nil
		}
	}
	return Verdict{}, &UnknownVerdictError{Value: name}
}

// AnsweredNeedsAQuestionError reports `answered` given for a record that asks
// nothing.
//
// §9.5.5 refuses it because the word would be false: a finding asserts, and an
// assertion is addressed or withdrawn. Letting it through would put a record in
// `answered` that no reply could have answered, and §9.1.3 counts an answered
// record as settled — so a convergence figure would be counting a finding
// nobody dealt with as dealt with.
type AnsweredNeedsAQuestionError struct {
	// Record is the id, and Kind what it turned out to be.
	Record, Kind string
}

func (e *AnsweredNeedsAQuestionError) Error() string {
	return fmt.Sprintf(
		"record %s is a %s, and §9.5.5 admits answered only on a question; "+
			"a finding is addressed or withdrawn", e.Record, e.Kind)
}

// VerdictRecord is one line of verdicts.ndjson: §9.5.5's judgement, with what
// it was made on.
//
// The evidence is stored and never parsed, exactly as §6.2.1 treats a record's
// evidence. It is there for the human who reads the round back and for §7.3's
// statistics to count beside, not for cr to draw a second conclusion from — a
// verdict cr re-derived from its evidence would be cr reaching the judgement
// §9.5.6 withholds, one step removed.
type VerdictRecord struct {
	// Record is the id of the record judged.
	Record string `json:"record"`
	// Verdict is §9.5.5's verb.
	Verdict string `json:"verdict"`
	// Evidence is what the agent gave for it, verbatim.
	Evidence string `json:"evidence"`
	// Head is the head the judgement was made against, so a verdict from
	// before a push is distinguishable from one after it.
	Head string `json:"head"`
	// Round is the round current when it was recorded.
	Round int `json:"round"`
	// At is the moment it was recorded.
	At time.Time `json:"at"`
}
