package cli

import (
	"encoding/json"
	"sort"
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
