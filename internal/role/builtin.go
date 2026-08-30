package role

import _ "embed"

// The four roles §2.5.1 requires v0.1 to ship, one per axis of §1.5. Each is
// embedded rather than built from a struct literal, because §2.5.2 requires
// `cr init --eject-roles` to write defaults that are byte-identical to the
// built-ins: a role assembled in Go and marshalled back out could only promise
// whatever the encoder chose that day, not the reviewed file in this
// repository.
//
// Each file carries a lens and nothing else. §4.3.5 puts project conventions in
// the rule corpus of §2.6 and never in prose buried inside role instructions,
// and the division earns its keep rather than tidying anything: a standard
// written here ships to every repository cr reviews, where nobody can review,
// scope, or retire it, while the same sentence written as a rule is versioned
// with the project, carries the rationale §2.6 item 4 lets a record quote, and
// is prunable once §2.6.3 finds it dead.
//
// The prose also has to hold §6.3's register. Wording that pushes a role
// towards asserting buys it nothing — cr computes the grade from the record and
// forces an argued one to a question regardless — while it costs the reviewer
// who wrote as though certain and is then read as hedging, which is exactly the
// trust §1.6 says is spent once.

// intentCoverageID is the id, and therefore the file stem, of the role §2.5.1
// puts on the intent axis.
const intentCoverageID = "intent-coverage"

//go:embed builtin/intent-coverage.json
var intentCoverage string

// correctnessID is the id, and file stem, of the role on the correctness axis.
const correctnessID = "correctness"

//go:embed builtin/correctness.json
var correctness string

// conventionID is the id, and file stem, of the role on the convention axis.
const conventionID = "convention"

//go:embed builtin/convention.json
var convention string

// testAdequacyID is the id, and file stem, of the role on the test axis. The id
// and the axis differ on purpose: §2.5.1 names the role `test-adequacy` and
// §1.5 closes the axis set at `test`, so the two are not interchangeable and
// neither may be spelled as the other.
const testAdequacyID = "test-adequacy"

//go:embed builtin/test-adequacy.json
var testAdequacy string

// Builtins returns the role files cr ships, keyed by role id, for §2.5.2's
// eject and the built-in layer of §2.5.4. The values are file contents rather
// than parsed roles, because writing them out unchanged is the whole point and
// a round trip through Role would not reproduce them.
//
// The map is rebuilt per call so no caller can edit the shipped set.
func Builtins() map[string]string {
	return map[string]string{
		intentCoverageID: intentCoverage,
		correctnessID:    correctness,
		conventionID:     convention,
		testAdequacyID:   testAdequacy,
	}
}
