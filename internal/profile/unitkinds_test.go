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

