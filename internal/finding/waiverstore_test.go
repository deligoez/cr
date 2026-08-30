package finding

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/deligoez/cr/internal/state"
)

// The pull request every waiver below is written from. §7.4.8 records it on a
// repository-wide waiver as provenance and §7.4.4 scopes a `not-here` one to
// it, so one number serves both and the tests can tell the two roles apart.
const (
	waiverOwner = "octocat"
	waiverRepo  = "hello"
	waiverPR    = 7
)

// waiverHome is a state root with §2.2's tree in place.
func waiverHome(t *testing.T) state.Layout {
	t.Helper()
	layout := state.New(filepath.Join(t.TempDir(), ".cr"))
	require.NoError(t, layout.Init())
	require.NoError(t, layout.EnsureRepo(waiverOwner, waiverRepo))
	return layout
}

// theProvenance is §7.4.8's four fields, reason included.
func theProvenance() WaiverProvenance {
	return WaiverProvenance{
		Round: 3, PR: waiverPR, Head: "0a1b2c3", Reason: "the legacy area is exempt",
	}
}

// waive writes the waiver one discarded record calls for, through the whole of
// Waive, so every test below exercises the scope derivation rather than a scope
// it chose for itself.
func waive(t *testing.T, layout state.Layout, record *Finding, prov WaiverProvenance) WaiverRecord {
	t.Helper()
	waiver, err := WaiverFor(record)
	require.NoError(t, err)
	recorded, err := Waive(layout, waiverOwner, waiverRepo, &waiver, prov)
	require.NoError(t, err)
	return recorded
}

// storedAt decodes one waiver file straight off disk.
//
// The file is read rather than the reader called, because what §7.4.4 fixes is
// which path the bytes land at: a reader asked for the repository's waivers
// would answer the same whichever file it had been pointed at.
func storedAt(t *testing.T, path string) []WaiverRecord {
	t.Helper()
	body, err := os.ReadFile(path)
	require.NoError(t, err)

	stored := make([]WaiverRecord, 0)
	for _, line := range strings.Split(string(body), "\n") {
		if strings.TrimSpace(line) == "" {
			continue
		}
		var record WaiverRecord
		require.NoError(t, json.Unmarshal([]byte(line), &record))
		stored = append(stored, record)
	}
	return stored
}

// waiverFiles are §7.4.4's two paths for the fixture repository.
func waiverFiles(layout state.Layout) (repository, pullRequest string) {
	return layout.WaiversFile(waiverOwner, waiverRepo),
		layout.PRFile(waiverOwner, waiverRepo, waiverPR, state.FileWaivers)
}

// §7.4.4 puts the two scopes in two files, and the scope follows the
// disposition: `wrong` is a fact about the class and goes to
// `~/.cr/waivers/<owner>/<repo>.ndjson`, `not-here` is a fact about this pull
// request and goes to that pull request's `waivers.ndjson`.
//
// Both records here carry one key, which is what makes the assertion about the
// files rather than about the data: nothing but the disposition separates them,
// so a routing that read anything else would put them in the same file. The
// cross-checks are the half with teeth — §7.4.3 forbids a `not-here` from
// silencing a finding outside its pull request, and a `not-here` line in the
// repository file is exactly that failure, invisible because its cost is a
// finding that stops being raised.
func TestTheTwoScopesOfSection744LandInTheTwoFilesItNames(t *testing.T) {
	layout := waiverHome(t)
	wrong, notHere := theSameDefectAtTheSameCode(t)

	wide := waive(t, layout, &wrong, theProvenance())
	here := waive(t, layout, &notHere, theProvenance())
	repository, pullRequest := waiverFiles(layout)

	assert.Equal(t, []WaiverRecord{wide}, storedAt(t, repository),
		"§7.4.4: a repository-wide waiver is written to ~/.cr/waivers/<owner>/<repo>.ndjson")
	assert.Equal(t, DispositionWrong, wide.Disposition)
	assert.Equal(t, "wr1", wide.ID)

	assert.Equal(t, []WaiverRecord{here}, storedAt(t, pullRequest),
		"§7.4.4: a pull-request-scoped waiver is written to that PR's waivers.ndjson per §2.3")
	assert.Equal(t, DispositionNotHere, here.Disposition)
	assert.Equal(t, "wp1", here.ID)

	// §7.1.6 re-ingests a draft's deletions every time the draft is
	// regenerated, so the same key arrives again in the ordinary course.
	t.Run("waiving one key twice writes one line", func(t *testing.T) {
		assert.Equal(t, wide, waive(t, layout, &wrong, theProvenance()))
		assert.Equal(t, here, waive(t, layout, &notHere, theProvenance()))
		assert.Len(t, storedAt(t, repository), 1)
		assert.Len(t, storedAt(t, pullRequest), 1)
	})
}

