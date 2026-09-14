package draft

import (
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/deligoez/cr/internal/finding"
)

// §8.2.3 is asked of the suggestion a comment will carry, and SentSuggestion is
// what says which that is: the stored field while the block is untouched, and
// the fence of the body the reviewer rewrote once it is not.
func TestTheSentSuggestionIsReadFromTheBodyThatWillBePosted(t *testing.T) {
	stored := suggesting(finding.OriginAgent)
	plain := aRecord("f1")
	for _, c := range []struct {
		name      string
		record    *finding.Finding
		preserved map[string]string
		want      string
	}{
		{name: "an untouched block sends the stored suggestion",
			record: stored, preserved: map[string]string{"f2": "```suggestion\n    other\n```"},
			want: "\tif err := dec.Decode(&body); err != nil {"},
		{name: "an edited fence sends the edit",
			record:    stored,
			preserved: map[string]string{"f1": "Reworded.\n\n```suggestion\n    if err := dec.Decode(&body); err != nil {\n    }\n```\n\nAfter."},
			want:      "    if err := dec.Decode(&body); err != nil {\n    }"},
		{name: "only the first fence is the suggestion",
			record:    stored,
			preserved: map[string]string{"f1": "```suggestion\n  first\n```\n\n```suggestion\n\tsecond\n```"},
			want:      "  first"},
		{name: "a fence the reviewer removed sends none",
			record: stored, preserved: map[string]string{"f1": "Reworded, and no block."},
			want: ""},
		{name: "a fence the reviewer added is sent",
			record: plain, preserved: map[string]string{"f1": "Added.\n```suggestion\n\tadded\n```"},
			want: "\tadded"},
		{name: "a fence nothing closes runs to the end of the body",
			record: plain, preserved: map[string]string{"f1": "```suggestion\n  open\n  still"},
			want: "  open\n  still"},
		{name: "a record with no suggestion and an untouched block sends none",
			record: plain, preserved: map[string]string{},
			want: ""},
	} {
		t.Run(c.name, func(t *testing.T) {
			assert.Equal(t, c.want, SentSuggestion(c.record, c.preserved))
		})
	}
}
