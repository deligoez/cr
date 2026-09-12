package finding

import (
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"strconv"
	"strings"
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
	const maxComments = 20

	for _, count := range []int{0, 1, 19, maxComments} {
		require.NoError(t, CommentCapFor(queue(count), maxComments).Err(),
			"%d comments fit under a cap of %d", count, maxComments)
	}

	for _, count := range []int{maxComments + 1, 40} {
		err := CommentCapFor(queue(count), maxComments).Err()
		var exceeded *CommentCapExceededError
		require.ErrorAs(t, err, &exceeded, "%d comments exceed a cap of %d", count, maxComments)
		assert.Equal(t, count, exceeded.Cap.Count, "the count reaches the caller as a number")
		assert.Equal(t, maxComments, exceeded.Cap.Max)
		assert.Contains(t, err.Error(), strconv.Itoa(count), "the block names the count")
	}
}

// §1.6.2's two halves pull in opposite directions and both are the rule: cr
// MUST block posting above the cap, and cr MUST NOT silently drop comments to
// fit. The implementation that satisfies the first while breaking the second is
// the tempting one — trim the queue to the cap and post it — and it reads as a
// helpfulness rather than as the failure the clause names.
//
// So the refusal is structural rather than remembered. Nothing that knows the
// cap hands back records, which leaves no fitted queue for a caller to reach
// for: a truncation has to be written from scratch, at the call site, against a
// type that offers nothing towards it.
//
// The set of cap-aware functions is read out of the package's own source rather
// than listed here, so a convenience added years from now by someone who never
// read §1.6.2 fails this test instead of quietly fitting the round.
func TestNothingCarriesAFittedQueueBackFromTheCap(t *testing.T) {
	aware := capAware(t)
	require.NotEmpty(t, aware,
		"the cap decision lives in this package; finding none of it proves nothing")

	for name, results := range aware {
		assert.False(t, namesARecord(results),
			"%s knows the cap and hands back records: §1.6.2 lets nothing fit the queue", name)
	}
}

// capAware returns the result list of every function of this package whose
// signature names the comment cap, keyed by the name callers use. A receiver
// counts as naming it, so a method that fitted the queue is caught the same way
// a function would be.
func capAware(t *testing.T) map[string]*ast.FieldList {
	t.Helper()
	sources, err := os.ReadDir(".")
	require.NoError(t, err)

	aware := make(map[string]*ast.FieldList)
	for _, source := range sources {
		name := source.Name()
		if !strings.HasSuffix(name, ".go") || strings.HasSuffix(name, "_test.go") {
			continue
		}
		parsed, err := parser.ParseFile(token.NewFileSet(), name, nil, parser.SkipObjectResolution)
		require.NoError(t, err)
		for _, decl := range parsed.Decls {
			fn, ok := decl.(*ast.FuncDecl)
			if !ok || !namesTheCap(fn) {
				continue
			}
			aware[doorName(fn)] = fn.Type.Results
		}
	}
	return aware
}

// namesTheCap reports whether a function's receiver, parameters, or results
// mention the cap by type. Every type of the decision is named for it, so the
// prefix is what identifies them and a fourth one added later is covered
// without being listed.
func namesTheCap(fn *ast.FuncDecl) bool {
	for _, part := range []*ast.FieldList{fn.Recv, fn.Type.Params, fn.Type.Results} {
		if part == nil {
			continue
		}
		found := false
		ast.Inspect(part, func(node ast.Node) bool {
			if named, ok := node.(*ast.Ident); ok && strings.HasPrefix(named.Name, "CommentCap") {
				found = true
			}
			return !found
		})
		if found {
			return true
		}
	}
	return false
}

// §11.1 makes the comment cap of §1.6.2 one of seven disclosures `--quiet` may
// never suppress, and a suppressed cap report is the silent drop §1.6.2 forbids
// reached through an output flag instead of a truncation: the comments the
// round could not fit would go unmentioned either way.
//
// The disclosure is therefore on the decision rather than at a call site, and
// it is one text on both paths — the §7.1.4 draft header at or under the cap,
// the block above it — so the block cannot name a different number from the one
// the user triaged against. Nothing here takes a flag, because there is no
// answer to `--quiet` for this report to give.
func TestTheCapReportIsAnHonestyDisclosureOnEitherPath(t *testing.T) {
	// The shape §11.1's shared writer consumes, so adopting the cap report
	// into it is a call site rather than a rewrite.
	var _ HonestyDisclosure = CommentCap{}

	under := CommentCapFor(queue(12), 20)
	assert.Equal(t, "12 comments queued against post.max_comments 20", under.Disclosure())
	require.NoError(t, under.Err(), "the report is made whether or not the cap was exceeded")

	atTheCap := CommentCapFor(queue(20), 20)
	assert.Equal(t, "20 comments queued against post.max_comments 20", atTheCap.Disclosure(),
		"twenty comments fit under a cap of twenty and nothing is over it")

	over := CommentCapFor(queue(21), 20)
	assert.Equal(t, "21 comments queued against post.max_comments 20, 1 over the cap",
		over.Disclosure())
	assert.Equal(t,
		"21 comments queued against post.max_comments 20, 1 over the cap: "+
			"triage the draft down to 20, or raise post.max_comments; cr will not drop 1 to fit",
		over.Err().Error(), "the block carries the disclosure whole and names the refusal")
}
