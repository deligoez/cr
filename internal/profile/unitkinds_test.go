package profile

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// §2.4.5: laravel-pest declares the two unit kinds of the measured case, a
// changelog and a translation file, each read by the intent role alone.
//
// Measured on a real pull request on a private Laravel repository: 52 prompts
// were emitted and some 15 carried meaning, and the changelog and translation
// units were a large share of the rest — a correctness, convention and test
// reading of a line of release notes or a translated string finds nothing a
// reader of the intent role would not.
func TestLaravelPestDeclaresTheChangelogAndTranslationKinds(t *testing.T) {
	p := loadLaravelPest(t)

	assert.Equal(t, []UnitKind{
		{Kind: "changelog", Globs: []string{"changelogs/**"}, Roles: []string{"intent-coverage"}},
		{Kind: "translation", Globs: []string{"resources/lang/**", "lang/**"}, Roles: []string{"intent-coverage"}},
	}, p.Units.Kinds)
}

// §4.6.7: a unit is of a kind when every one of its paths matches that kind's
// globs, and the first declared kind that fits is the one it takes.
func TestKindOfTakesTheFirstKindEveryPathMatches(t *testing.T) {
	p := loadLaravelPest(t)

	for path, want := range map[string]string{
		"changelogs/unreleased/fix.yml":  "changelog",
		"resources/lang/tr/messages.php": "translation",
		"lang/en/validation.php":         "translation",
		"app/Models/Order.php":           "",
		"tests/lang/en/Example.php":      "",
	} {
		kind, found := p.KindOf(path)
		assert.Equal(t, want != "", found, path)
		assert.Equal(t, want, kind.Kind, path)
	}
	_, found := p.KindOf("changelogs/a.yml", "app/Models/Order.php")
	assert.False(t, found, "a unit one of whose paths falls outside the globs is of no kind")
	_, found = p.KindOf()
	assert.False(t, found, "a unit with no path is of no kind")
}

// A profile that declares no kinds resolves an empty list, never null, and
// gives every unit no kind.
func TestAProfileWithoutKindsResolvesAnEmptyList(t *testing.T) {
	p, err := Parse(write(t, "laravel-pest", wellFormed), []byte(wellFormed))
	require.NoError(t, err)

	assert.Equal(t, []UnitKind{}, p.Units.Kinds)
	_, found := p.KindOf("changelogs/a.yml")
	assert.False(t, found)
}

// §2.4's `units.kinds` is refused with the entry and the field named when an
// entry cannot be applied: no kind, a kind named twice, no glob, or no roles
// list, so a typo is not read as a kind that matches nothing.
func TestAMalformedUnitKindIsRefusedNamingTheField(t *testing.T) {
	for name, tc := range map[string]struct{ kinds, field string }{
		"no kind": {`[{"globs": ["a/**"], "roles": []}]`, "units.kinds[0].kind"},
		"a repeated kind": {`[{"kind": "k", "globs": ["a/**"], "roles": []}, {"kind": "k", "globs": ["b/**"], "roles": []}]`,
			"units.kinds[1].kind"},
		"no glob":       {`[{"kind": "k", "globs": [], "roles": []}]`, "units.kinds[0].globs"},
		"an empty glob": {`[{"kind": "k", "globs": [""], "roles": []}]`, "units.kinds[0].globs"},
		"no roles":      {`[{"kind": "k", "globs": ["a/**"]}]`, "units.kinds[0].roles"},
		"an empty role": {`[{"kind": "k", "globs": ["a/**"], "roles": [""]}]`, "units.kinds[0].roles"},
	} {
		t.Run(name, func(t *testing.T) {
			content := `{
	"id": "laravel-pest",
	"match": {"files": ["artisan"], "globs": ["app/**/*.php"]},
	"axes": {"intent": true},
	"units": {"kinds": ` + tc.kinds + `}
}`
			_, err := Parse(write(t, "laravel-pest", content), []byte(content))
			var malformed *MalformedError
			require.ErrorAs(t, err, &malformed)
			assert.Equal(t, tc.field, malformed.Field)
		})
	}
}
