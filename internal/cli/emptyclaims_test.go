package cli

import (
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/deligoez/cr/internal/mapping"
)

// An issue that legitimately yields no claims is recorded as an empty claims
// file, and with an empty mapping and every cell filled the round reads
// complete; a round whose claims no `cr claims record` ever stored stays
// incomplete, naming them.
//
// Measured on release QA before the fix: `cr claims record` and `cr map record`
// both accepted the empty files, and `cr status` still gave "§4.6.5: the intent
// axis is active and this round has recorded no claims", because it read an
// empty claim set as claims nobody recorded.
func TestAnEmptyClaimsFileRecordedSatisfiesTheIntentPass(t *testing.T) {
	layout, issue, u := rerecordHome(t)
	meta, err := layout.ReadMeta(fixtureOwner, fixtureProject, fixturePRNumber)
	require.NoError(t, err)
	cells := make([]string, 0, len(meta.ActiveRoles))
	seats := make([]string, 0, len(meta.ActiveRoles))
	for _, role := range meta.ActiveRoles {
		cells = append(cells, `{"unit":"`+u+`","role":"`+role+`","result":"pass"}`)
		seats = append(seats, u+"/"+role)
	}
	rows := "§10.2.2: 1 of 1 unit(s) hold no complete row of cells for all " + strconv.Itoa(len(seats)) +
		" active role(s) at their current unit hash, missing " + strings.Join(seats, ", ")

	unrecorded := readCompleteness(t)
	assert.Equal(t, []string{
		rows,
		"§4.6.5: the intent axis is active and this round has recorded neither its claims nor " +
			"its mapping: record the claims with `cr claims record`, then the mapping with `cr map record`",
	}, unrecorded.Completeness.Reasons, "the control: a round with no claims recording at all")

	empty := filepath.Join(t.TempDir(), "empty.ndjson")
	require.NoError(t, os.WriteFile(empty, nil, 0o600))
	_, err = runCLIPrinting(t, "claims", "record", fixturePR, empty, "--repo", fixtureSlug, "--intent-file", issue)
	require.NoError(t, err)
	assert.Equal(t, []string{
		rows,
		"§4.6.5: the intent axis is active and this round has recorded no mapping: record it with `cr map record`",
	}, readCompleteness(t).Completeness.Reasons, "the empty claims are recorded, and the mapping is not yet")

	_, err = runCLIPrinting(t, "map", "record", fixturePR, empty, "--repo", fixtureSlug)
	require.NoError(t, err)
	assert.Equal(t, []string{rows}, readCompleteness(t).Completeness.Reasons,
		"with the empty mapping recorded, no reason names the intent pass")

	_, err = runCLIPrinting(t, "cells", "record", fixturePR, writeCellsInput(t, cells...), "--repo", fixtureSlug)
	require.NoError(t, err)
	report := readCompleteness(t)
	assert.True(t, report.Completeness.Complete)
	assert.Equal(t, []string{}, report.Completeness.Reasons)
	assert.Equal(t, intentCoverage{Claims: 0, Mapped: 0, Gaps: []mapping.Gap{}}, intentReport(t))
}
