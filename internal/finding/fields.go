package finding

import "slices"

// Requirement is what §6.1's Required column says about one row.
//
// The column is not the same question as "may the agent write this". §6.1.4
// names fields of its own that no agent may supply — disposition, thread_id and
// duplicate_of among them — and the column calls all three optional. reserved
// below is §6.1.4's own list, and it reads this column as well; Optional here
// means only that a record without the field is complete.
type Requirement int

const (
	// Optional marks a row the column answers "no".
	Optional Requirement = iota
	// Required marks a row the column answers "yes": a record missing it is
	// rejected with exit code 1, naming the line and the field (§6.1.3).
	Required
	// Computed marks a row the column answers "computed": cr writes it, and
	// a record arriving with it is rejected with exit code 1 (§6.1.4).
	Computed
	// Stamped marks the two rows the column answers "yes" but §2.3.3 takes
	// out of the agent's hands. head and round are required of a stored
	// record and refused on the wire: state.WriteStamped writes them on
	// every write and state.DecodeStamped rejects a line carrying either,
	// so a validator that demanded them of an agent's line would demand the
	// one thing that file may not contain.
	Stamped
)

// Field is one row of a record's field table: the name it goes by on the wire,
// and what the table's Required column says about it.
type Field struct {
	// Name is the JSON key, which is how a wire line names the field before
	// anything about that line is trusted.
	Name string
	// Requirement is the row's answer in the Required column.
	Requirement Requirement
}

// fields is §6.1's table, in the order §6.1 lists the rows. It is held here as
// data rather than restated at each call site, so §6.1.3's required-field
// rejection and §6.1.4's computed-field rejection cannot come to different
// conclusions about the same row, and a row added to the spec is added once.
//
// finding_test.go's specFields is a second transcription of the same table, and
// the two being identical is the point rather than an oversight: the test reads
// §6.1 independently and asserts this copy matches. Collapsing the duplication
// would leave the table checked against itself.
//
//nolint:dupl // deliberate: the twin in finding_test.go is an independent transcription
var fields = []Field{
	{Name: "id", Requirement: Required},
	{Name: "kind", Requirement: Required},
	{Name: "axis", Requirement: Computed},
	{Name: "role", Requirement: Required},
	{Name: "class", Requirement: Required},
	{Name: "rule", Requirement: Optional},
	{Name: "severity", Requirement: Required},
	{Name: "grade", Requirement: Computed},
	{Name: "unit", Requirement: Required},
	{Name: "claim", Requirement: Optional},
	{Name: "anchor", Requirement: Required},
	{Name: "summary", Requirement: Required},
	{Name: "evidence", Requirement: Required},
	{Name: "citations", Requirement: Optional},
	{Name: "probe", Requirement: Optional},
	{Name: "suggestion", Requirement: Optional},
	{Name: "suggestion_origin", Requirement: Optional},
	{Name: "state", Requirement: Computed},
	{Name: "disposition", Requirement: Optional},
	{Name: "duplicate_of", Requirement: Optional},
	{Name: "suppressed_by", Requirement: Optional},
	{Name: "thread_id", Requirement: Optional},
	{Name: "round", Requirement: Stamped},
	{Name: "head", Requirement: Stamped},
}

// citationFields is the field set of one entry of §6.1's citations array.
//
// §6.1 gives an entry no Required column of its own. It names four fields and
// computes two of them, which leaves the other two to the agent, and an entry
// without a path and a line points at nothing — a citation exists to be opened
// by a human (§6.2.6), and half of one cannot be.
var citationFields = []Field{
	{Name: "path", Requirement: Required},
	{Name: "line", Requirement: Required},
	{Name: "content_hash", Requirement: Computed},
	{Name: "origin", Requirement: Computed},
}

// alsoReserved are the fields §6.1.4 forbids that §6.1's Required column does
// not mark computed. `cr` writes all three: §7.2 sets `disposition` at triage,
// §8.4 sets `thread_id` at posting, and §6.4.3 sets `duplicate_of` when it
// suppresses a duplicate — which §6.5.1 lets `cr merge` carry in its output and
// lets nothing else write anywhere.
//
// `suppressed_by` is deliberately absent. §3.5.4 has the **agent** decide
// whether an attached thread already covers a finding, so that field holds the
// agent's judgement rather than `cr`'s, which is exactly what `duplicate_of`
// does not.
var alsoReserved = []string{"disposition", "duplicate_of", "thread_id"}

// claimHashes are §3.3's two computed claim fields, which §6.1.4 names in the
// same sentence as its own. They are rows of §3.3's table, not §6.1's, so a
// finding line carrying one carries a key no finding row can hold: §6.1.4
// forbids it in as many words, no finding has a use for either, and refusing
// them takes nothing from a record it may keep. `cr claims record` reads §3.3's
// table and refuses them there on its own account.
var claimHashes = []string{"span_hash", "issue_hash"}

// reserved is §6.1.4's fence: every field a record may not arrive carrying,
// in §6.1's table order with §3.3's two last, so a record overstepping in
// several places is always reported by the same one.
//
// Most of it is derived from the Required column rather than restated, so a row
// the spec marks computed joins the fence by being marked and the two readings
// of the table cannot come to different conclusions about the same row.
var reserved = reservedFields()

// reservedFields builds that list once, at package initialisation.
//
// One mutant survives here and is left deliberately: the arithmetic in the
// capacity hint changes only how much is allocated up front, and a test written
// to kill it would assert an implementation detail.
func reservedFields() []string {
	names := make([]string, 0, len(fields)+len(claimHashes))
	for _, field := range fields {
		if field.Requirement == Computed || slices.Contains(alsoReserved, field.Name) {
			names = append(names, field.Name)
		}
	}
	return append(names, claimHashes...)
}

// Fields returns §6.1's rows in table order. The result is a copy, so a caller
// can neither widen the table nor reorder it.
func Fields() []Field {
	return append(make([]Field, 0, len(fields)), fields...)
}

// CitationFields returns the rows of one citations entry, under the same rule.
func CitationFields() []Field {
	return append(make([]Field, 0, len(citationFields)), citationFields...)
}
