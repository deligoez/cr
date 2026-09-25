package cli

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/deligoez/cr/internal/observation"
	"github.com/deligoez/cr/internal/state"
)

// observationsFile writes an NDJSON file outside the state tree and returns its
// path, for `cr observations record` to read.
func observationsFile(t *testing.T, lines string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "observations.ndjson")
	require.NoError(t, os.WriteFile(path, []byte(lines), 0o600))
	return path
}

// The measured case §4.6.9 exists for: a role saw the defect the change fixed
// in one file still standing in a sibling file outside the diff. The line is
// stored, stamped with the round, and shown by `cr status` and in the header of
// `cr draft` — and nowhere a posted body is built from.
func TestAnObservationIsStoredAndShownByStatusAndTheDraftHeader(t *testing.T) {
	statusHome(t)
	seen := `{"path":"lib.go","line":4,"text":"store() is still called without the locale in the sibling loader."}`

	printed, err := runCLIPrinting(t, "observations", "record", fixturePR, observationsFile(t, seen+"\n"),
		"--repo", fixtureSlug)
	require.NoError(t, err)
	var recorded observationsRecordResult
	require.NoError(t, json.Unmarshal([]byte(printed), &recorded))
	require.Len(t, recorded.Recorded, 1)
	assert.Equal(t, 1, recorded.Round)

	layout, err := state.Default()
	require.NoError(t, err)
	stored, err := state.ReadRecords[observation.Observation](
		layout, fixtureOwner, fixtureProject, fixturePRNumber, state.FileObservations)
	require.NoError(t, err)
	require.Len(t, stored, 1)
	assert.Equal(t, "lib.go", stored[0].Path)
	assert.Equal(t, 4, stored[0].Line)
	assert.Equal(t, 1, stored[0].Round, "cr writes the round the observation was stored in")
	assert.NotEmpty(t, stored[0].Head, "and the head it resolved against")

	reported, err := runCLIPrinting(t, "status", fixturePR, "--repo", fixtureSlug)
	require.NoError(t, err)
	var status struct {
		Observations []observation.Observation `json:"observations"`
	}
	require.NoError(t, json.Unmarshal([]byte(reported), &status))
	require.Len(t, status.Observations, 1, "§4.6.9: `cr status` shows every observation of the round")
	assert.Equal(t, stored[0].Text, status.Observations[0].Text)

	_, err = runCLIPrinting(t, "draft", fixturePR, "--repo", fixtureSlug)
	require.NoError(t, err)
	drafted, err := os.ReadFile(layout.RoundFile(fixtureOwner, fixtureProject, fixturePRNumber, 1, state.FileDraft))
	require.NoError(t, err)
	assert.Contains(t, string(drafted),
		"  lib.go:4: store() is still called without the locale in the sibling loader.",
		"§4.6.9: `cr draft` shows the observation in the header cr owns")
	assert.Contains(t, string(drafted), "observations: 1 outside the round's units, shown here and never posted (§4.6.9)")
}

// §4.6.9 refuses, with exit code 1 and nothing stored, a line missing `path`
// or `text`, one whose `path:line` does not resolve at the head, and — as every
// recorded file does — a key the schema does not name, a key given twice, and
// a `round` the agent supplied.
func TestAnObservationThatDoesNotResolveIsRefusedWithExitOne(t *testing.T) {
	for name, line := range map[string]string{
		"no path":           `{"line":4,"text":"seen"}`,
		"no text":           `{"path":"lib.go","line":4}`,
		"a blank text":      `{"path":"lib.go","line":4,"text":"  "}`,
		"a missing file":    `{"path":"loader.go","line":1,"text":"seen"}`,
		"a line past it":    `{"path":"lib.go","line":40,"text":"seen"}`,
		"a zero line":       `{"path":"lib.go","line":0,"text":"seen"}`,
		"an unknown key":    `{"path":"lib.go","line":4,"text":"seen","severity":"high"}`,
		"a key twice":       `{"path":"lib.go","Path":"app.go","line":4,"text":"seen"}`,
		"a supplied round":  `{"path":"lib.go","line":4,"text":"seen","round":1}`,
		"a missing file 2x": `{"path":"lib.go","text":"seen"}` + "\n" + `{"path":"gone.go","text":"seen"}`,
	} {
		t.Run(name, func(t *testing.T) {
			statusHome(t)

			err := runCLI(t, "observations", "record", fixturePR, observationsFile(t, line+"\n"), "--repo", fixtureSlug)

			require.Error(t, err)
			assert.Equal(t, ExitValidation, exitCodeFor(err), err.Error())
			layout, err := state.Default()
			require.NoError(t, err)
			stored, err := state.ReadRecords[observation.Observation](
				layout, fixtureOwner, fixtureProject, fixturePRNumber, state.FileObservations)
			require.NoError(t, err)
			assert.Empty(t, stored, "a refused file stores nothing, the good lines above the bad one included")
		})
	}
}

// §4.6.9 allows an observation about a file as a whole: `line` left out, the
// path held to the head alone.
func TestAnObservationWithoutALineNamesTheFile(t *testing.T) {
	statusHome(t)

	printed, err := runCLIPrinting(t, "observations", "record", fixturePR,
		observationsFile(t, `{"path":"lib.go","text":"The loader has no test."}`+"\n"), "--repo", fixtureSlug)
	require.NoError(t, err)

	var recorded observationsRecordResult
	require.NoError(t, json.Unmarshal([]byte(printed), &recorded))
	require.Len(t, recorded.Recorded, 1)
	assert.Equal(t, "lib.go", recorded.Recorded[0].Location())
}

