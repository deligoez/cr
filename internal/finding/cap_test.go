package finding

import (
	"strconv"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// queue is a round's queued records, distinct only in id, because §1.6.2
// counts comments and reads nothing else off them.
func queue(count int) []*Finding {
	queued := make([]*Finding, 0, count)
	for i := 1; i <= count; i++ {
		queued = append(queued, &Finding{ID: idPrefix + strconv.Itoa(i), Kind: KindFinding})
	}
	return queued
}

// §1.6.2 caps one round's comments at post.max_comments and blocks posting
// above it, naming the count so the user can triage further. The cap is the
// last count that fits, so the question the test asks at each of nineteen,
// twenty and twenty-one is which side of the comparison the count falls on: a
// cap that blocked at twenty would refuse a round §1.6.2 permits, and one that
// let twenty-one through would post the comment the clause exists to stop.
func TestTheCapBlocksOnlyTheCountAboveIt(t *testing.T) {
	const max = 20

	for _, count := range []int{0, 1, 19, max} {
		require.NoError(t, CommentCapFor(queue(count), max).Err(),
			"%d comments fit under a cap of %d", count, max)
	}

	for _, count := range []int{max + 1, 40} {
		err := CommentCapFor(queue(count), max).Err()
		var exceeded *CommentCapExceededError
		require.ErrorAs(t, err, &exceeded, "%d comments exceed a cap of %d", count, max)
		assert.Equal(t, count, exceeded.Cap.Count, "the count reaches the caller as a number")
		assert.Equal(t, max, exceeded.Cap.Max)
		assert.Contains(t, err.Error(), strconv.Itoa(count), "the block names the count")
	}
}
