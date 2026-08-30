package intent

import (
	"encoding/json"
	"fmt"
	"strings"
)

// ClaimSource is the `source` row of §3.3's table: which text a claim was drawn
// from.
//
// It is a struct around an unexported name rather than the defined string type
// KeyOrigin above it uses, for the reason finding.State gives about §9.1's
// states. A defined string type is open — `var s ClaimSource = "spec"` compiles
// anywhere in the tree, because an untyped constant is assignable to it — so
// the vocabulary would be a convention rather than a fence. A struct whose only
// field is unexported can be built nowhere outside this file, so a source that
// is not one of the four below is unrepresentable, and adding one is an edit
// here.
//
// The fence earns more than tidiness. §3.3.1 and §3.3.2 send a claim to one of
// two validators on the strength of this field — the issue text for three of
// the four sources, the named note for the fourth — so a fifth source would be
// a claim neither rule checks. §3.3.2 also makes it the provenance §8.1.6
// discloses to the reader, and §3.3 draws the note-sourced claim's weaker
// standing from nothing else.
//
// The cost is that the four are `var` rather than `const`, since Go has no
// constant of struct type. Reassigning one is not widening the vocabulary — it
// is sabotage that breaks every test at once — and the hole it leaves is much
// smaller than the one it closes.
type ClaimSource struct{ name string }

// The four sources of §3.3's table, in the order the row names them. They are
// spelled ClaimFrom* rather than ClaimSource*, so they read the way KeyFromFlag
// and its siblings already do in this package.
var (
	// ClaimFromDescription is the issue description.
	ClaimFromDescription = ClaimSource{"description"}
	// ClaimFromAcceptance is the issue's acceptance criteria.
	ClaimFromAcceptance = ClaimSource{"acceptance"}
	// ClaimFromComment is a comment on the issue.
	ClaimFromComment = ClaimSource{"comment"}
	// ClaimFromNote is the context store of §3.6. It is the one source
	// §3.3.2 singles out: such a claim carries a `note_id`, has its span
	// validated against that note rather than the issue text, and rests on
	// unverified hearsay that §8.1.6 must disclose.
	ClaimFromNote = ClaimSource{"note"}
)

// claimSources is §3.3's set in the order the row writes it. It is the whole
// vocabulary: §3.3.1 and §3.3.2 partition it and add nothing to it.
var claimSources = []ClaimSource{
	ClaimFromDescription,
	ClaimFromAcceptance,
	ClaimFromComment,
	ClaimFromNote,
}

// String returns the source's name, which is what it goes by on the wire and in
// §3.3's table.
func (s ClaimSource) String() string {
	return s.name
}

// ClaimSources returns §3.3's four in row order. The result is a copy, so a
// caller can neither widen the set nor reorder it.
func ClaimSources() []ClaimSource {
	return append(make([]ClaimSource, 0, len(claimSources)), claimSources...)
}

// UnknownClaimSourceError reports a value that names no source of §3.3.
//
// It carries the value so the user can see what was rejected. `source` is a row
// the agent writes, so unlike an unknown record state this is an ordinary
// mistake in a file cr was handed rather than evidence of a hand-edited store.
type UnknownClaimSourceError struct {
	// Value is the offending name, exactly as it was written.
	Value string
}

func (e *UnknownClaimSourceError) Error() string {
	names := make([]string, 0, len(claimSources))
	for _, known := range claimSources {
		names = append(names, known.name)
	}
	return fmt.Sprintf("%q is not a claim source; §3.3 admits %s", e.Value, strings.Join(names, ", "))
}

// ParseClaimSource resolves a name into the source it goes by in §3.3's table.
// It is the only way into the type from a string, so every value that exists
// came either from a variable above or through this check.
func ParseClaimSource(name string) (ClaimSource, error) {
	for _, known := range claimSources {
		if known.name == name {
			return known, nil
		}
	}
	return ClaimSource{}, &UnknownClaimSourceError{Value: name}
}

// MarshalJSON writes the source's name.
func (s ClaimSource) MarshalJSON() ([]byte, error) {
	return json.Marshal(s.name)
}

// UnmarshalJSON reads a source back through ParseClaimSource, so a name outside
// §3.3 is refused at the file rather than carried around as a value nothing can
// act on.
//
// null and "" are left alone, and the difference from finding.State's reading
// of null is the difference between the two rows. `state` is computed, so a
// line supplying it at all is refused by §6.1.4's fence and failing here would
// answer with the wrong error. `source` is required, and §3.3's required-field
// walk reads null and "" as supplying nothing — so a claim holding either has
// no source, and reporting it as an unknown source would name the wrong fault
// and point the user at the vocabulary instead of at the missing row.
func (s *ClaimSource) UnmarshalJSON(data []byte) error {
	if !written(data) {
		return nil
	}
	var name string
	if err := json.Unmarshal(data, &name); err != nil {
		return err
	}
	parsed, err := ParseClaimSource(name)
	if err != nil {
		return err
	}
	*s = parsed
	return nil
}
