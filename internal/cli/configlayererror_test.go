package cli

import (
	"fmt"
	"os"
	"path/filepath"
	"testing"

	"github.com/deligoez/cr/internal/config"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// QA D-S01-3 and D-S01-4: a config file cr cannot read, parse or bind to its
// settings' types is a configuration failure, which §11.2 codes 3, and the
// refusal names what a reader has to open — the file, the layer of §2.7 that
// file is, and the key when the fault is one setting's. Before, the plain errors
// of internal/config matched no row of the exit table and fell to 2 with the
// usage hint, telling the reader to retype a command line that was right, and
// render.lang's refusal named the setting and not which of five layers said it.
//
// Each fixture is driven through `cr config` and through `cr brief`, because
// the code and the hint have to survive the path a user takes to them, and
// brief is a pull-request command that resolves configuration on its own.
// TestMain's gh fence keeps a brief the configuration failed to stop off the
// network.
func TestAConfigLayerCrCannotUseExitsThreeNamingTheFileKeyAndLayer(t *testing.T) {
	for _, fixture := range []struct {
		name string
		// global writes the global file rather than the per-repository one.
		global bool
		body   string
		key    string
		layer  string
		// message and hint are the whole texts, with %s the file's path.
		message, hint string
	}{
		{
			name: "an unparseable global config", global: true,
			body:  `{not json`,
			layer: config.LayerGlobalConfig,
			message: "the global config file %s cannot be used: it does not parse as a JSON object: " +
				"invalid character 'n' looking for beginning of object key string",
			hint: "repair the global config file %s so it parses as a JSON object, or remove it: " +
				"a missing file supplies nothing",
		},
		{
			name:  "a mistyped per-repository value",
			body:  `{"post": {"max_comments": "many"}}`,
			key:   "post.max_comments",
			layer: config.LayerRepoConfig,
			message: `post.max_comments, as the per-repository config file %s sets it, is invalid: ` +
				`expected a whole number, got "many"`,
			hint: "correct post.max_comments in the per-repository config file %s to a value the " +
				"message says it accepts, or remove it there so a lower layer supplies it",
		},
		{
			name:  "a render.lang cr does not render",
			body:  `{"render": {"lang": "de"}}`,
			key:   "render.lang",
			layer: config.LayerRepoConfig,
			message: `render.lang, as the per-repository config file %s sets it, is invalid: ` +
				`render.lang is "de", which is not a language cr renders; v0.2 has exactly tr and en, ` +
				`whose §8.1.4 question labels are built in rather than configured — set render.lang to one of them`,
			hint: "correct render.lang in the per-repository config file %s to a value the " +
				"message says it accepts, or remove it there so a lower layer supplies it",
		},
	} {
		t.Run(fixture.name, func(t *testing.T) {
			root := crHome(t)
			path := filepath.Join(root, "config.json")
			if !fixture.global {
				path = filepath.Join(root, "repos", "acme", "web", "config.json")
			}
			require.NoError(t, os.MkdirAll(filepath.Dir(path), 0o700))
			require.NoError(t, os.WriteFile(path, []byte(fixture.body), 0o600))

			for _, args := range [][]string{
				{"config", "--repo", "acme/web"},
				{"config", "--resolved", "--repo", "acme/web"},
				{"brief", "1", "--repo", "acme/web"},
			} {
				printed, err := runIn(t, args...)
				require.Error(t, err, "%v", args)
				assert.Empty(t, printed, "%v: a configuration cr refuses prints no answer", args)
				assert.Equal(t, ExitFile, exitCodeFor(err), "%v: §11.2 codes a configuration failure 3", args)

				var layer *config.LayerError
				require.ErrorAs(t, err, &layer, "%v", args)
				assert.Equal(t, config.Origin{From: fixture.layer, Source: path}, layer.Origin, "%v", args)
				assert.Equal(t, fixture.key, layer.Key, "%v", args)

				assert.Equal(t, fmt.Sprintf(fixture.message, path), err.Error(), "%v", args)
				assert.Equal(t, fmt.Sprintf(fixture.hint, path), hintFor(err), "%v", args)
			}
		})
	}
}
