package review

import (
	"path/filepath"

	"github.com/deligoez/cr/internal/finding"
	"github.com/deligoez/cr/internal/gh"
	"github.com/deligoez/cr/internal/git"
	"github.com/deligoez/cr/internal/intent"
	"github.com/deligoez/cr/internal/mapping"
	"github.com/deligoez/cr/internal/note"
	"github.com/deligoez/cr/internal/reinvention"
	"github.com/deligoez/cr/internal/role"
	"github.com/deligoez/cr/internal/rule"
	"github.com/deligoez/cr/internal/testadequacy"
	"github.com/deligoez/cr/internal/unit"
)

// Unit is one unit of the round as §3.4.6 recorded it, with the hunks the diff
// at the round's head gives it and, index for index, each hunk's text.
//
// The record comes from units.ndjson and the hunks from the diff, and both are
// needed. The record is what every other command of the round checks a unit id
// against, so the prompt names the unit the round actually holds; the hunks are
// what the role reads, and a record carries only their ranges.
type Unit struct {
	unit.Unit
	// Hunks are the unit's hunks, in the order the diff gave them.
	Hunks []git.Hunk
	// Texts are those hunks' texts, per git.HunkTexts.
	Texts []string
	// FanOut is the directory §4.6.2's output files for this unit sit in,
	// per state.Layout.FanOutDir.
	FanOut string
}

// Round is everything §4.6.1 attaches, gathered once for one round and read by
// every prompt the round emits.
//
// The attachments are each computed over the whole round by the package that
// owns them and narrowed to a unit only here, so no prompt can be built from an
// attachment another prompt of the same round did not see.
type Round struct {
	// Round and Head are the round the prompts are emitted for.
	Round int
	Head  string
	// Roles are the round's active roles of §4.5.1, in §2.5.5's corpus
	// order, which is the order the prompts are emitted in. `--axis`
	// narrows them per §4.6.5.
	Roles []role.Role
	// Active are the ids of §4.5.1's active set, whole and in the same
	// corpus order, whatever `--axis` narrowed the prompts to. §10.2.2
	// counts a row against every active role, so §4.6.3's expected set is
	// built from this rather than from Roles.
	Active []string
	// Units are the round's units, in §3.4.6's id order.
	Units []Unit
	// Claims are the claims recorded for the round.
	Claims []intent.Claim
	// Pairs is mapping.ndjson whole.
	Pairs []mapping.Pair
	// Mapped reports whether a mapping has been recorded for the round at
	// all. Before one is, no unit is known to be mapped to nothing: §4.6.5
	// says unmapped-ness is unknowable on the first pass.
	Mapped bool
	// SecondPass reports whether this invocation is §4.6.5's second intent
	// pass: the intent axis re-run once the mapping is stored. It narrows
	// the prompts to the units the mapping maps to zero claims, which is
	// what that pass exists to emit — §4.1.2's item and §4.1.5's notes have
	// nothing to say about a unit a claim covers.
	//
	// It is separate from Mapped because the two answer different
	// questions. Mapped is a fact about the round and shapes what every
	// prompt says about the mapping; this is a fact about the invocation
	// and shapes which units get one at all.
	SecondPass bool
	// Candidates are §4.3.1's attachments over the round's diff.
	Candidates reinvention.Attachments
	// Rules is the round's rule corpus of §2.6, which §2.6.1.4 injects into
	// its axis role's prompt where a rule carries no detector.
	Rules []rule.Resolved
	// Hits are §4.3.6's attachments, one per unit in Units' order.
	Hits []rule.Attachment
	// Tests are §4.4.1's attachments, one per unit in Units' order.
	Tests []testadequacy.Attachment
	// Threads are the ingested threads of §3.5, whole.
	Threads []gh.Thread
	// Proximity is `threads.proximity_lines`, §3.5.3's window.
	Proximity int
	// Notes are the issue key's notes that still stand.
	Notes []note.Note
	// Unmapped are §4.1.2's items that are still raised.
	Unmapped []UnmappedUnit
}

// Prompt is one prompt §4.6.1 emits: one active role over one unit.
//
// The attachments are carried in the text rather than beside it. The text is
// what an agent runs, so it is the one place every attachment has to be, and a
// structured copy alongside it would be a second rendering of the same facts
// that a reader could consult while the agent read the other.
type Prompt struct {
	// Role is the role's id.
	Role string `json:"role"`
	// Axis is the axis the role serves.
	Axis string `json:"axis"`
	// Unit is the unit's id.
	Unit string `json:"unit"`
	// Output is the NDJSON path §4.6.2 has the role write its records to,
	// which the text names as well.
	Output string `json:"output"`
	// Text is the prompt itself.
	Text string `json:"prompt"`
}

// Emit is §4.6.1: for every active role and every unit, one prompt, role by
// role in corpus order and unit by unit in id order within each.
//
// It forms no judgement, and its inputs are why it cannot. Every attachment was
// located by the package that owns it — candidates ranked by §4.3.2's formula,
// hits matched by §2.6.1's detectors, test files read off §2.4's globs, threads
// placed by §3.5.3's window — and the verdict each invites is left to the agent
// reading the prompt, per §2.1.3. Nothing here reads a hunk, a claim, or a note
// for what it means.
func Emit(r *Round) []Prompt {
	prompts := make([]Prompt, 0, len(r.Roles)*len(r.Units))
	for i := range r.Roles {
		lens := &r.Roles[i]
		for at := range r.Units {
			if !r.emits(r.Units[at].ID) {
				continue
			}
			output := filepath.Join(r.Units[at].FanOut, finding.FanOutFile(lens.ID))
			prompts = append(prompts, Prompt{
				Role:   lens.ID,
				Axis:   lens.Axis,
				Unit:   r.Units[at].ID,
				Output: output,
				Text:   r.text(lens, at, output),
			})
		}
	}
	return prompts
}

// emits reports whether the unit gets a prompt on this invocation.
//
// Every unit does, except on §4.6.5's second intent pass, which re-emits one
// prompt per unit the round's mapping maps to zero claims. Which units those
// are is mapping.ClaimsOf's answer rather than Round.Unmapped's: §4.1.5 leaves
// an item unraised once a note explains the unit, and that unit is still one
// the mapping maps to nothing — dropping it here would take away the prompt
// carrying the note the agent is meant to weigh.
func (r *Round) emits(id string) bool {
	return !r.SecondPass || len(mapping.ClaimsOf(r.Pairs, r.Round, id)) == 0
}
