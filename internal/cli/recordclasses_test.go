package cli

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/deligoez/cr/internal/finding"
	"github.com/deligoez/cr/internal/state"
)

// writeClassRole writes a per-repository role file declaring a class
// vocabulary, which §2.5 lets a project own.
func writeClassRole(t *testing.T, dir string, classes ...string) {
	t.Helper()
	quoted := make([]string, 0, len(classes))
	for _, class := range classes {
		quoted = append(quoted, `"`+class+`"`)
	}
	require.NoError(t, os.MkdirAll(dir, 0o750))
	require.NoError(t, os.WriteFile(filepath.Join(dir, "correctness.json"), []byte(
		`{"id":"correctness","title":"Correctness","axis":"correctness","instructions":"Check it.",`+
			`"classes":[`+strings.Join(quoted, ",")+`]}`), 0o600))
}

// §2.5.6: `cr record` reports every record of a role declaring `classes` whose
// class is not one of them, naming the record and the class, and stores it all
// the same. A record whose class is in the vocabulary is reported by nothing.
func TestRecordReportsAClassOutsideItsRolesVocabularyAndStoresItAnyway(t *testing.T) {
	layout := recordedHome(t)
	writeClassRole(t, layout.RepoRolesDir(recordOwner, recordRepo), "off-by-one")
	inside := aRecord("f1", "u1")
	inside["class"] = "off-by-one"
	file := writeRecordFile(t, "merged.ndjson", inside, aRecord("f2", "u2"))

	printed, err := runRecord(t, recordPR, file, "--repo", recordSlug)
	require.NoError(t, err, "§2.5.6 reports, and never rejects")

	var result struct {
		ClassesOutside []classOutside `json:"classes_outside"`
	}
	require.NoError(t, json.Unmarshal([]byte(printed), &result))
	assert.Equal(t, []classOutside{{Record: "f2", Role: "correctness", Class: "unchecked-error"}},
		result.ClassesOutside)

	stored, err := state.ReadRecords[finding.Finding](layout, recordOwner, recordRepo, recordPRNum, state.FileFindings)
	require.NoError(t, err)
	ids := make([]string, 0, len(stored))
	for i := range stored {
		ids = append(ids, stored[i].ID)
	}
	assert.Equal(t, []string{"f1", "f2"}, ids, "both records are stored")
}

// A role declaring no `classes` has no vocabulary to be outside of, so nothing
// is reported however a record is classed.
func TestRecordReportsNoClassWhereTheRoleDeclaresNone(t *testing.T) {
	recordedHome(t)
	file := writeRecordFile(t, "merged.ndjson", aRecord("f1", "u1"))

	printed, err := runRecord(t, recordPR, file, "--repo", recordSlug)
	require.NoError(t, err)

	var result struct {
		ClassesOutside []classOutside `json:"classes_outside"`
	}
	require.NoError(t, json.Unmarshal([]byte(printed), &result))
	assert.Equal(t, []classOutside{}, result.ClassesOutside, "§12: an empty list serialises as [], never null")
}

// At a terminal the report is one line per record, naming the record, the class
// and the role, and saying the record was recorded.
func TestATerminalRecordNamesEveryClassOutsideItsRolesVocabulary(t *testing.T) {
	var printed bytes.Buffer
	require.NoError(t, (&writer{out: &printed, mode: ModeText}).emit(&recordResult{
		Recorded: []*finding.Finding{},
		ClassesOutside: []classOutside{
			{Record: "f2", Role: "correctness", Class: "unchecked-error"},
			{Record: "f3", Role: "correctness", Class: "dropped-error"},
		},
	}))

	assert.Equal(t, []string{
		"recorded 0 in state draft",
		"f2 has class unchecked-error, which is not in role correctness's classes (§2.5.6); it was recorded",
		"f3 has class dropped-error, which is not in role correctness's classes (§2.5.6); it was recorded",
	}, strings.Split(strings.TrimRight(printed.String(), "\n"), "\n"))
}
