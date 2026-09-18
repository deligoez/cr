package cli

import (
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/deligoez/cr/internal/axis"
	"github.com/deligoez/cr/internal/review"
	"github.com/deligoez/cr/internal/state"
)

// unjoinedClaims is the claims section §4.6.5's first pass gives every intent
// prompt of round 1, over the claim lines listed.
func unjoinedClaims(lines ...string) string {
	return "No mapping is recorded for round 1 yet, so no claim is known to be mapped to this unit (§4.6.5). " +
		"The round's claims are below, whole. Mapping them to the round's units is this pass's output, " +
		"and `cr map record` stores it (§4.1.6).\n\n" + strings.Join(lines, "\n") + "\n"
}

// intentClaimSections is the claims section of every prompt `cr review --axis
// intent` emits, keyed by unit.
func intentClaimSections(t *testing.T) map[string]string {
	t.Helper()
	sections := map[string]string{}
	prompts := fanOut(t, "--axis", axis.Intent).Prompts
	for i := range prompts {
		prompt := &prompts[i]
		require.Equal(t, axis.Intent, prompt.Axis)
		sections[prompt.Unit] = promptSection(t, prompt.Text, "Claims mapped to this unit (§4.1.6)")
	}
	return sections
}

// recordClaimsFile runs `cr claims record` over lines against issue.
func recordClaimsFile(t *testing.T, issue string, lines ...string) {
	t.Helper()
	file := filepath.Join(t.TempDir(), "claims.ndjson")
	body := ""
	if len(lines) > 0 {
		body = strings.Join(lines, "\n") + "\n"
	}
	require.NoError(t, os.WriteFile(file, []byte(body), 0o600))
	_, err := runCLIPrinting(t, "claims", "record", fixturePR, file, "--repo", fixtureSlug, "--intent-file", issue)
	require.NoError(t, err)
}

// rerecordClaims are two claims rerecordIssue supports.
var rerecordClaims = []string{
	`{"id":"` + fixtureIssue + `#c1","text":"Load parses.","source":"description","span":"load parses the file"}`,
	`{"id":"` + fixtureIssue + `#c2","text":"Retries back off.","source":"description","span":"Retries back off"}`,
}

// rerecordClaimLines are rerecordClaims as a first-pass prompt lists them.
var rerecordClaimLines = []string{
	"- " + fixtureIssue + "#c1: Load parses.",
	"- " + fixtureIssue + "#c2: Retries back off.",
}

// Field feedback 2.2 through `cr review`: §4.6.5's first intent pass carries
// the claims, so before `cr claims record` has run for the round it refuses
// with exit 4 naming that command and writes no fan-out, while the remaining
// axes keep their own mapping refusal. Once claims are recorded — an empty
// file included — the pass emits, and every prompt carries what was recorded.
func TestTheIntentPassRefusesARoundWithNoClaimsRecorded(t *testing.T) {
	layout, issue, u := rerecordHome(t)
	meta, err := layout.ReadMeta(fixtureOwner, fixtureProject, fixturePRNumber)
	require.NoError(t, err)

	_, err = runCLIPrinting(t, "review", fixturePR, "--repo", fixtureSlug, "--axis", axis.Intent)
	var required *review.ClaimsRequiredError
	require.ErrorAs(t, err, &required)
	assert.Equal(t, review.ClaimsRequiredError{
		Round: 1, Head: meta.Head, Owner: fixtureOwner, Repo: fixtureProject, PR: fixturePRNumber, IntentFile: issue,
	}, *required)
	assert.Equal(t, "round 1 at head "+meta.Head+" has recorded no claims, so the intent pass cannot carry them; "+
		"§4.6.5's first pass emits the units and the claims: run `cr claims record "+fixturePR+" --repo "+
		fixtureSlug+" --intent-file "+issue+" <file.ndjson>`, with an empty file when the issue yields no claim, "+
		"and run this again", err.Error(), "v0.2.2 QA D-S22-3: the round was briefed from a file")
	assert.Equal(t, ExitState, exitCodeFor(err))
	assert.Equal(t, "record the round's claims with `cr claims record <pr> <file.ndjson>` first, "+
		"an empty file when the issue yields none", hintFor(err))
	_, statErr := os.Stat(layout.FanOutDir(fixtureOwner, fixtureProject, fixturePRNumber, 1, u))
	assert.ErrorIs(t, statErr, fs.ErrNotExist, "a refused pass writes no fan-out directory")

	_, err = runCLIPrinting(t, "review", fixturePR, "--repo", fixtureSlug)
	var unmapped *review.MappingRequiredError
	require.ErrorAs(t, err, &unmapped, "the remaining axes are refused for the mapping, as before")

	recordClaimsFile(t, issue)
	assert.Equal(t, map[string]string{u: unjoinedClaims("The round recorded no claim.")},
		intentClaimSections(t), "an empty claims file counts as recorded")

	recordClaimsFile(t, issue, rerecordClaims...)
	assert.Equal(t, map[string]string{u: unjoinedClaims(rerecordClaimLines...)}, intentClaimSections(t),
		"the recorded claims are what the first pass carries")
}

// v0.2.2 QA D-S22-3 from the other side: a round `cr brief` last read through
// the tracker command is refused naming `cr claims record` without
// `--intent-file`, since running it that way reads the same tracker. A brief
// given a relative file afterwards has the refusal name it absolute.
func TestTheIntentPassRefusalNamesTheIntentFileTheLastBriefRead(t *testing.T) {
	_, issue, _ := rerecordHome(t)
	tracker := filepath.Join(t.TempDir(), "tracker")
	require.NoError(t, os.WriteFile(tracker, []byte("#!/bin/sh\ncat "+issue+"\n"), 0o700))
	t.Setenv("CR_INTENT_CMD", `["`+tracker+`","issue","view","{key}"]`)
	refusal := func(intentFile string) string {
		t.Helper()
		_, err := runCLIPrinting(t, "review", fixturePR, "--repo", fixtureSlug, "--axis", axis.Intent)
		var required *review.ClaimsRequiredError
		require.ErrorAs(t, err, &required)
		return "round 1 at head " + required.Head + " has recorded no claims, so the intent pass cannot carry them; " +
			"§4.6.5's first pass emits the units and the claims: run `cr claims record " + fixturePR + " --repo " +
			fixtureSlug + intentFile + " <file.ndjson>`, with an empty file when the issue yields no claim, " +
			"and run this again"
	}

	_, err := runCLIPrinting(t, "brief", fixturePR, "--repo", fixtureSlug, "--issue", fixtureIssue)
	require.NoError(t, err)
	_, err = runCLIPrinting(t, "review", fixturePR, "--repo", fixtureSlug, "--axis", axis.Intent)
	assert.Equal(t, refusal(""), err.Error(), "a tracker brief names no --intent-file")

	t.Chdir(filepath.Dir(issue))
	_, err = runCLIPrinting(t, "brief", fixturePR, "--repo", fixtureSlug, "--issue", fixtureIssue,
		"--intent-file", filepath.Base(issue))
	require.NoError(t, err)
	_, err = runCLIPrinting(t, "review", fixturePR, "--repo", fixtureSlug, "--axis", axis.Intent)
	assert.Equal(t, refusal(" --intent-file "+issue), err.Error(), "a relative file is named absolute")
}

// claimsStampRemoved takes meta.json's claims stamp off the round, which is a
// round whose claims were stored before the stamp existed, or none at all.
func claimsStampRemoved(t *testing.T, layout state.Layout) {
	t.Helper()
	meta, err := layout.ReadMeta(fixtureOwner, fixtureProject, fixturePRNumber)
	require.NoError(t, err)
	meta.ClaimsRound, meta.ClaimsHead = 0, ""
	held, err := layout.LockPR(fixtureOwner, fixtureProject, fixturePRNumber)
	require.NoError(t, err)
	require.NoError(t, held.WriteMeta(&meta))
	require.NoError(t, held.Unlock())
}

// The refusal reads claims as recorded the way §10.2.5's completeness reason
// does: a round holding claims and no stamp was recorded before the stamp
// existed, and emits.
func TestTheIntentPassEmitsOverClaimsHeldWithoutAStamp(t *testing.T) {
	layout, issue, u := rerecordHome(t)
	recordClaimsFile(t, issue, rerecordClaims...)
	claimsStampRemoved(t, layout)

	assert.Equal(t, map[string]string{u: unjoinedClaims(rerecordClaimLines...)}, intentClaimSections(t))
}

// §4.6.5's re-emission once a mapping is stored is not the first pass, and is
// not refused whatever the claims stamp says.
func TestTheIntentReEmissionIsNotRefusedForClaims(t *testing.T) {
	layout, issue, u := rerecordHome(t)
	recordClaimsFile(t, issue)
	empty := filepath.Join(t.TempDir(), "mapping.ndjson")
	require.NoError(t, os.WriteFile(empty, nil, 0o600))
	_, err := runCLIPrinting(t, "map", "record", fixturePR, empty, "--repo", fixtureSlug)
	require.NoError(t, err)
	claimsStampRemoved(t, layout)

	prompts := fanOut(t, "--axis", axis.Intent).Prompts
	units := make([]string, 0, len(prompts))
	for _, prompt := range prompts {
		units = append(units, prompt.Unit)
	}
	assert.Equal(t, []string{u}, units, "the unit mapped to zero claims is re-emitted")
}

// §9.3.4 carries the closing round's claims into the round a moved head opens,
// and the claims stamp moves with them, so that round's intent pass emits and
// carries them — whether the carried set holds claims or is empty.
func TestTheIntentPassEmitsOverClaimsCarriedIntoAMovedHeadsRound(t *testing.T) {
	for name, carried := range map[string][]string{"claims": rerecordClaims, "empty": nil} {
		t.Run(name, func(t *testing.T) {
			layout, issue, _ := rerecordHome(t)
			recordClaimsFile(t, issue, carried...)

			dir, err := repoDir()
			require.NoError(t, err)
			require.NoError(t, os.WriteFile(filepath.Join(dir, "lib.go"),
				[]byte("package lib\n\nfunc Load() {\n\tparse()\n\tstore()\n}\n"), 0o600))
			mustGit(t, dir, "commit", "--quiet", "-am", "a later push")
			head := strings.TrimSpace(mustGit(t, dir, "rev-parse", fixtureHeadBranch))
			base := strings.TrimSpace(mustGit(t, dir, "rev-parse", "main"))
			t.Setenv("PATH", ghShim(t, t.TempDir(), head, base)+string(os.PathListSeparator)+os.Getenv("PATH"))
			_, err = runCLIPrinting(t, "brief", fixturePR, "--repo", fixtureSlug,
				"--issue", fixtureIssue, "--intent-file", issue)
			require.NoError(t, err)
			meta, err := layout.ReadMeta(fixtureOwner, fixtureProject, fixturePRNumber)
			require.NoError(t, err)
			require.Equal(t, 2, meta.Round, "the control: the moved head opened round 2")

			want := "The round recorded no claim."
			if carried != nil {
				want = strings.Join(rerecordClaimLines, "\n")
			}
			sections := intentClaimSections(t)
			require.NotEmpty(t, sections)
			for unitID, section := range sections {
				assert.Equal(t, strings.Replace(unjoinedClaims(want), "round 1", "round 2", 1), section, unitID)
			}
		})
	}
}
