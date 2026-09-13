// Package mapping models the claim-to-unit mapping of spec/0.1.0.md §4.1.6.
//
// The mapping is the agent's judgement, and it is the join the whole intent
// axis reads. §4.1.1 lets a unit be mapped to zero or more claims; §4.1.2 turns
// a unit mapped to none into an unmapped-unit item; §4.1.3 turns a claim mapped
// to no unit into an intent gap; §4.2.1 evaluates each unit against the claims
// it is mapped to. Every one of those rules asks this one file, so nothing else
// in cr may hold a second opinion about which claim covers which unit.
//
// cr forms none of it. §4.1.6 says so outright — "the claim-to-unit mapping is
// the agent's judgement" — and this package therefore decodes, validates
// against ids the round already fixed, and refuses; it never proposes a pair,
// and §4.1.5 adds the same prohibition for the neighbouring decision: cr must
// not decide by matching text.
package mapping

import (
	"bytes"
	"encoding/json"
	"fmt"
	"slices"
	"strings"

	"github.com/deligoez/cr/internal/state"
)

// Pair is one line of mapping.ndjson: a claim of §3.3 and a unit of §3.4.6 the
// agent judged it implemented by, with the head and round §2.3.3 stamps onto
// every record of that file.
//
// §4.1.6 fixes the shape as "one `{claim, unit}` pair per line", and the pair
// carries nothing else. There is no confidence, no rationale, and no ordering:
// a field cr does not read would be a field cr could not check, and §4.1.6
// gives the mapping's meaning entirely to the two ids.
type Pair struct {
	// Claim is the claim id of §3.3, formed as <ISSUE-KEY>#c<n>.
	Claim string `json:"claim"`
	// Unit is the unit id of §3.4.6, scoped to the round that formed it.
	Unit string `json:"unit"`
	state.Stamp
}

// RejectedPairError reports a pair §4.1.6 refuses.
//
// It takes the shape §3.3.1, §6.1.3 and §4.5.6 already give a refused line:
// name the line so the user can open it, and name the field so they know which
// id is wrong. The cli layer maps it onto §11.2's code 1 — the file was found,
// read, and parsed, and what is wrong is the agent's data inside it.
type RejectedPairError struct {
	// File is the NDJSON file the agent handed the command.
	File string
	// Line is the one-based line the pair sits on, counting blank lines.
	Line int
	// Field is the field at fault, named by its JSON key.
	Field string
	// Problem is what is wrong with that field.
	Problem string
}

func (e *RejectedPairError) Error() string {
	return fmt.Sprintf("%s line %d: %s %s", e.File, e.Line, e.Field, e.Problem)
}

// NotAcceptedError is §4.6.6's refusal: a round whose intent axis is unavailable
// per §4.5.3 has no intent role and no mapping to produce, so `cr map record`
// is not accepted there and the mapping is treated as empty.
//
// §11.2 codes it 4. The command line is right and the file may be well formed;
// what refuses is where the round stands — it resolved no issue key — which no
// edit to the file can change.
type NotAcceptedError struct {
	// Owner, Repo, and PR name the pull request, so the message is runnable.
	Owner string
	Repo  string
	PR    int
	// Round is the round that resolved no issue key.
	Round int
}

func (e *NotAcceptedError) Error() string {
	return fmt.Sprintf(
		"round %d of %s/%s#%d resolved no issue key, so §4.5.3 marks the intent axis unavailable "+
			"and §4.6.6 accepts no mapping: there is no intent role to produce one, the mapping is "+
			"treated as empty, and `cr review %d --repo %s/%s` needs none",
		e.Round, e.Owner, e.Repo, e.PR, e.PR, e.Owner, e.Repo)
}

// Decode reads the mapping an agent hands `cr map record`, holding every line
// to §4.1.6.
//
// claims and units are the ids the current round already fixed: the claims
// §3.3.1 recorded and the units §3.7 had `cr brief` write. Both are round
// scoped, and that is the point of passing them rather than looking them up
// here — §9.3.5 has a command read only the current round's records, and §3.4.6
// forbids a unit id to be carried across rounds, so a pair checked against the
// whole of either file would join a claim to a piece of code this round never
// formed.
//
// The checks run inside the decode rather than after it because
// state.DecodeStamped is the one place that counts lines, blank ones included,
// and §4.1.6's rejection has to name the line the user must open.
func Decode(file string, body []byte, claims, units []string) ([]*Pair, error) {
	against := pairChecker{file: file, claims: claims, units: units}
	return state.DecodeStamped[Pair](file, body, against.check)
}

// pairChecker holds what one file's pairs are checked against: the file they
// arrived in, and the round's claim and unit ids.
type pairChecker struct {
	file   string
	claims []string
	units  []string
}

// check holds one line to §4.1.6's two ids.
//
// The claim is settled before the unit because that is the order §4.1.6 writes
// them in and the order the pair reads in, so a line with both wrong is always
// reported by the same one. Presence is read off the wire rather than off the
// decoded pair, for the reason state.DecodeStamped gives about head and round:
// `""` is a value the agent chose exactly as much as an id is, and an empty
// claim would join a unit to nothing while looking like a mapping.
func (c pairChecker) check(line int, supplied map[string]json.RawMessage, pair *Pair) error {
	if err := c.id(line, "claim", supplied, pair.Claim, c.claims); err != nil {
		return err
	}
	return c.id(line, "unit", supplied, pair.Unit, c.units)
}

// id holds one of the pair's two fields to the ids the round fixed.
//
// Both are checked the same way because §4.1.6 rejects them in one clause —
// "rejecting an unknown claim or unit id with exit code 1" — and the reason is
// the same on each side. A pair naming an id nothing else in the round holds is
// a join to a row that does not exist: §4.1.3 would report an unimplemented
// claim that is mapped, or §4.1.2 an unmapped unit that is not.
func (c pairChecker) id(
	line int, field string, supplied map[string]json.RawMessage, named string, known []string,
) error {
	switch {
	case !written(supplied[field]):
		return &RejectedPairError{
			File: c.file, Line: line, Field: field,
			Problem: "is required by §4.1.6, which forms every mapping line as {claim, unit}",
		}
	case !slices.Contains(known, named):
		return &RejectedPairError{
			File: c.file, Line: line, Field: field,
			Problem: fmt.Sprintf(
				"names %q, which this round does not hold; §4.1.6 rejects an unknown %s id, "+
					"and the round holds %s",
				named, field, listed(known),
			),
		}
	}
	return nil
}

// listed renders the ids a round holds for a message, and says so when it holds
// none rather than trailing off after "holds".
func listed(values []string) string {
	if len(values) == 0 {
		return "none"
	}
	return strings.Join(values, ", ")
}

// The two values a JSON object can hold under a key and still supply nothing.
var (
	nullLiteral = []byte("null")
	emptyString = []byte(`""`)
)

// written reports whether a wire line supplied a field, reading null and the
// empty string as absent for the reason internal/intent's own written does: an
// empty id names nothing, and §4.1.6's requirement is about the id and not
// about the key.
func written(value json.RawMessage) bool {
	value = bytes.TrimSpace(value)
	return len(value) > 0 &&
		!bytes.Equal(value, nullLiteral) &&
		!bytes.Equal(value, emptyString)
}
