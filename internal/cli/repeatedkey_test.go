package cli

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/deligoez/cr/internal/finding"
	"github.com/deligoez/cr/internal/state"
)

// writeLineWith writes one NDJSON line holding line's fields followed by added,
// which is raw JSON text beginning with a comma, and returns the file's path.
// A map cannot hold one key twice, so a line that gives one is spliced.
func writeLineWith(t *testing.T, name string, line map[string]any, added string) string {
	t.Helper()
	body, err := json.Marshal(line)
	require.NoError(t, err)
	path := filepath.Join(t.TempDir(), name)
	require.NoError(t, os.WriteFile(path, []byte(string(body[:len(body)-1])+added+"}\n"), 0o600))
	return path
}

// readStored reads a stored file's bytes, so a refusal can be shown to have
// left it as it was.
func readStored(t *testing.T, path string) string {
	t.Helper()
	held, err := os.ReadFile(path)
	require.NoError(t, err)
	return string(held)
}

// §6.1.4 through `cr record` with the citations key given twice. The decode
// fills the entries from the first array and keeps every field the second
// leaves out, so the first copy's content_hash, or its origin beside a record
// naming a rule, would be stored while the fence read only the second copy.
// The line is refused with exit code 1 naming the file, the line and the key,
// and findings.ndjson is left as it was. The control gives the citation once.
func TestRecordRefusesTheCitationsKeyGivenTwice(t *testing.T) {
	cited := aRecord("f1", "u1")
	citation := `"citations":[{"path":"` + recordPath + `","line":42}]`

	t.Run("given once", func(t *testing.T) {
		layout := recordedHome(t)
		path := writeLineWith(t, "merged.ndjson", cited, ","+citation)

		_, err := runRecord(t, recordPR, path, "--repo", recordSlug)

		require.NoError(t, err)
		stored, err := state.ReadRecords[finding.Finding](
			layout, recordOwner, recordRepo, recordPRNum, state.FileFindings)
		require.NoError(t, err)
		require.Len(t, stored, 1)
		hash, err := finding.AnchorContentHash([]string{"handler line 42"})
		require.NoError(t, err)
		assert.Equal(t, []finding.Citation{{Path: recordPath, Line: 42, ContentHash: hash, Origin: finding.OriginAgent}},
			stored[0].Citations, "cr stamps the hash of the line the one citation names")
	})

	ruled := aRecord("f1", "u1")
	ruled["rule"] = "r1"
	for name, tc := range map[string]struct {
		line  map[string]any
		first string
	}{
		"with a content hash": {
			line: cited, first: `{"path":"` + recordPath + `","line":42,"content_hash":"x"}`,
		},
		"with an origin beside a rule": {
			line: ruled, first: `{"path":"` + recordPath + `","line":42,"origin":"rule"}`,
		},
	} {
		t.Run(name, func(t *testing.T) {
			layout := recordedHome(t)
			stored := layout.PRFile(recordOwner, recordRepo, recordPRNum, state.FileFindings)
			before := readStored(t, stored)
			path := writeLineWith(t, "merged.ndjson", tc.line, `,"citations":[`+tc.first+`],`+citation)

			_, err := runRecord(t, recordPR, path, "--repo", recordSlug)

			var repeated *state.RepeatedKeyError
			require.ErrorAs(t, err, &repeated)
			assert.Equal(t, &state.RepeatedKeyError{File: path, Line: 1, Key: "citations"}, repeated)
			assert.Equal(t, ExitValidation, exitCodeFor(err))
			assert.Equal(t, before, readStored(t, stored), "the refused file leaves the round as it found it")
		})
	}
}

// §2.6.1.3 through `cr record`: a rule key inside a citations entry is refused
// with exit code 1 naming the entry, not dropped on the way to
// findings.ndjson, and the key given twice inside the entry is refused as a
// repeated key before the entry is read at all.
func TestRecordRefusesARuleInsideACitation(t *testing.T) {
	ruled := aRecord("f1", "u1")
	ruled["rule"] = "r1"
	for name, tc := range map[string]struct {
		entry string
		want  error
	}{
		"once": {
			entry: `"rule":"r1"`,
			want: &finding.RejectedRecordError{
				Line: 1, Field: "citations[0].rule",
				Problem: "is not a citation's to carry: §2.6.1.3 puts the rule id on the record",
			},
		},
		"twice": {
			entry: `"rule":"r1","Rule":null`,
			want:  &state.RepeatedKeyError{Line: 1, Key: "citations[0].rule"},
		},
	} {
		t.Run(name, func(t *testing.T) {
			layout := recordedHome(t)
			stored := layout.PRFile(recordOwner, recordRepo, recordPRNum, state.FileFindings)
			before := readStored(t, stored)
			path := writeLineWith(t, "merged.ndjson", ruled,
				`,"citations":[{"path":"`+recordPath+`","line":42,`+tc.entry+`}]`)

			_, err := runRecord(t, recordPR, path, "--repo", recordSlug)

			switch want := tc.want.(type) {
			case *finding.RejectedRecordError:
				var rejected *finding.RejectedRecordError
				require.ErrorAs(t, err, &rejected)
				want.File = path
				assert.Equal(t, want, rejected)
			case *state.RepeatedKeyError:
				var repeated *state.RepeatedKeyError
				require.ErrorAs(t, err, &repeated)
				want.File = path
				assert.Equal(t, want, repeated)
			}
			assert.Equal(t, ExitValidation, exitCodeFor(err))
			assert.Equal(t, before, readStored(t, stored), "the refused file leaves the round as it found it")
		})
	}
}

// §3.3 through `cr claims record` with note_id given twice. A later null leaves
// the string the earlier copy set, so a claim drawn from the issue text would be
// stored naming a note while the fence, reading only the null, found no
// note_id. The line is refused with exit code 1 naming the file, the line and
// the key, under one spelling and under two, and claims.ndjson is left as it
// was. The control is the same claim with no note_id.
func TestClaimsRecordRefusesNoteIDGivenTwice(t *testing.T) {
	claim := map[string]any{
		"id": claimsIssue + "#c1", "text": "Back off exponentially.",
		"source": "acceptance", "span": "backs off exponentially",
	}
	run := func(t *testing.T, path string) error {
		t.Helper()
		return runClaimsRecord(t, claimsPR, path, "--repo", claimsSlug, "--intent-file", anIssueFile(t))
	}

	t.Run("given never", func(t *testing.T) {
		claimedHome(t)
		require.NoError(t, run(t, writeLineWith(t, "claims.ndjson", claim, "")))
	})
	for name, added := range map[string]string{
		"under one spelling":  `,"note_id":"` + claimsIssue + `#n2","note_id":null`,
		"under two spellings": `,"note_id":"` + claimsIssue + `#n2","Note_ID":null`,
	} {
		t.Run(name, func(t *testing.T) {
			layout := claimedHome(t)
			stored := layout.PRFile(claimsOwner, claimsRepo, claimsPRNum, state.FileClaims)
			before := readStored(t, stored)
			path := writeLineWith(t, "claims.ndjson", claim, added)

			err := run(t, path)

			var repeated *state.RepeatedKeyError
			require.ErrorAs(t, err, &repeated)
			assert.Equal(t, &state.RepeatedKeyError{File: path, Line: 1, Key: "note_id"}, repeated)
			assert.Equal(t, ExitValidation, exitCodeFor(err))
			assert.Equal(t, before, readStored(t, stored), "the refused file leaves the round as it found it")
		})
	}
}
