package cli

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/deligoez/cr/internal/finding"
	"github.com/deligoez/cr/internal/state"
)

// Invariant 4 through the command, at §6.3.1's first moment: a record the agent
// handed in as `kind: finding` reaches findings.ndjson as a question, because
// §6.2 graded it `argued`.
//
// Two records go in, written by the same fixture and differing only in their
// citations, and one of them keeps the register it arrived in. That is what
// makes the assertion about the forcing rather than about the command: cr did
// not rewrite every kind it was given, it rewrote the one §6.2 left with
// nothing behind it.
//
// The forcing is asserted on disk rather than on the printed payload. §9.1
// keeps the record for the rest of the round and §7.2's triage rewrites it from
// there, so a command that reported a question while storing a finding would
// hand the assertion back at draft time with nothing having refused it.
func TestRecordForcesAnArguedRecordToAQuestion(t *testing.T) {
	layout := gradedHome(t)

	argued := aGradedRecord("f1")
	require.Equal(t, "finding", argued["kind"], "the agent wrote it as an assertion")
	cited := aGradedRecord("f2")
	cited["citations"] = []map[string]any{{"path": "app.go", "line": 1}}

	_, err := runRecord(t, "7", writeRecordFile(t, "merged.ndjson", argued, cited), "--repo", fixtureSlug)
	require.NoError(t, err)

	stored, err := state.ReadRecords[finding.Finding](
		layout, fixtureOwner, fixtureProject, fixturePRNumber, state.FileFindings)
	require.NoError(t, err)
	require.Len(t, stored, 2)

	require.Equal(t, finding.GradeArgued, stored[0].Grade)
	assert.Equal(t, finding.KindQuestion, stored[0].Kind,
		"invariant 4 and §6.3.1: an argued record is forced to a question")

	require.Equal(t, finding.GradeCited, stored[1].Grade)
	assert.Equal(t, finding.KindFinding, stored[1].Kind,
		"§6.2 lets a cited record assert, so the agent's register stands")
}

// No flag on `cr record` turns the forcing off, per §6.3.3.
//
// The command is run again with every global flag §11.1 gives it, and the
// record still lands as a question. This is a weaker statement than §6.3.3's —
// which is about flags, settings, environment variables, profile fields and
// role instructions alike, and is answered structurally by §2.7's table holding
// no key for the forcing — but it is the half that can be exercised, and it is
// the half a future flag would break first.
func TestNoFlagOnRecordTurnsTheForcingOff(t *testing.T) {
	for _, flags := range [][]string{
		{},
		{"--quiet"},
		{"--json"},
		{"--no-color"},
		{"--compact"},
	} {
		t.Run("cr record "+flagsNamed(flags), func(t *testing.T) {
			layout := gradedHome(t)
			file := writeRecordFile(t, "merged.ndjson", aGradedRecord("f1"))

			_, err := runRecord(t, append(
				[]string{"7", file, "--repo", fixtureSlug}, flags...)...)
			require.NoError(t, err)

			stored, err := state.ReadRecords[finding.Finding](
				layout, fixtureOwner, fixtureProject, fixturePRNumber, state.FileFindings)
			require.NoError(t, err)
			require.Len(t, stored, 1)
			assert.Equal(t, finding.KindQuestion, stored[0].Kind,
				"§6.3.3: no flag disables or overrides the forcing")
		})
	}
}

// flagsNamed renders a flag set for a subtest name, and says so when it is
// empty rather than leaving the name trailing off.
func flagsNamed(flags []string) string {
	if len(flags) == 0 {
		return "with no flags"
	}
	var named strings.Builder
	for _, flag := range flags {
		named.WriteString(flag + " ")
	}
	return named.String()[:len(named.String())-1]
}
