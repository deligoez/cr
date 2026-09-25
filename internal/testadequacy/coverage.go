package testadequacy

import (
	"encoding/json"
	"fmt"
	"slices"
	"strings"
)

// Classification is §4.4.1's verdict on one unit. §4.4.1 fixes the vocabulary
// at three words and the set is closed: it is a literal below, and a value
// outside it is rejected at the point a cell is decoded.
type Classification string

// The three classifications of §4.4.1, in the order it lists them.
const (
	// Covered is a unit the changed tests exercise.
	Covered Classification = "covered"
	// PartiallyCovered is a unit they exercise in part.
	PartiallyCovered Classification = "partially-covered"
	// Uncovered is a unit they do not exercise.
	Uncovered Classification = "uncovered"
)

// classifications is the closed set. There is no registration entry point and
// no configuration key names a classification, so no layer can widen it.
var classifications = []Classification{Covered, PartiallyCovered, Uncovered}

// Classifications is the closed set, in the order §4.4.1 lists it, for the
// round's contract file to name.
func Classifications() []Classification {
	return slices.Clone(classifications)
}

// Coverage is the `coverage` value §4.5.5 puts on a test-axis cell: the
// classification the agent made, and the test paths it rested on.
//
// Both fields are unexported and this package offers no constructor that takes
// a classification, so the only way a Coverage carrying one comes into existence
// is UnmarshalJSON reading the agent's own line. §2.1.3 lists "whether a unit is
// covered by tests" among the judgements that belong to the agent alone, and
// this is that rule made structural rather than remembered: cr cannot fill the
// field in — not by mistake, not helpfully, and not as a default — because there
// is no exported way to put a value there. Attach supplies the candidate
// evidence and this records the decision.
//
// It is the device state.Stamp uses, pointed the other way. There, setStamp is
// unexported so no type outside that package can claim to be stamped; here, the
// classification is unexported so no code outside a decoded agent line can claim
// to have classified.
type Coverage struct {
	classification Classification
	testPaths      []string
}

// wireCoverage is the JSON shape of §4.5.5's `coverage` value. It is a separate
// type on purpose: exporting these fields on Coverage itself would hand cr back
// the setter the unexported fields exist to withhold, and this one is reachable
// only from the two methods below.
type wireCoverage struct {
	Classification Classification `json:"classification"`
	TestPaths      []string       `json:"test_paths"`
}

// UnmarshalJSON decodes the agent's coverage value, rejecting a classification
// outside §4.4.1's three.
//
// An unknown value is refused rather than carried, because a cell is what §10.2
// reads completeness out of: a classification cr does not recognise is a cell
// whose verdict nothing downstream can act on, and accepting it would let a
// round report itself complete on a word no rule in the spec defines. The empty
// string is one such value, so a coverage value that omits the classification is
// refused too.
func (c *Coverage) UnmarshalJSON(data []byte) error {
	var w wireCoverage
	if err := json.Unmarshal(data, &w); err != nil {
		return err
	}
	if !slices.Contains(classifications, w.Classification) {
		return &InvalidClassificationError{Value: string(w.Classification)}
	}
	c.classification = w.Classification
	c.testPaths = paths(w.TestPaths)
	return nil
}

// MarshalJSON writes §4.5.5's `coverage` value back out, with `test_paths` as an
// empty array rather than null when the agent rested on none.
func (c Coverage) MarshalJSON() ([]byte, error) {
	return json.Marshal(wireCoverage{Classification: c.classification, TestPaths: c.TestPaths()})
}

// Classification returns the verdict the agent recorded.
func (c Coverage) Classification() Classification {
	return c.classification
}

// TestPaths returns the test paths the verdict rested on. The result is a copy,
// so a caller cannot reach through it into a recorded cell.
func (c Coverage) TestPaths() []string {
	return paths(c.testPaths)
}

// paths copies a path list and turns an absent one into an empty one, so no
// coverage value ever serialises a slice as null.
func paths(values []string) []string {
	return append(make([]string, 0, len(values)), values...)
}

// InvalidClassificationError reports a coverage value whose classification is
// not one of §4.4.1's three. It carries the value so the user can see what was
// rejected. The cli layer maps it onto exit code 1: the file was read and
// parsed, and what is wrong is the data inside it.
type InvalidClassificationError struct {
	// Value is the offending classification, exactly as it was written.
	Value string
}

func (e *InvalidClassificationError) Error() string {
	names := make([]string, 0, len(classifications))
	for _, c := range classifications {
		names = append(names, string(c))
	}
	return fmt.Sprintf(
		"classification %q is not one §4.4.1 defines; write one of %s",
		e.Value, strings.Join(names, ", "),
	)
}
