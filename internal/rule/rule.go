// Package rule models the rule file of spec/0.1.0.md §2.6.
//
// A rule is one normative statement about how code in a repository must be
// written. §2.6 keeps rules and roles apart on purpose: a role is a lens, and a
// rule is a specific standard that lens enforces. So a rule is data — a set of
// fields cr reads mechanically — and never prompt text with a schema wrapped
// around it. That separation is what lets §2.6.1 evaluate a rule over the diff
// without cr forming an opinion, and what lets §2.6.3 count a rule's hits
// across rounds and report a dead one.
//
// A rule file is project-owned and hand-edited, so it is untrusted input, and
// the defence is the same one §2.5 gets: the struct below carries exactly the
// twelve rows of §2.6's table, and every key outside that table is refused by
// name rather than ignored. A silently dropped key leaves its author believing
// it took effect, which for a rule means believing a standard is being enforced
// when nothing is enforcing it.
//
// A malformed rule file aborts the command with exit code 3, per §2.6.5. The
// abort is a *MalformedError, or an *axis.InvalidError when `axis` is not one
// of the four ids of §1.5; the cli layer maps both onto the code.
//
// This package defends the file format and nothing else. §2.6 item 1's layer
// resolution, §2.6.1's detection, §2.6.2's fixes, and §2.6.3's harvesting are
// separate obligations, written elsewhere against this schema.
package rule

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"slices"
	"strings"

	"github.com/deligoez/cr/internal/axis"
	"github.com/deligoez/cr/internal/finding"
)

// Defaults for the three optional scalar rows of §2.6's table. They are named
// constants rather than literals at the assignment, because §2.6 states each
// one as part of the field's definition and a reader checking the table against
// the code should find the same three words.
const (
	// DefaultAxis is the axis a rule without one enforces.
	DefaultAxis = axis.Convention
	// DefaultSeverity is the severity records from a rule without one carry.
	DefaultSeverity = finding.SeverityMedium
	// DefaultKind is the register records from a rule without one are
	// written in. §2.6's table defaults it to `question`, which is P2 in the
	// schema: a mechanical match is not yet a defect, and §2.6.1.5 makes the
	// point again by having a hit reach a draft only once the agent confirms
	// it.
	DefaultKind = finding.KindQuestion
)

// idPattern is the kebab-case §2.6 requires of a rule id: lowercase
// alphanumerics in hyphen-separated groups. Because §2.6 also fixes the id as
// the file stem, this constrains the filename as much as the field. The id is
// what §2.6 item 3 stamps onto every record a rule produces and what
// §2.6.1.6's statistics accumulate under, so it must be one shape.
var idPattern = regexp.MustCompile(`^[a-z0-9]+(-[a-z0-9]+)*$`)

// The key sets of §2.6's table and of the two objects it points at. Each is
// both the allowlist and the message a rejected key is answered with, so the
// two cannot drift apart.
var (
	fields = []string{
		"id", "title", "rationale", "axis", "class", "severity", "kind",
		"detect", "fix", "globs", "exempt", "profiles",
	}
	detectFields = []string{"pattern", "mode"}
	fixFields    = []string{"replace", "with"}
)

// The closed value sets a rule shares with the record it produces. They are
// built out of finding's own constants rather than out of literals, so a
// rule's severity and a record's severity cannot come to mean different
// strings; what a list here can still get wrong is membership, and that is
// what the tests hold.
var (
	severities = []finding.Severity{
		finding.SeverityCritical, finding.SeverityHigh,
		finding.SeverityMedium, finding.SeverityLow,
	}
	kinds = []finding.Kind{finding.KindFinding, finding.KindQuestion}
)

// Detect is §2.6.1's optional mechanical detector.
//
// It is a pointer on Rule rather than a value, because its absence is
// load-bearing: §2.6.1.4 injects a rule *without* a detect block into its axis
// role's prompt as text, so an empty block and no block are two different
// rules. §2.6.1.2's compilation of Pattern and its closing of Mode at `regex`
// are that section's obligations, not this one's.
type Detect struct {
	// Pattern is the regular expression, in Go regexp syntax.
	Pattern string `json:"pattern"`
	// Mode is how the pattern is applied; §2.6.1.2 admits only `regex` in
	// v0.2.
	Mode string `json:"mode"`
}

// Fix is §2.6.2's optional suggestion template, applied to a matched line to
// produce suggestion text. It is a pointer on Rule for the reason Detect is:
// §2.6.2.1 makes the pair optional, and a rule that carries no fix suggests
// nothing rather than suggesting an empty replacement.
//
// §2.6.2.3 is the invariant behind it: cr never applies a fix to any file. A
// Fix only ever produces text for the author to accept.
type Fix struct {
	// Replace is the regular expression matched against the hit's line.
	Replace string `json:"replace"`
	// With is the replacement that produces the suggestion.
	With string `json:"with"`
}

// Rule is one §2.6 rule: every row of the table, with each absent list
// normalised to an empty one so a rule never serialises a slice as null.
//
// The set of fields is the contract. §2.6 gives a rule a standard (`title`,
// `rationale`), a lens (`axis`), the shape of the records it produces
// (`class`, `severity`, `kind`), an optional mechanism (`detect`, `fix`), and a
// scope (`globs`, `exempt`, `profiles`). It gives a rule no say in what cr
// emits, and no way to grade or state a record it produced: §6.2 computes the
// grade from the evidence and §2.6.1.5 keeps a hit short of a verdict.
type Rule struct {
	// ID is the rule id, equal to the file stem, kebab-case.
	ID string `json:"id"`
	// Title is the one-line statement of the standard.
	Title string `json:"title"`
	// Rationale is why the standard exists, quotable to the author.
	// §2.6 item 4 has a record produced by this rule able to quote it, so
	// the author learns the standard and not only the violation.
	Rationale string `json:"rationale"`
	// Axis is the axis that enforces the rule, one of the four ids of §1.5,
	// defaulting to DefaultAxis.
	Axis string `json:"axis"`
	// Class is the defect class assigned to records from this rule.
	Class string `json:"class"`
	// Severity is the default severity for those records, defaulting to
	// DefaultSeverity.
	Severity finding.Severity `json:"severity"`
	// Kind is the default register for those records, defaulting to
	// DefaultKind.
	Kind finding.Kind `json:"kind"`
	// Detect is the optional mechanical detector of §2.6.1, nil when the
	// rule carries no detect block.
	Detect *Detect `json:"detect,omitempty"`
	// Fix is the optional suggestion template of §2.6.2, nil when the rule
	// carries no fix block.
	Fix *Fix `json:"fix,omitempty"`
	// Globs names the paths the rule applies to. Empty means all source.
	Globs []string `json:"globs"`
	// Exempt names the paths excluded, for legacy areas.
	Exempt []string `json:"exempt"`
	// Profiles names the profiles the rule applies to. Empty means all.
	Profiles []string `json:"profiles"`
}

// MalformedError reports a rule file cr cannot use. It carries the file so the
// user can open it and the field so they know what to fix, which is what
// §2.6.5's abort is for. The cli layer maps it onto exit code 3.
type MalformedError struct {
	// File is the rule file.
	File string
	// Field is the offending field, empty when the fault is the file as a
	// whole and no single field can be blamed for it.
	Field string
	// Problem completes the sentence naming the field.
	Problem string
}

func (e *MalformedError) Error() string {
	if e.Field == "" {
		return fmt.Sprintf("%s: %s", e.File, e.Problem)
	}
	return fmt.Sprintf("%s: %s %s", e.File, e.Field, e.Problem)
}

// Load reads and validates one rule file. path also fixes the expected id,
// which §2.6 requires to equal the file stem.
func Load(path string) (Rule, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return Rule{}, &MalformedError{File: path, Problem: fmt.Sprintf("cannot be read: %v", err)}
	}
	return Parse(path, data)
}

// A rule reaches cr from one of two places, and they differ in exactly one
// requirement. §2.6 fixes a rule id to its file stem, which a rule file has and
// an element of a profile's `rules` array (§2.4) — the third layer of §2.6 item
// 1 — does not. Everything else §2.6's table asks of a rule it asks of both, so
// the two share one parse and part on this constant alone. A profile is data cr
// reads like any other, not a place where the schema is relaxed.
const (
	// fromFile parses a rule that has a file of its own.
	fromFile = true
	// fromProfile parses a rule embedded in a profile.
	fromProfile = false
)

// Parse validates data as the rule file at path. The path is only read for the
// file stem and for the error message; nothing on disk is touched.
func Parse(path string, data []byte) (Rule, error) {
	return parse(path, data, fromFile)
}

// parse is the body a rule file and an embedded rule share. stemmed says which
// of the two this is; path names whatever a fault is reported against, which is
// the rule file in the first case and the profile file in the second.
//
// The key check runs before the field checks, for the reason §2.5's does: a
// file carrying both an unknown key and a missing required field is most often
// one file, where the author wrote the field under a name cr does not know, and
// naming the key they used is the answer that leads them to it.
//
// Defaults are applied before validation rather than after, so what is
// validated is the value the rule will actually carry. A rule that omits
// `severity` is a rule at `medium`, and validating the empty string first would
// reject the ordinary case §2.6's table exists to permit.
func parse(path string, data []byte, stemmed bool) (Rule, error) {
	var keys map[string]json.RawMessage
	if err := json.Unmarshal(data, &keys); err != nil {
		return Rule{}, decodeError(path, err)
	}
	if err := checkKeys(path, "", keys, fields); err != nil {
		return Rule{}, err
	}
	var r Rule
	if err := json.Unmarshal(data, &r); err != nil {
		return Rule{}, decodeError(path, err)
	}
	if err := checkBlocks(path, keys); err != nil {
		return Rule{}, err
	}
	r.applyDefaults()
	if err := r.validate(path, stemmed); err != nil {
		return Rule{}, err
	}
	if err := compiles(path, &r); err != nil {
		return Rule{}, err
	}
	r.Globs, r.Exempt, r.Profiles = list(r.Globs), list(r.Exempt), list(r.Profiles)
	return r, nil
}

// checkKeys refuses any key outside the set §2.6 fixes for one object. scope is
// the object's field name — empty for the rule file itself — and both prefixes
// the rejected key and names the table in the message.
//
// This is where "a rule is data" stops being a sentence and becomes a fence. A
// rule declaring `grade`, `state`, or a `prompt` is rejected by name rather
// than decoded into nothing, so no rule file can quietly claim a field §6.1.4
// reserves for cr or turn itself back into the prompt text §2.6 separated it
// from.
//
// Keys are visited in sorted order so a file carrying two unknown keys reports
// the same one on every run.
func checkKeys(path, scope string, keys map[string]json.RawMessage, allowed []string) error {
	prefix, label := "", "rule"
	if scope != "" {
		prefix, label = scope+".", scope
	}
	unknown := make([]string, 0, len(keys))
	for key := range keys {
		if !slices.Contains(allowed, key) {
			unknown = append(unknown, prefix+key)
		}
	}
	if len(unknown) == 0 {
		return nil
	}
	slices.Sort(unknown)
	return &MalformedError{
		File:  path,
		Field: unknown[0],
		Problem: fmt.Sprintf(
			"is not a %s field; §2.6 has exactly %s%s",
			label, prefix, strings.Join(allowed, ", "+prefix),
		),
	}
}

// checkBlocks holds the two objects of §2.6's table to their own key sets.
// §2.6.1 names `detect.pattern` and `detect.mode` and §2.6.2 names
// `fix.replace` and `fix.with`; both sets are closed, and a key outside one is
// as unusable as a key outside the table itself.
//
// A block that is absent or null is skipped rather than rejected: §2.6 marks
// both optional, and §2.6.1.4 gives a rule without a detect block its own
// behaviour.
func checkBlocks(path string, keys map[string]json.RawMessage) error {
	for _, block := range []struct {
		scope   string
		allowed []string
	}{
		{scope: "detect", allowed: detectFields},
		{scope: "fix", allowed: fixFields},
	} {
		raw, present := keys[block.scope]
		if !present {
			continue
		}
		var inner map[string]json.RawMessage
		if err := json.Unmarshal(raw, &inner); err != nil {
			return decodeError(path, err)
		}
		if err := checkKeys(path, block.scope, inner, block.allowed); err != nil {
			return err
		}
	}
	return nil
}

// decodeError turns a decoding failure into a MalformedError, keeping the field
// the decoder identified so a type error names the field like every other
// fault. A `globs` that is a string rather than an array arrives here.
func decodeError(path string, err error) error {
	var typeErr *json.UnmarshalTypeError
	if errors.As(err, &typeErr) && typeErr.Field != "" {
		return &MalformedError{
			File:    path,
			Field:   typeErr.Field,
			Problem: fmt.Sprintf("must be %s, but the file has %s", typeErr.Type, typeErr.Value),
		}
	}
	return &MalformedError{File: path, Problem: fmt.Sprintf("is not valid JSON: %v", err)}
}

// applyDefaults fills the three optional scalar rows §2.6's table gives a
// default. Absence and blankness are treated alike: a `severity` of "" is not a
// severity, and the table's default is the honest reading of both.
func (r *Rule) applyDefaults() {
	if r.Axis == "" {
		r.Axis = DefaultAxis
	}
	if r.Severity == "" {
		r.Severity = DefaultSeverity
	}
	if r.Kind == "" {
		r.Kind = DefaultKind
	}
}

// validate applies every requirement of §2.6's table, in table order, so the
// first fault a user sees is the earliest one in the file's own layout.
func (r *Rule) validate(path string, stemmed bool) error {
	if err := r.validateID(path, stemmed); err != nil {
		return err
	}
	if err := r.validateText(path); err != nil {
		return err
	}
	return r.validateEnums(path)
}

// validateID holds `id` to §2.6's two rules at once: it equals the file stem,
// and it is kebab-case. The pair is what makes a rules directory listable as a
// list of rule ids.
//
// The stem half applies only to a rule that has a file — see fromFile. An
// element of a profile's `rules` array has none, and comparing its id against a
// stem derived from its own id would report a check that cannot fail as one
// that passed. The kebab-case half applies to both, because §2.6 item 3 stamps
// the id onto every record either kind of rule produces.
func (r *Rule) validateID(path string, stemmed bool) error {
	stem := strings.TrimSuffix(filepath.Base(path), filepath.Ext(path))
	switch {
	case r.ID == "":
		return &MalformedError{File: path, Field: "id", Problem: "is required"}
	case stemmed && r.ID != stem:
		return &MalformedError{
			File:    path,
			Field:   "id",
			Problem: fmt.Sprintf("is %q, but §2.6 requires it to equal the file stem %q", r.ID, stem),
		}
	case !idPattern.MatchString(r.ID):
		// The remedy is only named where it exists. Telling the author of
		// an embedded rule to rename a file sends them looking for one
		// that was never written.
		problem := fmt.Sprintf("is %q, which is not kebab-case; §2.6 requires %s", r.ID, idPattern)
		if stemmed {
			problem += ", and the file must be renamed with it"
		}
		return &MalformedError{File: path, Field: "id", Problem: problem}
	}
	return nil
}

// validateText holds the two required prose rows and `class`.
//
// A required text field is checked for blankness rather than absence. A title
// of " " states no standard and a rationale of " " is quotable to nobody, so
// the two cases have the same fix and get the same message.
//
// `rationale` is required for a reason worth keeping in view: §2.6 item 4 has
// every record a rule produces able to quote it, so a rule without one can
// still name a violation and can no longer teach the standard. That is the
// difference between a comment that spends trust and one that builds it.
func (r *Rule) validateText(path string) error {
	if strings.TrimSpace(r.Title) == "" {
		return &MalformedError{File: path, Field: "title", Problem: "is required"}
	}
	if strings.TrimSpace(r.Rationale) == "" {
		return &MalformedError{
			File:  path,
			Field: "rationale",
			Problem: "is required; §2.6 item 4 has every record from this rule able to quote it, " +
				"so the author learns the standard and not only the violation",
		}
	}
	if r.Class == "" {
		return &MalformedError{File: path, Field: "class", Problem: "is required"}
	}
	// §6.1 fixes the form of a defect class and finding.ValidateClass is the
	// one place that judges it. The error it builds is shaped for a record —
	// a file and a line inside an NDJSON stream — so a rule file borrows the
	// judgement and discards the sentence, reporting the fault in its own
	// words. The line handed over is never read back.
	if finding.ValidateClass(path, 0, r.Class) != nil {
		return &MalformedError{
			File:  path,
			Field: "class",
			Problem: fmt.Sprintf(
				"is %q, which is not kebab-case; §6.1 fixes the form at [a-z0-9-]+, "+
					"and every record from this rule carries it",
				r.Class,
			),
		}
	}
	return nil
}

// validateEnums holds the three closed value sets a rule shares with the
// records it produces: §1.5's axes, and §6.1's severities and kinds. Each has
// already been defaulted, so what reaches here is the value the rule carries.
func (r *Rule) validateEnums(path string) error {
	// §1.5 closes the axis id set, and axis.Validate is the one place that
	// judges it.
	if err := axis.Validate(path, "axis", r.Axis); err != nil {
		return err
	}
	if !slices.Contains(severities, r.Severity) {
		return &MalformedError{
			File:  path,
			Field: "severity",
			Problem: fmt.Sprintf(
				"is %q, which is not a severity; §6.1 has exactly %s",
				r.Severity, names(severities),
			),
		}
	}
	if !slices.Contains(kinds, r.Kind) {
		return &MalformedError{
			File:  path,
			Field: "kind",
			Problem: fmt.Sprintf(
				"is %q, which is not a kind; §6.1 has exactly %s",
				r.Kind, names(kinds),
			),
		}
	}
	return nil
}

// names joins a closed value set for a message, so the sentence a user reads
// is built from the same list the check reads.
func names[T ~string](values []T) string {
	written := make([]string, 0, len(values))
	for _, value := range values {
		written = append(written, string(value))
	}
	return strings.Join(written, ", ")
}

// list copies a decoded list and turns an absent one into an empty one, so no
// rule field ever serialises as null.
func list(values []string) []string {
	return append(make([]string, 0, len(values)), values...)
}
