package cli

import (
	"encoding/json"
	"slices"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/deligoez/cr/internal/finding"
	"github.com/deligoez/cr/internal/post"
)

// §8.3.3 and §6.1's `thread_id` for two comments of one review that say the
// same thing: each record is paired with the thread its own comment became, in
// findings.ndjson and in posted.json alike, through `cr post --confirm` and
// through `cr post --reconcile`, which share the read-back.
//
// GitHub is not asked to list the threads in the order the comments were sent,
// so each path runs twice: once with the threads in payload order and once
// reversed. The records sit in different files, so the thread each comment
// opened is the one hanging where that comment was posted.
func TestIdenticalBodiesEachGetTheirOwnThread(t *testing.T) {
	for _, send := range []struct {
		name  string
		shim  func(t *testing.T, listed *post.Review)
		reach func(t *testing.T)
	}{
		{
			name: "confirm",
			shim: func(t *testing.T, listed *post.Review) { ghShimming(t, listed) },
			reach: func(t *testing.T) {
				_, err := runPost(t, draftPR, "--repo", draftSlug, "--confirm")
				require.NoError(t, err)
			},
		},
		{
			name: "reconcile",
			shim: func(t *testing.T, listed *post.Review) { reconcilingShim(t, listed) },
			reach: func(t *testing.T) {
				_, err := runPost(t, draftPR, "--repo", draftSlug, "--confirm")
				require.Error(t, err, "§8.4.4: the send meets a gh that dies without answering")
				_, err = runCLIPrinting(t, "post", draftPR, "--repo", draftSlug, "--reconcile")
				require.NoError(t, err)
			},
		},
	} {
		for _, order := range []struct {
			name     string
			reversed bool
		}{
			{name: "threads in payload order"},
			{name: "threads reversed", reversed: true},
		} {
			t.Run(send.name+" "+order.name, func(t *testing.T) {
				first, second := aCitedRecord("f1"), aCitedRecord("f2")
				second.Anchor.Path = "internal/api/second.go"
				first.Summary = "The error Decode returns is dropped."
				second.Summary = first.Summary
				layout := draftedHome(t, first, second)
				redraft(t)
				payload := builtPayload(t)
				require.Len(t, payload.Comments, 2)
				require.Equal(t, payload.Comments[0].Body, payload.Comments[1].Body,
					"the fixture's two comments carry one body")

				listed := &post.Review{Body: payload.Body, Comments: slices.Clone(payload.Comments)}
				if order.reversed {
					slices.Reverse(listed.Comments)
				}
				// The printed payload carries no record id, so each comment's
				// record is named by the file it was posted on.
				recordOn := map[string]string{first.Anchor.Path: "f1", second.Anchor.Path: "f2"}
				want := make(map[string]string, len(listed.Comments))
				for i := range listed.Comments {
					want[recordOn[listed.Comments[i].Path]] = threadIDFor(i)
				}
				require.Len(t, want, 2)
				require.NotEqual(t, want["f1"], want["f2"])

				send.shim(t, listed)
				send.reach(t)

				stored := roundRecordsByID(t, layout)
				require.Len(t, stored, 2)
				got := make(map[string]string, len(stored))
				for id, record := range stored {
					assert.Equal(t, finding.StatePosted, record.State, id)
					got[id] = record.ThreadID
				}
				assert.Equal(t, want, got,
					"§6.1: each record carries the thread its own comment became")

				var inPosted map[string]string
				require.NoError(t, json.Unmarshal(readPosted(t, layout)[postedThreads], &inPosted))
				assert.Equal(t, want, inPosted,
					"§8.3.3: posted.json keys each thread by its own record")
			})
		}
	}
}
