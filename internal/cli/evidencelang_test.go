package cli

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

// §8.1.7 through `cr draft`: under render.lang `tr` the evidence region's
// field names are Turkish, and the values beneath them are the record's.
//
// Measured with cr 0.15.0 on a real pull request on a private Laravel
// repository: the region read `kind:`, `target:`, `result: no-test-failed` and
// `input:` in an otherwise Turkish comment.
func TestTheDraftsEvidenceRegionSpeaksRenderLang(t *testing.T) {
	t.Setenv("CR_RENDER_LANG", "tr")
	drafted, _ := gradedDraft(t)

	assert.Contains(t, blockOf(t, drafted, "f1"), "<!-- cr:evidence -->\n"+
		"tür: mutation\n"+
		"hedef: app.go:3\n"+
		"sonuç: no-test-failed\n"+
		"girdi:\n```\n--- a/app.go\n+++ b/app.go\n```\n"+
		"test çıktısı:\n```\nTests:  12 passed\n```\n"+
		"<!-- cr:/evidence -->")
	assert.Contains(t, blockOf(t, drafted, "f2"), "<!-- cr:evidence -->\n"+
		"kaynak: app.go:1\n"+
		"<!-- cr:/evidence -->")
}
