package rule

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/deligoez/cr/internal/finding"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// profileFile is the path Resolve is given for the profile carrying the third
// layer. §2.4 ties a profile id to its file stem, so the embedded rules are
// parsed under the name the profile is actually written as.
const profileFile = "profiles/laravel-pest.json"

// ruleDoc builds a rule satisfying every required field of §2.6 under the given
// id, with overrides applied first: a value replaces that field, and nil
// removes it.
func ruleDoc(id string, overrides map[string]any) map[string]any {
	doc := map[string]any{
		"id":        id,
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
	return doc
}

// rulesDir writes one rules directory holding every document, each under the
// file stem §2.6 requires to equal its id, and returns the directory.
func rulesDir(t *testing.T, docs ...map[string]any) string {
	t.Helper()
	dir := t.TempDir()
	for _, doc := range docs {
		data, err := json.Marshal(doc)
		require.NoError(t, err)
		id, ok := doc["id"].(string)
		require.True(t, ok, "a rule document needs a string id to be written under")
		require.NoError(t, os.WriteFile(filepath.Join(dir, id+fileExt), data, 0o600))
	}
	return dir
}

// embedded turns rule documents into the `rules` array a profile carries them
// in, which §2.4 hands over verbatim.
func embedded(t *testing.T, docs ...map[string]any) []json.RawMessage {
	t.Helper()
	raw := make([]json.RawMessage, 0, len(docs))
	for _, doc := range docs {
		data, err := json.Marshal(doc)
		require.NoError(t, err)
		raw = append(raw, data)
	}
	return raw
}

// ids reads the resolved corpus as a list of rule ids.
func ids(corpus []Resolved) []string {
	written := make([]string, 0, len(corpus))
	for at := range corpus {
		written = append(written, corpus[at].Rule.ID)
	}
	return written
}

// sources reads the resolved corpus as the list of layers it resolved from.
func sources(corpus []Resolved) []Source {
	from := make([]Source, 0, len(corpus))
	for at := range corpus {
		from = append(from, corpus[at].Source)
	}
	return from
}

// §2.6 item 1 fixes the resolution order: per-repository, then global, then the
// resolved profile's `rules` array. All three layers are populated here with
// ids that do not collide, so what is asserted is the sequence itself rather
// than the shadowing that item 2 covers.
func TestRulesResolveInLayersHighestFirst(t *testing.T) {
	corpus, err := Resolve(
		rulesDir(t, ruleDoc("from-repo", nil)),
		rulesDir(t, ruleDoc("from-global", nil)),
		profileFile,
		embedded(t, ruleDoc("from-profile", nil)),
	)
	require.NoError(t, err)

	assert.Equal(t, []string{"from-repo", "from-global", "from-profile"}, ids(corpus))
	assert.Equal(t, []Source{RepoSource, GlobalSource, ProfileSource}, sources(corpus),
		"a caller cannot tell an override from the rule it replaced without the layer")
}

// The constants are declared in §2.6 item 1's order, and code that reads them
// as an order — the corpus above, and anything sorting by it later — is reading
// that declaration. A reordering would change the corpus while every test
// naming the constants still passed.
func TestTheSourceConstantsRunHighestPrecedenceFirst(t *testing.T) {
	assert.Less(t, RepoSource, GlobalSource, "§2.6 item 1 puts per-repository above global")
	assert.Less(t, GlobalSource, ProfileSource, "§2.6 item 1 puts global above the profile's array")
}

// §2.6 item 2: a lower layer carrying the same id is overridden whole, never
// merged field by field. This is the acceptance the section turns on, so the
// lower rule is given a value in every optional row §2.6's table has — a
// severity, a kind, an axis, both blocks, and all three lists — and the winning
// rule states none of them.
//
// The assertion is equality with the winning file parsed alone. A field-by-field
// list would defend the fields it happened to name and pass a merge that leaked
// any other, including a row added to §2.6 years from now.
func TestALowerLayersUntouchedFieldsDoNotSurviveIntoTheWinner(t *testing.T) {
	furnished := map[string]any{
		"axis":     "correctness",
		"severity": "high",
		"kind":     "finding",
		"detect":   map[string]any{"pattern": "DB::raw", "mode": "regex"},
		"fix":      map[string]any{"replace": "DB::raw", "with": "DB::query"},
		"globs":    []string{"app/**"},
		"exempt":   []string{"app/Legacy/**"},
		"profiles": []string{"laravel-pest"},
	}

	for _, c := range []struct {
		name         string
		repo, global []map[string]any
		profile      []map[string]any
		expected     Source
	}{
		{
			name:     "per-repository over global",
			repo:     []map[string]any{ruleDoc("no-raw-sql", nil)},
			global:   []map[string]any{ruleDoc("no-raw-sql", furnished)},
			expected: RepoSource,
		},
		{
			name:     "global over the profile's array",
			global:   []map[string]any{ruleDoc("no-raw-sql", nil)},
			profile:  []map[string]any{ruleDoc("no-raw-sql", furnished)},
			expected: GlobalSource,
		},
		{
			name:     "per-repository over both",
			repo:     []map[string]any{ruleDoc("no-raw-sql", nil)},
			global:   []map[string]any{ruleDoc("no-raw-sql", furnished)},
			profile:  []map[string]any{ruleDoc("no-raw-sql", furnished)},
			expected: RepoSource,
		},
	} {
		t.Run(c.name, func(t *testing.T) {
			corpus, err := Resolve(
				rulesDir(t, c.repo...), rulesDir(t, c.global...),
				profileFile, embedded(t, c.profile...),
			)
			require.NoError(t, err)

			require.Len(t, corpus, 1, "§2.6 item 2 makes a rule id unique after resolution")
			won := corpus[0]
			assert.Equal(t, c.expected, won.Source)

			// What the winning layer wrote, parsed with no other layer
			// in sight. Anything the loser contributed shows up here.
			alone, err := Load(filepath.Join(
				rulesDir(t, ruleDoc("no-raw-sql", nil)), "no-raw-sql"+fileExt))
			require.NoError(t, err)
			assert.Equal(t, alone, won.Rule,
				"the loser's fields reached the winner; §2.6 item 2 overrides whole")

			// Named individually as well, because the equality above
			// says a merge happened and these say what a merge would
			// have meant: a pattern the user never wrote, running
			// against paths they never named, under their rule's id.
			assert.Nil(t, won.Rule.Detect, "an inherited detect block runs a pattern nobody wrote")
			assert.Nil(t, won.Rule.Fix)
			assert.Empty(t, won.Rule.Globs)
			assert.Empty(t, won.Rule.Exempt)
			assert.Empty(t, won.Rule.Profiles)
			assert.Equal(t, DefaultAxis, won.Rule.Axis)
			assert.Equal(t, DefaultSeverity, won.Rule.Severity)
			assert.Equal(t, DefaultKind, won.Rule.Kind)
		})
	}
}

// A rule the winning layer omits is not shadowed by anything and stays in the
// corpus. Override is per id, not per layer: a per-repository directory holding
// one rule does not switch the other layers off.
func TestAShadowedLayerStillContributesItsOtherRules(t *testing.T) {
	corpus, err := Resolve(
		rulesDir(t, ruleDoc("no-raw-sql", nil)),
		rulesDir(t, ruleDoc("no-raw-sql", nil), ruleDoc("handle-every-error", nil)),
		profileFile,
		embedded(t, ruleDoc("no-raw-sql", nil), ruleDoc("test-every-branch", nil)),
	)
	require.NoError(t, err)

	assert.Equal(t, []string{"no-raw-sql", "handle-every-error", "test-every-branch"}, ids(corpus))
	assert.Equal(t, []Source{RepoSource, GlobalSource, ProfileSource}, sources(corpus))
}

// Neither on-disk layer has to exist. The per-repository directory appears only
// once a repository has been seen, and a run that has customised nothing has
// neither — which is a corpus of the profile's own rules, not a fault.
func TestAnAbsentRulesDirectoryHoldsNoRule(t *testing.T) {
	absent := filepath.Join(t.TempDir(), "never-created")

	corpus, err := Resolve(absent, absent, profileFile, nil)
	require.NoError(t, err)

	assert.Empty(t, corpus)
	assert.NotNil(t, corpus, "an empty corpus serialises as [], never null")
}

// A machine that customised its rules globally and nowhere else is the common
// case, not the edge: no per-repository directory, and a profile — or §2.4.4's
// empty one — carrying no `rules` array. The corpus is then the global layer
// alone, holding more rules than the other two layers together: a shape every
// fixture above leaves out.
func TestAGlobalLayerAloneIsTheWholeCorpus(t *testing.T) {
	absent := filepath.Join(t.TempDir(), "never-created")

	corpus, err := Resolve(absent,
		rulesDir(t, ruleDoc("no-raw-sql", nil), ruleDoc("handle-every-error", nil)),
		profileFile, nil)
	require.NoError(t, err)

	assert.Equal(t, []string{"handle-every-error", "no-raw-sql"}, ids(corpus))
	assert.Equal(t, []Source{GlobalSource, GlobalSource}, sources(corpus))
}

// An absent directory is the one listing failure that means "no rule". Every
// other one is reported, so a rules directory cr cannot read aborts rather than
// silently resolving to the layer below it — which would enforce the global
// standard while the user believes their own is in force.
//
// A regular file where a directory belongs is the portable way to produce one:
// os.ReadDir answers ENOTDIR, which is neither absence nor a rule.
func TestARulesDirectoryCrCannotListIsReportedRatherThanSkipped(t *testing.T) {
	notADir := filepath.Join(t.TempDir(), "rules")
	require.NoError(t, os.WriteFile(notADir, []byte("not a directory"), 0o600))

	_, err := Resolve(notADir, rulesDir(t, ruleDoc("no-raw-sql", nil)), profileFile, nil)

	var malformed *MalformedError
	require.ErrorAs(t, err, &malformed)
	assert.Equal(t, notADir, malformed.File)
	assert.Contains(t, malformed.Problem, "cannot be listed")
}

// §2.6.5 aborts on a malformed rule without qualifying which one, and a rule a
// higher layer is about to shadow is still one. Passing it over would hand the
// user the rule below while they believe their own override is enforcing their
// standard, and cr would never say so.
func TestAMalformedRuleAbortsEvenWhereAHigherLayerShadowsIt(t *testing.T) {
	repo := rulesDir(t, ruleDoc("no-raw-sql", nil))
	global := rulesDir(t, ruleDoc("no-raw-sql", map[string]any{"rationale": nil}))

	_, err := Resolve(repo, global, profileFile, nil)

	var malformed *MalformedError
	require.ErrorAs(t, err, &malformed)
	assert.Equal(t, "rationale", malformed.Field)
}

// §2.6 fixes a rule id to the file stem, and an element of a profile's `rules`
// array has no file. Everything else §2.6's table asks of a rule it asks of an
// embedded one, so the id is still required and still kebab-case, and a fault
// is still §2.6.5's abort — reported against the profile file and the array
// position, which is what the author has open.
func TestAProfileRuleIsHeldToEverySpecFieldExceptTheFileStem(t *testing.T) {
	t.Run("an id matching no file resolves", func(t *testing.T) {
		corpus, err := Resolve(t.TempDir(), t.TempDir(), profileFile,
			embedded(t, ruleDoc("prefer-value-objects", nil)))
		require.NoError(t, err)

		require.Len(t, corpus, 1)
		assert.Equal(t, "prefer-value-objects", corpus[0].Rule.ID)
		assert.Equal(t, ProfileSource, corpus[0].Source)
	})

	for _, c := range []struct {
		name, field string
		doc         map[string]any
	}{
		{
			name:  "the id is required",
			field: "rules[1].id",
			doc:   ruleDoc("prefer-value-objects", map[string]any{"id": nil}),
		},
		{
			name:  "the class is required",
			field: "rules[1].class",
			doc:   ruleDoc("prefer-value-objects", map[string]any{"class": nil}),
		},
		{
			name:  "a key §2.6's table does not have",
			field: "rules[1].instructions",
			doc:   ruleDoc("prefer-value-objects", map[string]any{"instructions": "Judge it."}),
		},
		{
			name:  "a key the detect block does not have",
			field: "rules[1].detect.exclude",
			doc: ruleDoc("prefer-value-objects", map[string]any{
				"detect": map[string]any{"exclude": "app/Legacy/**"},
			}),
		},
	} {
		t.Run(c.name, func(t *testing.T) {
			_, err := Resolve(t.TempDir(), t.TempDir(), profileFile,
				embedded(t, ruleDoc("handle-every-error", nil), c.doc))

			var malformed *MalformedError
			require.ErrorAs(t, err, &malformed)
			assert.Equal(t, c.field, malformed.Field,
				"the array position is what turns the message into a line to open")
			assert.Equal(t, profileFile, malformed.File)
		})
	}
}

// The kebab-case requirement holds for an embedded rule, and the remedy does
// not: there is no file to rename. A message telling its author to rename one
// sends them looking for a file that was never written.
func TestAnEmbeddedRuleIsNotToldToRenameAFileItHasNot(t *testing.T) {
	_, err := Resolve(t.TempDir(), t.TempDir(), profileFile,
		embedded(t, ruleDoc("Prefer Value Objects", nil)))

	var malformed *MalformedError
	require.ErrorAs(t, err, &malformed)
	assert.Equal(t, "rules[0].id", malformed.Field)
	assert.Contains(t, malformed.Problem, "not kebab-case")
	assert.NotContains(t, malformed.Problem, "renamed")

	// The same id in a file keeps the remedy it does have.
	_, fileErr := Load(filepath.Join(
		rulesDir(t, ruleDoc("Prefer Value Objects", nil)), "Prefer Value Objects"+fileExt))
	require.ErrorAs(t, fileErr, &malformed)
	assert.Contains(t, malformed.Problem, "renamed")
}

// §2.6 item 2 settles a collision between layers and says nothing about one
// inside a layer, because there is no answer to give: neither element is above
// the other. Taking the first would enforce one of two standards its author
// wrote and never mention the other, which is the silence §2.6.5's abort exists
// to prevent.
func TestTwoRulesInOneProfileCannotShareAnID(t *testing.T) {
	_, err := Resolve(t.TempDir(), t.TempDir(), profileFile, embedded(t,
		ruleDoc("handle-every-error", nil),
		ruleDoc("no-raw-sql", nil),
		ruleDoc("handle-every-error", map[string]any{"severity": "critical"}),
	))

	var malformed *MalformedError
	require.ErrorAs(t, err, &malformed)
	assert.Equal(t, "rules[2].id", malformed.Field)
	assert.Contains(t, malformed.Problem, "rules[0]", "the message names the element it collides with")
	assert.Equal(t, profileFile, malformed.File)
}

// §2.6 fixes no corpus order the way §2.5.5 does for roles, but §2.1.1 requires
// the same inputs to give the same result. A directory listing is the operating
// system's order, and `a.json` sorts before `a-b.json` there while by id `a`
// comes before `a-b`, so file order and id order are not the same order.
func TestALayerResolvesAscendingByRuleID(t *testing.T) {
	corpus, err := Resolve(
		rulesDir(t, ruleDoc("no-raw-sql", nil), ruleDoc("a-b", nil), ruleDoc("a", nil)),
		t.TempDir(), profileFile,
		embedded(t, ruleDoc("z-last", nil), ruleDoc("m-middle", nil)),
	)
	require.NoError(t, err)

	assert.Equal(t, []string{"a", "a-b", "no-raw-sql", "m-middle", "z-last"}, ids(corpus))
}

// A rule that resolves keeps every value its own layer gave it. The override
// tests above assert that nothing leaks in; this one asserts the winner is not
// flattened to defaults on the way through.
func TestAResolvedRuleKeepsItsOwnFields(t *testing.T) {
	corpus, err := Resolve(t.TempDir(), t.TempDir(), profileFile, embedded(t,
		ruleDoc("no-raw-sql", map[string]any{
			"severity": "high",
			"kind":     "finding",
			"detect":   map[string]any{"pattern": "DB::raw", "mode": "regex"},
			"globs":    []string{"app/**"},
		}),
	))
	require.NoError(t, err)

	require.Len(t, corpus, 1)
	got := corpus[0].Rule
	assert.Equal(t, finding.SeverityHigh, got.Severity)
	assert.Equal(t, finding.KindFinding, got.Kind)
	require.NotNil(t, got.Detect)
	assert.Equal(t, "DB::raw", got.Detect.Pattern)
	assert.Equal(t, []string{"app/**"}, got.Globs)
}
