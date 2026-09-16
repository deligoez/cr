package intent

import "github.com/deligoez/cr/internal/state"

// Requirement is what §3.3's Required column says about one row.
//
// It is the shape internal/finding holds §6.1's column in, and deliberately not
// the same declaration. The two tables are two contracts: §3.3's rows decide
// what `cr claims record` accepts and §6.1's decide what `cr record` accepts,
// and sharing a type would make one section's field vocabulary an import of the
// other's — so a row moved in §6.1 would arrive here as a compile error about a
// table it does not govern.
type Requirement int

const (
	// Optional marks a row the column answers "no".
	Optional Requirement = iota
	// Required marks a row the column answers "yes": a claim missing it is
	// rejected with exit code 1, naming the line and the field.
	Required
	// Computed marks a row the column answers "computed": cr writes it,
	// and a claim arriving with it is rejected with exit code 1. §3.3.1
	// says so of both in as many words, and §6.1.4 names the same two.
	Computed
	// Stamped marks the row the column answers "yes" but §2.3.3 takes out
	// of the agent's hands. head is required of a stored claim and refused
	// on the wire: state's stamped writers write it on every write and
	// state.DecodeStamped rejects a line carrying it, so a validator that
	// demanded it of an agent's line would demand the one thing that file
	// may not contain.
	Stamped
)

// ClaimField is one row of §3.3's field table: the name it goes by on the wire,
// and what the table's Required column says about it.
type ClaimField struct {
	// Name is the JSON key, which is how a wire line names the field
	// before anything about that line is trusted.
	Name string
	// Requirement is the row's answer in the Required column.
	Requirement Requirement
}

// claimFields is §3.3's table, in the order §3.3 lists the rows. It is held
// here as data rather than restated at each call site, so the required-field
// rejection and the computed-field rejection cannot come to different
// conclusions about the same row, and a row added to the spec is added once.
//
// `round` is absent because §3.3's table has no such row. §2.3.3 requires it on
// every record of claims.ndjson all the same, and state.Stamp carries it beside
// head; state.DecodeStamped fences the pair together, so nothing is lost by
// this table describing only what §3.3 wrote.
var claimFields = []ClaimField{
	{Name: "id", Requirement: Required},
	{Name: "text", Requirement: Required},
	{Name: "source", Requirement: Required},
	{Name: "span", Requirement: Required},
	{Name: "note_id", Requirement: Optional},
	{Name: "file", Requirement: Optional},
	{Name: "span_hash", Requirement: Computed},
	{Name: "issue_hash", Requirement: Computed},
	{Name: "head", Requirement: Stamped},
}

// ClaimFields returns §3.3's rows in table order. The result is a copy, so a
// caller can neither widen the table nor reorder it.
func ClaimFields() []ClaimField {
	return append(make([]ClaimField, 0, len(claimFields)), claimFields...)
}

// Claim is one record of claims.ndjson: something the issue asks for, drawn by
// the agent from a named span of a named text and recorded by cr (§3.3).
//
// The field order is §3.3's table's, and claimFields says of each row whether
// the agent must supply it, may supply it, or may never supply it. The struct
// is the resolved claim — what claims.ndjson holds once cr has written the
// computed fields. It is not the wire form: a line an agent hands in is a JSON
// object whose keys are read before any of it is trusted, exactly as
// state.DecodeStamped already refuses a line carrying head or round.
//
// Nothing here validates a whole claim. §3.3.1's span-occurs-in-the-issue-text
// rule, §3.3.2's validation against the named note, and §3.3.3's drift check all
// read this schema; none of them lives in it, because each needs a text this
// struct does not carry.
type Claim struct {
	// ID is §3.3's `<ISSUE-KEY>#c<n>`. It is two things at once: a
	// per-issue counter, and a foreign key into the key §3.2 resolved.
	// SplitClaimID reads the second half back out.
	ID string `json:"id"`
	// Text is the claim, verbatim or minimally normalised. It is the
	// agent's wording of what the issue asks for, and Span below is where
	// that wording came from.
	Text string `json:"text"`
	// Source is which text the claim was drawn from, and so which of
	// §3.3.1 and §3.3.2 validates it.
	Source ClaimSource `json:"source"`
	// Span is the verbatim substring of that text the claim was drawn
	// from. §3.3.1 requires it to occur in the issue text, and §3.3.2 sets
	// it to the note's body when Source is ClaimFromNote.
	Span string `json:"span"`
	// NoteID is the note the claim came from. §3.3 requires it when Source
	// is ClaimFromNote, and a claim drawn out of the issue text carries no
	// note to name.
	NoteID string `json:"note_id,omitempty"`
	// File is the extra intent file of §3.1.5 the claim came from, as the
	// separator line above its text names it. §3.3 requires it when Source
	// is ClaimFromFile, and a claim drawn from the tracker's own text or
	// from a note carries no file to name.
	File string `json:"file,omitempty"`
	// SpanHash is the normalised hash of Span, written by cr. omitempty
	// because a claim that has not been through ComputeClaimHashes has no
	// hash, and the key is left off rather than written empty.
	SpanHash string `json:"span_hash,omitempty"`
	// IssueHash is the normalised hash of the whole issue text at
	// extraction, written by cr. §3.3.3 compares it against a fresh hash
	// of the issue at the start of every round to detect drift.
	IssueHash string `json:"issue_hash,omitempty"`
	// Stamp carries head, the last row of §3.3's table, together with the
	// round §2.3.3 adds. It is embedded rather than spelled out because
	// the pair has one author and it is never the agent: the writer sets
	// it on the way out, so a claim cannot reach claims.ndjson through any
	// path that skips it.
	state.Stamp
}
