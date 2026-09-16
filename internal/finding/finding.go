// Package finding models the review record of spec/0.1.0.md §6.1.
//
// One record is one finding or one question, and it is what cr merges,
// deduplicates, grades, drafts, and posts. §6.1's field table is the whole
// surface, and its Required column is where the trust economy is enforced: the
// column separates what the agent may write from what cr writes itself, so
// §6.2 can compute a grade from the record and §6.3 can force an argued record
// to a question without either depending on the agent's cooperation.
//
// The struct below is the resolved record — what findings.ndjson holds once cr
// has written the computed fields. It is not the wire form. A line an agent
// hands in is a JSON object whose keys are read before any of it is trusted,
// exactly as state.DecodeStamped already refuses a line carrying head or round,
// and fields.go holds §6.1's table as data so a validator checks those keys
// against the table rather than restating it.
//
// Nothing here validates a whole record. §6.1.3's required-field rejection,
// §6.1.4's computed-field rejection, §6.2's grade computation, §6.3's forcing,
// and §6.4's dedup all read this schema; none of them lives in it.
package finding

import (
	"github.com/deligoez/cr/internal/git"
	"github.com/deligoez/cr/internal/state"
)

// Kind is the register the record is written in.
//
// It is not a label on a fixed body: §6.3 forces every record graded argued to
// a question, and §8.1.4 gives a question its own cr-owned line, so this field
// decides whether the record reaches the author as an assertion or as a
// request. P2 and P3 turn on it.
type Kind string

const (
	// KindFinding asserts something about the code.
	KindFinding Kind = "finding"
	// KindQuestion asks about it instead.
	KindQuestion Kind = "question"
)

// kinds is §6.1's two registers, the closed set the decoder holds a line's kind
// to.
var kinds = []Kind{KindFinding, KindQuestion}

// Kinds returns §6.1's two registers. The result is a copy, so a caller can
// neither widen the set nor reorder it.
//
// It is the list the decoder refuses by, handed out rather than restated, so a
// prompt telling a role which values kind takes (§4.6.2) names exactly the
// values `cr merge` and `cr record` accept.
func Kinds() []Kind {
	return append(make([]Kind, 0, len(kinds)), kinds...)
}

// Severity is the four-value scale of §6.1, ordered critical, high, medium,
// low. §6.4.2 compares two records by it when picking a duplicate group's
// representative.
type Severity string

const (
	// SeverityCritical is the highest of the four.
	SeverityCritical Severity = "critical"
	// SeverityHigh is the second.
	SeverityHigh Severity = "high"
	// SeverityMedium is the third.
	SeverityMedium Severity = "medium"
	// SeverityLow is the lowest.
	SeverityLow Severity = "low"
)

// Severities returns §6.1's four severities, highest first. The result is a
// copy, so a caller can neither widen the set nor reorder it.
//
// The order is severityStrength's, which §6.4.2 ranks a duplicate group by, so
// the vocabulary a caller holds a value to and the order cr compares two
// records by cannot come apart. §7.2 needs the first half: it makes `severity`
// freely editable in a draft marker, and "freely" is about cr forming no
// opinion of its own rather than about the vocabulary — a fifth word is not a
// severity the reviewer chose but one nothing downstream can rank.
func Severities() []Severity {
	return append(make([]Severity, 0, len(severityStrength)), severityStrength...)
}

// Grade is the evidence grade of §6.2, computed by cr from the record and never
// asserted by the agent. §6.2's own rules decide which of the three a record
// earns.
type Grade string

const (
	// GradeProbed rests on an experiment cr ran and recorded.
	GradeProbed Grade = "probed"
	// GradeCited rests on a location cr resolved against the head.
	GradeCited Grade = "cited"
	// GradeArgued rests on neither, and §6.3 forces it to a question.
	GradeArgued Grade = "argued"
)

// Grades returns §6.2's three grades, strongest first. The result is a copy, so
// a caller can neither widen the set nor reorder it.
//
// It is Severities' counterpart and reads the same list §6.4.2 ranks a
// duplicate group's representative by, so a report tallying records by grade
// and the comparison that picks a representative cannot come to hold different
// vocabularies. §10.1.4 is the first caller.
func Grades() []Grade {
	return append(make([]Grade, 0, len(gradeStrength)), gradeStrength...)
}

// Origin names what produced a piece of a record: cr's own rule machinery, or
// the agent.
//
// §6.1's suggestion_origin and §6.2.5's citation origin take the same two
// values and answer the same question about the same record, so they share one
// type. Both are cr's answer rather than the agent's: §6.2.5 rejects a citation
// arriving with an origin at all, and §7.1.3 labels a rule-produced suggestion
// as machine generated on the strength of it.
type Origin string

const (
	// OriginAgent marks what the reviewing agent produced.
	OriginAgent Origin = "agent"
	// OriginRule marks what a rule of §2.6 produced.
	OriginRule Origin = "rule"
)

// origins is the two values §6.1's suggestion_origin row names.
var origins = []Origin{OriginAgent, OriginRule}

// Origins returns those two values, under the rule Kinds gives: the result is a
// copy of the list the decoder refuses a suggestion_origin by.
func Origins() []Origin {
	return append(make([]Origin, 0, len(origins)), origins...)
}

// Disposition is why a record was discarded during triage (§7.2). The two are
// deliberately distinct: they write waivers of different scope, and only wrong
// counts against the class in §7.3's statistics.
type Disposition string

const (
	// DispositionWrong marks a false positive.
	DispositionWrong Disposition = "wrong"
	// DispositionNotHere marks a record that is not wrong but does not
	// belong on this pull request.
	DispositionNotHere Disposition = "not-here"
)

// Citation is one entry of §6.1's citations array: a location in the code that
// a human can open.
//
// It is the machine-readable input to grading, and the whole of what a cited
// grade rests on. §6.2.4 fixes how little that is — record-time validation
// establishes that the location exists, never that it supports the summary — so
// §6.2.6 renders every citation of a cited record verbatim into the draft
// block, leaving the judgement to the human.
//
// A citation carries no side. §6.1 resolves every one of them against the
// current head, so a side would be a second answer to a question already
// settled, and the wrong answer would move a location across §6.2.1's unit
// boundary.
type Citation struct {
	// Path is the file the citation points into.
	Path string `json:"path"`
	// Line is its line, numbered in the current head.
	Line int `json:"line"`
	// ContentHash is the normalised hash of that line, computed by cr at
	// record time per §6.2.3. It is recorded so a v0.3 migration can detect
	// drift and does no validating work in v0.1.
	ContentHash string `json:"content_hash,omitempty"`
	// Origin is stamped by cr per §6.2.5 by matching the citation
	// positionally against cr's own detection output.
	Origin Origin `json:"origin,omitempty"`
}

// Anchor binds a record to a code location, per §9.2. Every record carries one:
// an item with no code location never becomes a record (§6.1.2, §4.1.3).
//
// The fence is §1.6's as much as §6.1's. A review comment spends the
// reviewer's standing with the author, so volume is a cost in itself,
// independent of correctness — and an unanchored comment costs the most for
// the least, because the author has to work out what it is about before they
// can judge whether it is right. So v0.3 has one comment channel and this
// field is it: an item cr cannot point at is reported by `cr status` (§10.1.2)
// and never posted (§1.6.1).
//
// §9.2 owns the rest of the anchor's rules — the pre-image of the content hash,
// the bound on the context window, the ordering of the two line numbers, and
// the tree each side resolves against — and they are enforced with the anchor
// rather than here. This is the field set §6.1's anchor row points at, so a
// record can hold one losslessly.
type Anchor struct {
	// Path is the file the record is about.
	Path string `json:"path"`
	// Side is the file version the two line numbers are counted in: RIGHT
	// in the head, LEFT in the merge base (§9.2.1).
	Side git.Side `json:"side"`
	// StartLine is the first line of the anchored range.
	StartLine int `json:"start_line"`
	// Line is its last line, inclusive.
	Line int `json:"line"`
	// ContentHash is the normalised hash of the anchored lines taken as one
	// text, recorded for a v0.3 migration per §9.2.3.
	ContentHash string `json:"content_hash"`
	// ContextBefore holds up to three lines above the range, and
	// ContextAfter up to three below, recorded for the same reason.
	ContextBefore []string `json:"context_before,omitempty"`
	ContextAfter  []string `json:"context_after,omitempty"`
}

// Finding is one record of findings.ndjson: a finding or a question in any of
// §9.1's states, holding every row of §6.1's table.
//
// The field order is the table's, and fields.go says of each row whether the
// agent must supply it, may supply it, or may never supply it. Head and round
// arrive last because they arrive differently: they come from the embedded
// state.Stamp, which the writer sets on the way out (§2.3.3), so this type
// cannot be written to findings.ndjson through any path that skips them.
type Finding struct {
	// ID is f<n>, stable for the life of the pull request. See NextID: a
	// unit id is u<n> and round-scoped, and the two rules are opposites.
	ID string `json:"id"`
	// Kind is the register, forced to KindQuestion by §6.3 when Grade is
	// GradeArgued.
	Kind Kind `json:"kind"`
	// Axis is computed: the axis of Role, written by cr. §6.1.4 covers it
	// under "any other field marked computed", so an agent that supplied it
	// could choose the axis its own role is graded on — and §6.2's cited
	// grade turns on the axis not being test.
	Axis string `json:"axis,omitempty"`
	// Role is the role that produced the record.
	Role string `json:"role"`
	// Class is the kebab-case defect class, held to §6.1's form by
	// ValidateClass.
	Class string `json:"class"`
	// Rule is the rule id, when a rule of §2.6 produced the record.
	Rule string `json:"rule,omitempty"`
	// Severity is the record's severity.
	Severity Severity `json:"severity"`
	// Grade is computed per §6.2 and never asserted by the agent.
	Grade Grade `json:"grade,omitempty"`
	// Unit is the unit id the record sits on.
	Unit string `json:"unit"`
	// Claim is the claim id, when the axis produces one.
	Claim string `json:"claim,omitempty"`
	// Anchor is the code location, per §9.2.
	Anchor Anchor `json:"anchor"`
	// Summary is one English sentence, structured for dedup. §6.1.1 keeps
	// it English: the reader-facing prose is produced at draft time per
	// §8.1, and this is the text cr's own machinery reads.
	Summary string `json:"summary"`
	// Evidence is English prose explaining what supports the record, under
	// the same rule as Summary.
	Evidence string `json:"evidence"`
	// Citations are the resolved locations §6.2 grades from.
	Citations []Citation `json:"citations,omitempty"`
	// Probe is the probe id, when graded GradeProbed.
	Probe string `json:"probe,omitempty"`
	// Suggestion holds exact replacement lines.
	Suggestion string `json:"suggestion,omitempty"`
	// SuggestionOrigin says what produced them.
	SuggestionOrigin Origin `json:"suggestion_origin,omitempty"`
	// State is computed, written only by the commands of §9.1, and holds
	// one of state.go's seven. omitzero rather than omitempty because a
	// State is a struct: a record that has been through no §9.1 transition
	// carries no state, and the key is left off rather than written empty.
	State State `json:"state,omitzero"`
	// Disposition is set once the record is discarded (§7.2).
	Disposition Disposition `json:"disposition,omitempty"`
	// DuplicateOf names the representative when §6.4.3 suppressed this
	// record as a duplicate. It is the one computed field §6.5.1 lets cr
	// merge write into its output.
	DuplicateOf string `json:"duplicate_of,omitempty"`
	// SuppressedBy names the existing human thread that already covers the
	// record, per §3.5.4.
	SuppressedBy string `json:"suppressed_by,omitempty"`
	// ThreadID is the GitHub thread id, once posted.
	ThreadID string `json:"thread_id,omitempty"`
	// Stamp carries round and head, the last two rows of §6.1's table.
	state.Stamp
}
