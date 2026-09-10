package cli

import (
	"encoding/json"
	"os"
	"sort"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/deligoez/cr/internal/finding"
	"github.com/deligoez/cr/internal/state"
)

// readRendered decodes the current round's rendered.json as raw fields, so a
// test sees every key the document holds, whatever its value's shape.
func readRendered(t *testing.T, layout state.Layout) map[string]json.RawMessage {
	t.Helper()
	body, err := layout.ReadRound(draftOwner, draftRepo, draftPRNum, draftRound, state.FileRendered)
	require.NoError(t, err)
	var doc map[string]json.RawMessage
	require.NoError(t, json.Unmarshal(body, &doc))
	return doc
}

// §7.1.5 through the command: rendered.json holds one entry per queued record,
// keyed by its id, each the record's agent region as a string, and no
// aggregate hash beside them.
//
// The document is read as raw fields rather than into the map cr writes, so a
// key of any other shape — a hash over all the regions, a count, a round —
// would be seen here instead of being dropped by the decoder. The records in
// other states are in the fixture so "per record" is shown to mean per record
// the draft holds.
func TestRenderedJSONHoldsOneEntryPerRecordAndNoAggregateHash(t *testing.T) {
	first, second := aStoredRecord("f1", finding.StateDraft), aStoredRecord("f2", finding.StateDraft)
	layout := draftedHome(t, first, aStoredRecord("f3", finding.StateDuplicate), second)

	_, err := runDraft(t, draftPR, "--repo", draftSlug)
	require.NoError(t, err)

	doc := readRendered(t, layout)
	keys := make([]string, 0, len(doc))
	for key := range doc {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	assert.Equal(t, []string{"f1", "f2"}, keys, "§7.1.5: one entry per record, and nothing else")

	for id, record := range map[string]*finding.Finding{"f1": first, "f2": second} {
		var region string
		require.NoError(t, json.Unmarshal(doc[id], &region), "each entry is the region as text")
		assert.Equal(t, record.Summary+"\n\n"+record.Evidence, region,
			"§8.1.2's initial body, exactly as rendered")
		assert.Contains(t, readDraft(t, layout), region, "and exactly as draft.md carries it")
	}
}

// §7.1.5: rendered.json holds only the body cr generated, so an edit to
// draft.md never becomes an entry — not on the run after it, and not on the run
// after that.
//
// This is the third-draft case the criterion names. Were the reviewer's body
// written into rendered.json by the second run, the third would find the draft
// equal to its entry, take the block for untouched, and put cr's English back
// over the reviewer's prose. The body is rewritten here the way an agent
// rewrites it into `render.lang`, and the draft is run twice more.
func TestAnEditedBodyNeverBecomesItsRenderedEntry(t *testing.T) {
	record := aStoredRecord("f1", finding.StateDraft)
	layout := draftedHome(t, record)
	generated := record.Summary + "\n\n" + record.Evidence

	_, err := runDraft(t, draftPR, "--repo", draftSlug)
	require.NoError(t, err)

	for run := 2; run <= 3; run++ {
		edited := strings.Replace(readDraft(t, layout), generated,
			"The reviewer's own wording: the error from Decode never reaches the caller.", 1)
		require.NoError(t, os.WriteFile(
			layout.RoundFile(draftOwner, draftRepo, draftPRNum, draftRound, state.FileDraft),
			[]byte(edited), 0o600))

		_, err = runDraft(t, draftPR, "--repo", draftSlug)
		require.NoError(t, err)

		var entry string
		require.NoError(t, json.Unmarshal(readRendered(t, layout)["f1"], &entry))
		assert.Equal(t, generated, entry, "run %d: the entry is still the body cr generated", run)
	}
}
