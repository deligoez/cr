package rule

import (
	"encoding/json"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/deligoez/cr/internal/axis"
	"github.com/deligoez/cr/internal/finding"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// specFields is the §2.6 field table, in its own order. It is written out here
// rather than taken from the package, because a guard that read the value it
// judges would pass whatever the package happened to say.
var specFields = []string{
	"id", "title", "rationale", "axis", "class", "severity", "kind",
	"detect", "fix", "globs", "exempt", "profiles",
}

// write puts content at a rule path with the given stem and returns it.
func write(t *testing.T, stem, content string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), stem+".json")
	require.NoError(t, os.WriteFile(path, []byte(content), 0o600))
	return path
}

// ruleFile writes a rule file at the given stem satisfying every required field
// of §2.6, with overrides applied first: a value replaces that field, and nil
// removes it. Building the document rather than splicing strings is what lets a
// case say "title is absent" and "title is blank" as two different files.
func ruleFile(t *testing.T, stem string, overrides map[string]any) string {
	t.Helper()
	doc := map[string]any{
		"id":        stem,
		"title":     "Every error is handled where it is returned.",
		"rationale": "A dropped error turns a failure into a wrong answer nobody sees.",
		"class":     "unchecked-error",
	}
	for key, value := range overrides {
		if value == nil {
			delete(doc, key)
			continue
		}
		doc[key] = value
	}
	data, err := json.Marshal(doc)
	require.NoError(t, err)
	return write(t, stem, string(data))
}

// §2.6's table is the whole rule surface, and the sentence above it — a rule is
// one normative statement, data, kept separate from roles — holds only if the
// struct carries exactly the table's rows. A missing row drops something the
// spec promised; an extra one is the crack a prompt, a grade, or a §6.1.4 stamp
// field would come through. Walking the struct reflectively catches either,
// including a field added years from now.
//
// The allowlist is held to the same literal, because it is the runtime half of
// the same claim. The struct alone only makes an unknown key inert; the
// allowlist is what makes it audible, and a key it forgot would be decoded into
// nothing and reported as nothing.
func TestARuleCarriesExactlyTheSpecFields(t *testing.T) {
	names := make([]string, 0, reflect.TypeFor[Rule]().NumField())
	for field := range reflect.TypeFor[Rule]().Fields() {
		name, _, _ := strings.Cut(field.Tag.Get("json"), ",")
		names = append(names, name)
	}

	assert.Equal(t, specFields, names)
	assert.Equal(t, specFields, fields,
		"§2.6's table is the allowlist; a key missing from it is a key cr would silently ignore")
}

// §2.6's table marks `axis`, `severity`, and `kind` optional and gives each a
// default. A rule that states only its standard is the ordinary case, so the
// three defaults are what most rule files actually run on, and getting one
// wrong changes what every record from that rule says without any file saying
// so.
//
// `kind` defaulting to `question` is P2 written into the schema: a rule is a
// standard, and a match against it is not yet a defect. §2.6.1.5 says the same
// thing from the other side — a hit reaches a draft only once the agent
// confirms it.
func TestTheOptionalScalarsTakeTheDefaultsTheTableGives(t *testing.T) {
	r, err := Load(ruleFile(t, "handle-every-error", nil))
	require.NoError(t, err)

	assert.Equal(t, axis.Convention, r.Axis, "§2.6's table defaults `axis` to convention")
	assert.Equal(t, finding.SeverityMedium, r.Severity, "§2.6's table defaults `severity` to medium")
	assert.Equal(t, finding.KindQuestion, r.Kind, "§2.6's table defaults `kind` to question")

	assert.Equal(t, DefaultAxis, r.Axis)
	assert.Equal(t, DefaultSeverity, r.Severity)
	assert.Equal(t, DefaultKind, r.Kind)
}

// A stated value must survive, or the defaults above would be the only values a
// rule ever has. The pair matters more than either half: a defaulting bug that
// overwrote what the file said would still pass the test above.
func TestAStatedScalarIsCarriedRatherThanDefaulted(t *testing.T) {
	r, err := Load(ruleFile(t, "handle-every-error", map[string]any{
		"axis":     axis.Correctness,
		"severity": string(finding.SeverityHigh),
		"kind":     string(finding.KindFinding),
	}))
	require.NoError(t, err)

	assert.Equal(t, axis.Correctness, r.Axis)
	assert.Equal(t, finding.SeverityHigh, r.Severity)
	assert.Equal(t, finding.KindFinding, r.Kind)
}

// §2.6 fixes the id twice over: kebab-case, and equal to the file stem. The
// second is what makes it unspoofable — the stem already names the rule, so a
// file stating a different id is a contradiction with no correct resolution,
// and cr refuses it rather than preferring one half over the other. §2.6 item 3
// then stamps that id onto every record the rule produces and §2.6.1.6
// accumulates statistics under it, so one rule must be one string.
func TestTheIDMustBeTheKebabCaseFileStem(t *testing.T) {
	t.Run("a well-formed file takes its id from the stem", func(t *testing.T) {
		r, err := Load(ruleFile(t, "handle-every-error", nil))
		require.NoError(t, err)
		assert.Equal(t, "handle-every-error", r.ID)
	})

	for _, c := range []struct {
		name, stem string
		id         any
		says       string
	}{
		{"absent", "handle-every-error", nil, "is required"},
		{"naming another rule", "handle-every-error", "no-bare-except", `"no-bare-except"`},
		{"capitalised", "Handle-Every-Error", "Handle-Every-Error", "kebab-case"},
		{"hyphen-terminated", "handle-every-error-", "handle-every-error-", "kebab-case"},
		{"underscored", "handle_every_error", "handle_every_error", "kebab-case"},
	} {
		t.Run(c.name, func(t *testing.T) {
			path := ruleFile(t, c.stem, map[string]any{"id": c.id})

			_, err := Load(path)

			var malformed *MalformedError
			require.ErrorAs(t, err, &malformed)
			assert.Equal(t, "id", malformed.Field)
			assert.Contains(t, err.Error(), path)
			assert.Contains(t, err.Error(), c.says)
		})
	}
}

// §2.6's table marks `title`, `rationale`, and `class` required, and §2.6.5
// aborts on a file that omits one. Blankness is refused with absence: a title
// of " " states no standard and a rationale of " " is quotable to nobody, so
// both have the same fix.
//
// `rationale` carries the most weight of the three. §2.6 item 4 has every
// record from a rule able to quote it, so a rule without one can name a
// violation and can no longer teach the standard — which in the trust economy
// is the difference between a comment that builds standing and one that spends
// it.
func TestEveryRequiredRowIsRequiredAndMayNotBeBlank(t *testing.T) {
	for _, field := range []string{"title", "rationale", "class"} {
		for _, c := range []struct {
			name  string
			value any
		}{
			{"absent", nil},
			{"blank", ""},
		} {
			t.Run(field+" "+c.name, func(t *testing.T) {
				path := ruleFile(t, "handle-every-error", map[string]any{field: c.value})

				_, err := Load(path)

				var malformed *MalformedError
				require.ErrorAs(t, err, &malformed)
				assert.Equal(t, field, malformed.Field)
				assert.Contains(t, err.Error(), path)
			})
		}
	}

	t.Run("whitespace is blank for the prose rows", func(t *testing.T) {
		for _, field := range []string{"title", "rationale"} {
			path := ruleFile(t, "handle-every-error", map[string]any{field: "   "})

			_, err := Load(path)

			var malformed *MalformedError
			require.ErrorAs(t, err, &malformed)
			assert.Equal(t, field, malformed.Field)
		}
	})

	t.Run("the rationale refusal says why it is required", func(t *testing.T) {
		_, err := Load(ruleFile(t, "handle-every-error", map[string]any{"rationale": nil}))

		assert.Contains(t, err.Error(), "quote it")
	})
}

// §2.6's table assigns a rule's `class` to every record from that rule, and
// §6.1 fixes the form of a defect class at [a-z0-9-]+. A rule carrying anything
// else would mint records cr's own record validation refuses, so the refusal
// happens at the rule file, where the user can fix it.
//
// The two spellings matter for more than tidiness: §6.4.1 deduplicates on the
// class, §7.3 counts triage outcomes by it, and §7.4 waives by it, so two
// spellings of one class are two classes everywhere downstream.
func TestTheClassMustBeTheFormSection61Fixes(t *testing.T) {
	for _, class := range []string{"Unchecked-Error", "unchecked error", "unchecked_error", "unchecked.error"} {
		t.Run(class, func(t *testing.T) {
			path := ruleFile(t, "handle-every-error", map[string]any{"class": class})

			_, err := Load(path)

			var malformed *MalformedError
			require.ErrorAs(t, err, &malformed)
			assert.Equal(t, "class", malformed.Field)
			assert.Contains(t, err.Error(), "[a-z0-9-]+")
		})
	}
}

// The three closed sets a rule shares with the records it produces. §1.5 closes
// the axes, and §6.1 closes the severities at four and the kinds at two. A rule
// naming a fifth severity or a third kind would produce records outside §6.1's
// schema, and the refusal belongs on the rule file rather than on every record
// it later mints.
//
// Every member is accepted as well as every non-member refused. A check written
// against a list with one value missing refuses a legal rule, which is the
// failure a rejection-only test cannot see.
func TestTheAxisSeverityAndKindSetsAreClosed(t *testing.T) {
	t.Run("every axis of §1.5 is accepted", func(t *testing.T) {
		for _, id := range axis.IDs() {
			r, err := Load(ruleFile(t, "handle-every-error", map[string]any{"axis": id}))
			require.NoError(t, err)
			assert.Equal(t, id, r.Axis)
		}
	})

	t.Run("an axis outside §1.5 aborts as an axis error", func(t *testing.T) {
		path := ruleFile(t, "handle-every-error", map[string]any{"axis": "security"})

		_, err := Load(path)

		var invalid *axis.InvalidError
		require.ErrorAs(t, err, &invalid)
		assert.Equal(t, "axis", invalid.Field)
		assert.Equal(t, path, invalid.File)
	})

	t.Run("every severity of §6.1 is accepted", func(t *testing.T) {
		for _, severity := range []finding.Severity{
			finding.SeverityCritical, finding.SeverityHigh,
			finding.SeverityMedium, finding.SeverityLow,
		} {
			r, err := Load(ruleFile(t, "handle-every-error", map[string]any{"severity": string(severity)}))
			require.NoError(t, err)
			assert.Equal(t, severity, r.Severity)
		}
	})

	t.Run("every kind of §6.1 is accepted", func(t *testing.T) {
		for _, kind := range []finding.Kind{finding.KindFinding, finding.KindQuestion} {
			r, err := Load(ruleFile(t, "handle-every-error", map[string]any{"kind": string(kind)}))
			require.NoError(t, err)
			assert.Equal(t, kind, r.Kind)
		}
	})

	for _, c := range []struct{ field, value, says string }{
		{"severity", "blocker", "critical, high, medium, low"},
		{"severity", "Medium", "critical, high, medium, low"},
		{"kind", "suggestion", "finding, question"},
		{"kind", "Question", "finding, question"},
	} {
		t.Run(c.field+" "+c.value, func(t *testing.T) {
			path := ruleFile(t, "handle-every-error", map[string]any{c.field: c.value})

			_, err := Load(path)

			var malformed *MalformedError
			require.ErrorAs(t, err, &malformed)
			assert.Equal(t, c.field, malformed.Field)
			assert.Contains(t, err.Error(), c.says)
		})
	}
}

// §2.6 marks `detect` and `fix` optional, and §2.6.1.4 gives a rule *without* a
// detect block a different behaviour from one with an empty one: it is injected
// into its axis role's prompt as text instead of being evaluated. So absence
// has to be distinguishable from emptiness, which is why both are pointers and
// why this test asserts nil rather than a zero value.
func TestTheOptionalBlocksAreAbsentRatherThanEmptyWhenTheFileOmitsThem(t *testing.T) {
	t.Run("omitted", func(t *testing.T) {
		r, err := Load(ruleFile(t, "handle-every-error", nil))
		require.NoError(t, err)

		assert.Nil(t, r.Detect, "§2.6.1.4 reads a missing detect block, not an empty one")
		assert.Nil(t, r.Fix, "§2.6.2.1 makes the fix optional")
	})

	t.Run("present", func(t *testing.T) {
		r, err := Load(ruleFile(t, "handle-every-error", map[string]any{
			"detect": map[string]any{"pattern": `_ = err`, "mode": "regex"},
			"fix":    map[string]any{"replace": `_ = err`, "with": "if err != nil {"},
		}))
		require.NoError(t, err)

		require.NotNil(t, r.Detect)
		assert.Equal(t, `_ = err`, r.Detect.Pattern)
		assert.Equal(t, "regex", r.Detect.Mode)
		require.NotNil(t, r.Fix)
		assert.Equal(t, `_ = err`, r.Fix.Replace)
		assert.Equal(t, "if err != nil {", r.Fix.With)
	})

	t.Run("empty blocks are carried as empty, not as absent", func(t *testing.T) {
		r, err := Load(ruleFile(t, "handle-every-error", map[string]any{
			"fix": map[string]any{},
		}))
		require.NoError(t, err)
		assert.NotNil(t, r.Fix)

		_, err = Load(ruleFile(t, "handle-every-error", map[string]any{
			"detect": map[string]any{},
		}))
		var malformed *MalformedError
		require.ErrorAs(t, err, &malformed,
			"an empty block is a detect block; §2.6.1.2 is what refuses its contents")
		assert.Equal(t, "detect.mode", malformed.Field)
	})
}

// cr's output contract writes an empty collection as [], never null. A rule
// omitting `globs` applies to all source, and a `globs: null` in a payload
// reads as a different claim from `globs: []` to anything downstream that
// checks the key rather than its length.
func TestAnAbsentListIsEmptyRatherThanNull(t *testing.T) {
	r, err := Load(ruleFile(t, "handle-every-error", nil))
	require.NoError(t, err)

	assert.NotNil(t, r.Globs)
	assert.NotNil(t, r.Exempt)
	assert.NotNil(t, r.Profiles)
	assert.Empty(t, r.Globs)

	encoded, err := json.Marshal(r)
	require.NoError(t, err)
	assert.Contains(t, string(encoded), `"globs":[]`)
	assert.Contains(t, string(encoded), `"exempt":[]`)
	assert.Contains(t, string(encoded), `"profiles":[]`)
	assert.NotContains(t, string(encoded), "null")
}

// The scope rows are carried as written. §2.6's table gives `globs`, `exempt`,
// and `profiles` their meaning — the paths a rule applies to, the legacy areas
// excluded, and the profiles it belongs to — and §2.6.1's detection is written
// against them.
func TestTheScopeRowsAreCarriedAsWritten(t *testing.T) {
	r, err := Load(ruleFile(t, "handle-every-error", map[string]any{
		"globs":    []string{"internal/**/*.go"},
		"exempt":   []string{"internal/legacy/**"},
		"profiles": []string{"generic"},
	}))
	require.NoError(t, err)

	assert.Equal(t, []string{"internal/**/*.go"}, r.Globs)
	assert.Equal(t, []string{"internal/legacy/**"}, r.Exempt)
	assert.Equal(t, []string{"generic"}, r.Profiles)
}

// §2.6.5 aborts on a malformed rule file without qualifying what made it
// malformed, so a file that is not JSON at all, a file cr cannot open, and a
// field of the wrong type all have to fail the same way. A type error keeps the
// field the decoder identified, so the user is told which row to look at rather
// than being handed a decoder's sentence about an offset.
func TestAFileCrCannotReadAsWrittenIsMalformed(t *testing.T) {
	t.Run("not JSON", func(t *testing.T) {
		path := write(t, "handle-every-error", "id: handle-every-error\n")

		_, err := Load(path)

		var malformed *MalformedError
		require.ErrorAs(t, err, &malformed)
		assert.Empty(t, malformed.Field, "no single row can be blamed for a file that does not parse")
		assert.Contains(t, err.Error(), path)
	})

	t.Run("missing", func(t *testing.T) {
		path := filepath.Join(t.TempDir(), "handle-every-error.json")

		_, err := Load(path)

		var malformed *MalformedError
		require.ErrorAs(t, err, &malformed)
		assert.Contains(t, err.Error(), "cannot be read")
	})

	t.Run("a row of the wrong type names the row", func(t *testing.T) {
		path := ruleFile(t, "handle-every-error", map[string]any{"globs": "internal/**/*.go"})

		_, err := Load(path)

		var malformed *MalformedError
		require.ErrorAs(t, err, &malformed)
		assert.Equal(t, "globs", malformed.Field)
	})

	t.Run("a block of the wrong type names the block", func(t *testing.T) {
		path := ruleFile(t, "handle-every-error", map[string]any{"detect": "_ = err"})

		_, err := Load(path)

		var malformed *MalformedError
		require.ErrorAs(t, err, &malformed)
		assert.Equal(t, "detect", malformed.Field)
	})
}
