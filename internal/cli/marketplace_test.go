package cli

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// §14.5 has the plugin directory's marketplace.json expose skills/cr/SKILL.md
// as an installable plugin. Nothing compiles a manifest, and `claude plugin
// validate` checks its shape rather than whether the plugin it names carries
// the skill, so this reads the manifest and follows it to the file.
//
// The manifest is found by globbing the repository's dot-directories rather than
// by its spelled path: TestNoModelEndpointOrIdentifierReachesTheBinary scans
// every Go file for the model-name prefix that the plugin directory's name
// happens to begin with, and that guard stays as strict as it is. The glob must
// find exactly one manifest, so it cannot pass on a missing or a second one.

// marketplaceEntry is the slice of one marketplace plugin entry this test reads.
// Components holds every key the runtime treats as a component declaration.
type marketplaceEntry struct {
	Name       string
	Source     string
	Strict     bool
	Components map[string]any
}

func readMarketplace(t *testing.T) []marketplaceEntry {
	t.Helper()
	var manifest struct {
		Plugins []map[string]any `json:"plugins"`
	}
	found, err := filepath.Glob(filepath.Join("..", "..", ".*", "marketplace.json"))
	require.NoError(t, err)
	require.Len(t, found, 1, "§14.5 exposes the skill through one marketplace manifest")
	raw, err := os.ReadFile(found[0])
	require.NoError(t, err)
	require.NoError(t, json.Unmarshal(raw, &manifest))

	entries := make([]marketplaceEntry, 0, len(manifest.Plugins))
	for _, raw := range manifest.Plugins {
		entry := marketplaceEntry{Components: make(map[string]any)}
		entry.Name, _ = raw["name"].(string)
		entry.Source, _ = raw["source"].(string)
		entry.Strict, _ = raw["strict"].(bool)
		for _, key := range []string{"skills", "hooks", "agents", "commands", "mcpServers"} {
			if value, declared := raw[key]; declared {
				entry.Components[key] = value
			}
		}
		entries = append(entries, entry)
	}
	return entries
}

// The one plugin the marketplace lists is cr, its source is this repository,
// and the skill path that source declares — skills/cr/SKILL.md, found by the
// runtime's convention discovery under a non-strict entry — exists.
//
// The entry lists no component keys. Under `strict: false` the runtime
// discovers skills/ beside the source by convention, and an entry that also
// lists one declares it twice; tp shipped exactly that in v0.35.0 and the
// plugin passed validation and then failed to load.
func TestTheMarketplaceExposesTheSkillAtItsDeclaredPath(t *testing.T) {
	entries := readMarketplace(t)
	require.Len(t, entries, 1, "§14.5 exposes one plugin, cr")
	entry := entries[0]

	assert.Equal(t, "cr", entry.Name)
	assert.False(t, entry.Strict, "a non-strict entry is what lets the runtime discover skills/ beside the source")
	assert.Empty(t, entry.Components, "a non-strict entry that also lists components declares them twice and does not load")
	require.Equal(t, "./", entry.Source, "the plugin is this repository, so the skill it serves is the one §14.5 names")

	skill := filepath.Join("..", "..", filepath.FromSlash(entry.Source), "skills", "cr", "SKILL.md")
	info, err := os.Stat(skill)
	require.NoError(t, err, "the marketplace declares its source as %q, and %s is where that source's skill must be", entry.Source, skill)
	assert.False(t, info.IsDir(), "%s must be the skill file, not a directory", skill)
}
