package post

import (
	"encoding/json"
	"go/ast"
	"go/parser"
	"go/token"
	"io/fs"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/deligoez/cr/internal/finding"
	"github.com/deligoez/cr/internal/gh"
	"github.com/deligoez/cr/internal/git"
)

// recorder is a Sender that writes nothing and remembers every invocation it
// was handed, and the body each carried, so a test can count the calls one
// round makes.
type recorder struct {
	calls  [][]string
	bodies [][]byte
}

func (r *recorder) Write(body []byte, args ...string) (string, error) {
	r.calls = append(r.calls, args)
	r.bodies = append(r.bodies, body)
	return `{"id":991}`, nil
}

// queued is one record of a round, anchored on its own lines so a payload can
// be read back comment by comment.
func queued(id, path string, start, end int) *finding.Finding {
	return &finding.Finding{
		ID: id, Kind: finding.KindFinding, Role: "correctness", Class: "dropped-error",
		Severity: finding.SeverityMedium, Unit: "u1",
		Anchor: finding.Anchor{
			Path: path, Side: git.Right, StartLine: start, Line: end,
		},
		Summary: "The added line ignores the error.", Evidence: "The second result is dropped.",
	}
}

// aRound is three queued records with the bodies §8.1.2 produced for them.
func aRound() (records []*finding.Finding, bodies map[string]string) {
	return []*finding.Finding{
			queued("f1", "app/Models/Order.php", 11, 11),
			queued("f2", "app/Models/Order.php", 40, 42),
			queued("f3", "app/Services/Ledger.php", 7, 7),
		}, map[string]string{
			"f1": "The error is dropped.",
			"f2": "Is the rounding deliberate?",
			"f3": "The ledger is written twice.",
		}
}

// §8.3.1: a round's comments are posted as one review, so the author is
// notified once.
//
// The count is the assertion and the comment count is the control: three
// comments make one call, so the call count is a property of the round rather
// than of how many findings it happened to hold. A run that posted per comment
// would notify the author once per finding, which §1.6's economy pays for in
// the same trust a wrong comment does.
func TestAMultiCommentRoundIsOneReviewCreationCall(t *testing.T) {
	records, bodies := aRound()
	review := Build(records, bodies)
	require.Len(t, review.Comments, 3)
	sender := &recorder{}

	payload, err := review.Payload()
	require.NoError(t, err)

	out, err := Create(sender, "acme", "web", 7, payload)

	require.NoError(t, err)
	assert.Equal(t, `{"id":991}`, out)
	require.Len(t, sender.calls, 1, "§8.3.1: three comments are one review, and one notification")
	assert.Equal(t, []string{
		"api", "repos/acme/web/pulls/7/reviews",
		"--method", "POST",
		"--input", "-",
	}, sender.calls[0])
	assert.Equal(t, [][]byte{payload}, sender.bodies, "the payload is the call's whole body")
}

// The review a created call names is its `node_id`, the id GraphQL gives each
// of its comments' `pullRequestReview`, and an answer that names none — or
// cannot be read — names no review at all.
func TestTheCreatedReviewIsTheNodeIDTheCallAnswers(t *testing.T) {
	assert.Equal(t, "PRR_kwDOAbCd",
		CreatedReview(`{"id":991,"node_id":"PRR_kwDOAbCd","state":"COMMENTED"}`))
	assert.Empty(t, CreatedReview(`{"id":991}`))
	assert.Empty(t, CreatedReview(`not json`))
}

// The payload carries every queued record as its own comment, and GitHub's own
// distinction between a one-line and a multi-line comment: `start_line` is
// present for the range and absent for the single line.
func TestThePayloadCarriesEveryCommentInTheFieldsGitHubNames(t *testing.T) {
	records, bodies := aRound()
	review := Build(records, bodies)

	payload, err := review.Payload()
	require.NoError(t, err)

	var document struct {
		Event    string `json:"event"`
		Comments []struct {
			Path      string `json:"path"`
			StartLine int    `json:"start_line"`
			StartSide string `json:"start_side"`
			Line      int    `json:"line"`
			Side      string `json:"side"`
			Body      string `json:"body"`
		} `json:"comments"`
	}
	require.NoError(t, json.Unmarshal(payload, &document))

	assert.Equal(t, "COMMENT", document.Event, "§8.3.2: the event is COMMENT")
	require.Len(t, document.Comments, 3)
	assert.Equal(t, "app/Models/Order.php", document.Comments[0].Path)
	assert.Equal(t, 11, document.Comments[0].Line)
	assert.Equal(t, "RIGHT", document.Comments[0].Side)
	assert.Equal(t, "The error is dropped.", document.Comments[0].Body)
	assert.Zero(t, document.Comments[0].StartLine, "a one-line comment carries no start_line")
	assert.Equal(t, 40, document.Comments[1].StartLine, "a range carries the line it starts on")
	assert.Equal(t, "RIGHT", document.Comments[1].StartSide)
	assert.Equal(t, 42, document.Comments[1].Line)
	assert.Equal(t, "app/Services/Ledger.php", document.Comments[2].Path)
	assert.NotContains(t, string(payload), `"record"`, "the record id is cr's, not GitHub's")
}

// start_side travels with start_line and never without it, which is what
// Comment's field says of it. A one-line comment carries neither, whether its
// anchor names the one line twice or leaves the start at zero. The second is an
// anchor §9.2's validation refuses before a record exists, and it is the one
// input on which a start_side could be written with no start_line beside it;
// only a range carries both.
func TestStartSideTravelsWithStartLineAlone(t *testing.T) {
	review := Build([]*finding.Finding{
		queued("f1", "app/Models/Order.php", 11, 11),
		queued("f2", "app/Models/Order.php", 0, 12),
		queued("f3", "app/Models/Order.php", 40, 42),
	}, nil)

	require.Len(t, review.Comments, 3)
	for _, comment := range review.Comments[:2] {
		assert.Zero(t, comment.StartLine, comment.Record)
		assert.Emptyf(t, comment.StartSide, "%s: a one-line comment carries no start_side", comment.Record)
	}
	assert.Equal(t, 40, review.Comments[2].StartLine)
	assert.Equal(t, git.Right, review.Comments[2].StartSide)
}

// §8.3.2: no other event value is reachable through this package.
//
// There is no parameter and no constructor that takes one, so the only way to
// try is to write the field directly — and Payload writes the constant over it
// on the way out. The value a caller set therefore reaches no document, which
// is stronger than refusing it: there is no arrangement of these calls that
// sends APPROVE or REQUEST_CHANGES.
func TestAnEventWrittenOntoTheReviewDoesNotReachThePayload(t *testing.T) {
	for _, verdict := range []string{"APPROVE", "REQUEST_CHANGES"} {
		review := Build(nil, nil)
		review.Event = verdict

		payload, err := review.Payload()

		require.NoError(t, err)
		assert.Contains(t, string(payload), `"event": "COMMENT"`)
		assert.NotContains(t, string(payload), verdict)
	}
}

// moduleRoot is the repository root, found from this file rather than from the
// working directory, so the scan below covers what cr ships and not whatever
// the test happened to be run from.
func moduleRoot(t *testing.T) string {
	t.Helper()
	_, file, _, ok := runtime.Caller(0)
	require.True(t, ok, "the compiler kept no path for this file, so the scan has no root")
	return filepath.Dir(filepath.Dir(filepath.Dir(file)))
}

// No string cr ships holds either verdict event.
//
// The claim §8.3.2 makes is about a value reaching GitHub, and a value can
// arrive by any route — a constant, a struct field, a configuration key read at
// run time. So the guard reads string literals rather than types: a verdict cr
// never spells is a verdict cr cannot send, whatever later code does with the
// fields. Comments are not literals and are left alone, which is what lets the
// package above name the two events it refuses to emit; tests are excluded for
// the same reason, this one included.
func TestNoStringCrShipsHoldsAVerdictEvent(t *testing.T) {
	root := moduleRoot(t)

	scanned := 0
	found := make([]string, 0)
	require.NoError(t, filepath.WalkDir(root, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			if path != root && strings.HasPrefix(d.Name(), ".") {
				return fs.SkipDir
			}
			return nil
		}
		if filepath.Ext(path) != ".go" || strings.HasSuffix(path, "_test.go") {
			return nil
		}
		rel, err := filepath.Rel(root, path)
		require.NoError(t, err)
		scanned++
		found = append(found, verdictLiterals(t, path, rel)...)
		return nil
	}))

	require.Greater(t, scanned, 10, "only %d files were scanned, so this guard proved nothing", scanned)
	assert.Empty(t, found, "§8.3.2: cr emits COMMENT and has no other event to emit")
}

// verdictLiterals names every string literal in one file holding a verdict
// event.
func verdictLiterals(t *testing.T, path, rel string) []string {
	t.Helper()
	parsed, err := parser.ParseFile(token.NewFileSet(), path, nil, 0)
	require.NoError(t, err)

	found := make([]string, 0)
	ast.Inspect(parsed, func(node ast.Node) bool {
		literal, isLiteral := node.(*ast.BasicLit)
		if !isLiteral || literal.Kind != token.STRING {
			return true
		}
		for _, verdict := range []string{"APPROVE", "REQUEST_CHANGES"} {
			if strings.Contains(literal.Value, verdict) {
				found = append(found, rel+" holds the string "+verdict)
			}
		}
		return true
	})
	return found
}

// The Sender the gate will hand Create is a gh.Confirmation, and an
// unconfirmed one writes nothing.
//
// It is asserted here rather than trusted, because the interface is what keeps
// this package free of the mint: a Sender is satisfiable by any type, so the
// thing worth pinning is that the real one refuses without §8.5's flag. The
// zero value is the only Confirmation any package outside internal/gh can
// build.
func TestTheRealSenderRefusesWithoutTheGate(t *testing.T) {
	var sender Sender = gh.Confirmation{}

	out, err := Create(sender, "acme", "web", 7, []byte("{}\n"))

	var refused *gh.WriteRefusedError
	require.ErrorAs(t, err, &refused)
	assert.Empty(t, out)
}
