package brief

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/deligoez/cr/internal/gh"
)

// advance puts one more commit on the branch under review and returns the
// revision it moves to. That is how a moved head reaches §9.3.1 in production:
// a real revision meta.json has never seen, rather than a flag saying the two
// disagree.
func advance(t *testing.T, dir, body, message string) string {
	t.Helper()
	require.NoError(t, os.WriteFile(filepath.Join(dir, "order.go"), []byte(body), 0o600))
	runGit(t, dir, "add", "order.go")
	runGit(t, dir, "commit", "--quiet", "-m", message)
	return runGit(t, dir, "rev-parse", "HEAD")
}

// §9.3.3 over a run of briefs: the index moves exactly once per distinct head.
//
// The two halves of §9.3.3's biconditional each have a way of passing a
// narrower test. A brief that never moved the round would satisfy "a same-head
// brief is idempotent" for the trivial reason that it is inert, and a brief
// that incremented on every run would satisfy "the index moves when the head
// differs" while moving it again on the re-run that follows. Neither survives a
// ladder: the round is read back off disk after each brief, and the repeats are
// where a per-invocation counter shows itself.
//
// The last step returns to the first head. §9.3.3 compares the current head
// against the one meta.json recorded, so a head seen two rounds ago is as
// different as one never seen at all — which is what a revert or a re-pushed
// branch actually does. An implementation that remembered every head instead
// would leave the round at 3 here and nothing else in the ladder would notice.
func TestTheRoundIndexMovesExactlyOncePerDistinctHead(t *testing.T) {
	dir, first, base := repository(t)
	second := advance(t, dir,
		"package shop\n\nfunc Total() int { return subtotal() + shipping() + tax() }\n",
		"tax the order")
	third := advance(t, dir,
		"package shop\n\nfunc Total() int { return subtotal() + shipping() + tax() - discount() }\n",
		"discount the order")
	require.NotEqual(t, first, second)
	require.NotEqual(t, second, third)

	src := sources(t, dir, answering(first, base, oneThread))

	// briefAt runs `cr brief` with GitHub reporting one head, and answers
	// with the round meta.json carries afterwards. The payload is checked
	// against the file rather than trusted: §9.3.3 is a claim about what
	// the command stores, and a Round computed and not written would orient
	// this run and no later one.
	briefAt := func(head string) int {
		t.Helper()
		src.GH = gh.WithRunner(answering(head, base, oneThread))
		assembled, err := Run(src)
		require.NoError(t, err)
		recorded, err := src.Layout.ReadMeta(testOwner, testRepo, testPR)
		require.NoError(t, err)
		assert.Equal(t, assembled.Round, recorded.Round,
			"the round `cr brief` reports is the round it recorded")
		assert.Equal(t, head, recorded.Head,
			"§9.3.3 stores the head the round stands at")
		return recorded.Round
	}

	for _, step := range []struct {
		head string
		want int
		why  string
	}{
		{first, 1, "§9.3.3 numbers rounds from 1"},
		{first, 1, "the head has not moved, so the round is the one already open"},
		{first, 1, "a third brief at the same head moves nothing either"},
		{second, 2, "the head differs from the recorded one, so the index moves"},
		{second, 2, "and having moved, it stays where the new head put it"},
		{third, 3, "the next distinct head opens the next round"},
		{third, 3, "which is idempotent in its turn"},
		{first, 4, "a head recorded two rounds ago differs from the recorded head too"},
	} {
		assert.Equal(t, step.want, briefAt(step.head), step.why)
	}
}
