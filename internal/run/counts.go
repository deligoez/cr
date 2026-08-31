package run

import (
	"fmt"
	"regexp"
	"strconv"
	"strings"
)

// countBits bounds one capture at the largest count that fits an int on every
// platform cr builds for, and maxCount bounds the sum of them. A "count"
// beyond that is not a number of tests, it is a pattern reading something
// else, and §5.2.1's answer for a group it cannot read as a count is that the
// counts are undetermined.
const (
	countBits = 31
	maxCount  = 1<<countBits - 1
)

// Counter reads §5.2.1's two test counts out of the runner's output.
//
// It is an io.Writer teed off the same stream Tail is teed off, which is what
// settles round 12's undefined-input-stream finding: the text the patterns are
// matched against is the runner's stdout and stderr merged, in the order the
// runner wrote them. sandbox.Run gives the two the same writer, so os/exec
// hands them one pipe and one copying goroutine, and a profile whose recap
// reaches one stream and whose failures reach the other is read whole. Reading
// a single stream would have left the count depending on which of the two the
// runner chose, which §2.1.1 asks cr not to have.
//
// The whole of that stream is retained rather than Tail's last few kilobytes.
// §5.2.1 sums each pattern over every match, and a sum taken over a truncated
// view is not an undetermined count but a wrong one: a byte-bounded tail can
// cut a recap line in half, and the shipped laravel-pest patterns would then
// read `2 failed, 4 passed` as four tests and no failures — a run §5.2.5 would
// call passed and §5.3.5 would accept as a baseline. Nothing is retained at
// all when no `tests.count_pattern` is configured, because no amount of output
// can determine a count there is no pattern for.
type Counter struct {
	count  *regexp.Regexp
	failed *regexp.Regexp
	held   strings.Builder
}

// NewCounter compiles a profile's `tests.count_pattern` and
// `tests.failed_pattern`.
//
// Both are already held to exactly one capture group by internal/profile, and
// an empty `tests.count_pattern` is §2.4's absent one: the counter then
// retains nothing and reports both counts undetermined, which is what §5.2.1
// means by recording the counts "when `tests.count_pattern` is configured".
//
// A `tests.count_pattern` with no `tests.failed_pattern` beside it is given
// the same answer rather than an error. §2.4 requires the pair together and
// profile.Parse refuses the half-configured file, so no loaded profile reaches
// here that way; undetermined is the reading that cannot invent a clean run
// out of a missing pattern, since a failed count is otherwise zero whenever
// nothing matches.
func NewCounter(countPattern, failedPattern string) (*Counter, error) {
	if countPattern == "" || failedPattern == "" {
		return &Counter{}, nil
	}
	count, err := regexp.Compile(countPattern)
	if err != nil {
		return nil, fmt.Errorf("tests.count_pattern: %w", err)
	}
	failed, err := regexp.Compile(failedPattern)
	if err != nil {
		return nil, fmt.Errorf("tests.failed_pattern: %w", err)
	}
	return &Counter{count: count, failed: failed}, nil
}

// Write retains the output when there is a pattern to match against it, and
// discards it otherwise.
//
// The whole of p is reported as written either way: what the caller hands over
// is accepted, and what is kept is this type's business rather than a short
// write. Writes are not synchronised, for the reason Tail's are not: os/exec
// calls Write from at most one goroutine at a time when a command's Stdout and
// Stderr are the same comparable value.
func (c *Counter) Write(p []byte) (int, error) {
	if c.count != nil {
		c.held.Write(p)
	}
	return len(p), nil
}

// Counts is §5.2.1's pair, nil for a count the output does not determine.
//
// The two travel together on every failure, because §5.2.1 says they do: a
// `tests.count_pattern` that never matches and a group that does not parse
// both leave *both* counts undetermined. The asymmetry is only in the other
// direction — a `tests.failed_pattern` that never matches yields zero, because
// a runner that prints a status only when its count is non-zero prints nothing
// at all for no failures, and an all-passing Pest run never writes the word
// failed.
func (c *Counter) Counts() (executed, failed *int) {
	if c.count == nil {
		return nil, nil
	}
	output := c.held.String()
	ran, matched, ok := sum(c.count, output)
	if !matched || !ok {
		return nil, nil
	}
	broke, _, ok := sum(c.failed, output)
	if !ok {
		return nil, nil
	}
	return &ran, &broke
}

// sum adds up the single capture group of every match of re in output, which
// is §5.2.1's "matched repeatedly … the sum of its matches".
//
// matched says whether re matched at all, which §5.2.1 reads differently for
// the two patterns. ok says whether every capture is a base-10 non-negative
// integer and the total is still a count: strconv.ParseUint permits no sign,
// so a capture reading -1 fails here rather than subtracting from the total,
// and a total that outgrows maxCount is refused rather than wrapping into a
// smaller number than the run really produced.
func sum(re *regexp.Regexp, output string) (total int, matched, ok bool) {
	for _, match := range re.FindAllStringSubmatch(output, -1) {
		matched = true
		n, err := strconv.ParseUint(match[1], 10, countBits)
		if err != nil {
			return 0, matched, false
		}
		if total > maxCount-int(n) {
			return 0, matched, false
		}
		total += int(n)
	}
	return total, matched, true
}
