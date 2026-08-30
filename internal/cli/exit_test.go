package cli

import (
	"errors"
	"fmt"
	"path/filepath"
	"testing"

	"github.com/deligoez/cr/internal/axis"
	"github.com/deligoez/cr/internal/config"
	"github.com/deligoez/cr/internal/finding"
	"github.com/deligoez/cr/internal/gh"
	"github.com/deligoez/cr/internal/git"
	"github.com/deligoez/cr/internal/intent"
	"github.com/deligoez/cr/internal/profile"
	"github.com/deligoez/cr/internal/state"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// §1.5 makes an axis id outside the closed set abort with exit code 3, and the
// abort must survive the wrapping a command adds on the way out. Anything with
// no mapping stays a malformed invocation.
func TestInvalidAxisExitsWithTheFileCode(t *testing.T) {
	invalid := axis.Validate("roles/security.json", "axis", "security")
	assert.Equal(t, ExitFile, exitCodeFor(invalid))
	assert.Equal(t, ExitFile, exitCodeFor(fmt.Errorf("loading roles: %w", invalid)))
	assert.Equal(t, ExitUsage, exitCodeFor(errors.New("unknown flag")))
}

// §2.5 item 3 makes a malformed profile file abort with exit code 3, and the
// abort must survive the wrapping a command adds on the way out.
func TestMalformedProfileExitsWithTheFileCode(t *testing.T) {
	_, err := profile.Parse("profiles/generic.json", []byte(`{"id": "generic"}`))
	require.Error(t, err)
	assert.Equal(t, ExitFile, exitCodeFor(err))
	assert.Equal(t, ExitFile, exitCodeFor(fmt.Errorf("loading profiles: %w", err)))
}

// §2.7 lets no layer supply the confirmation gate, the argued forcing, the
// question label, or the provenance and evidence regions, and a name that would
// address one is a bad configuration rather than a bad invocation, so §11.2
// codes it 3. The code must survive the wrapping a command adds on the way out.
func TestProtectedConfigNameExitsWithTheFileCode(t *testing.T) {
	_, err := config.Resolve(config.Sources{Environ: []string{"CR_POST_AUTO_CONFIRM=1"}})
	require.Error(t, err)
	assert.Equal(t, ExitFile, exitCodeFor(err))
	assert.Equal(t, ExitFile, exitCodeFor(fmt.Errorf("resolving configuration: %w", err)))
}

// §3.1.3 codes a non-zero exit from an external command 3 and surfaces its
// stderr. git is one of the three external tools cr drives, so a git that
// refuses exits the same way a refusing tracker command does, and the code must
// survive the wrapping a command adds on the way out.
func TestAFailedGitCommandExitsWithTheFileCode(t *testing.T) {
	_, err := git.MergeBase(filepath.Join(t.TempDir(), "no-such-checkout"), "main", "feature")
	require.Error(t, err)
	assert.Equal(t, ExitFile, exitCodeFor(err))
	assert.Equal(t, ExitFile, exitCodeFor(fmt.Errorf("ingesting the diff: %w", err)))
}

// gh is the third of those tools, and §3.5's ingestion is the first thing that
// drives it. A GraphQL error arrives as a refusal like any other — gh exits
// non-zero with the message on stderr — so it maps onto the same code, and the
// code must survive the wrapping a command adds on the way out.
func TestAFailedGhCommandExitsWithTheFileCode(t *testing.T) {
	err := error(&gh.CommandError{
		Args:   []string{"api", "graphql"},
		Stderr: "Could not resolve to a Repository with the name 'acme/web'.",
		Err:    errors.New("exit status 1"),
	})
	assert.Equal(t, ExitFile, exitCodeFor(err))
	assert.Equal(t, ExitFile, exitCodeFor(fmt.Errorf("ingesting threads: %w", err)))
}

// The tracker is the command §3.1.3 was written about, and it is the one of the
// three cr does not choose: §3.1.1 makes the program the user's own argv, so a
// refusal can say anything at all. It still exits 3, and the code must survive
// the wrapping a command adds on the way out.
func TestAFailedTrackerCommandExitsWithTheFileCode(t *testing.T) {
	err := error(&intent.CommandError{
		Args:   []string{"jira", "issue", "view", "CR-1", "--plain"},
		Stderr: "ERROR unable to authenticate: 401 Unauthorized",
		Err:    errors.New("exit status 2"),
	})
	assert.Equal(t, ExitFile, exitCodeFor(err))
	assert.Equal(t, ExitFile, exitCodeFor(fmt.Errorf("reading the issue: %w", err)))
}

// §6.1 rejects a class that is not kebab-case. The record's file was read and
// parsed, so nothing about it failed as a file; what is wrong is the agent's
// data inside it, which §11.2 codes 1. The code must survive the wrapping a
// command adds on the way out.
func TestAnInvalidClassExitsWithTheValidationCode(t *testing.T) {
	err := finding.ValidateClass("review-correctness.ndjson", 7, "Missing Test")
	require.Error(t, err)
	assert.Equal(t, ExitValidation, exitCodeFor(err))
	assert.Equal(t, ExitValidation, exitCodeFor(fmt.Errorf("merging findings: %w", err)))
}

// §6.1.3 rejects a record missing a required field, naming a unit the current
// round does not have, or claiming a role other than the one whose §4.6.2
// output file it arrived in. The file was found, read, and parsed, so nothing
// about it failed as a file; the fault is the agent's data inside it, which
// §11.2 codes 1 and not the 3 an unusable file gets. Both the per-record
// rejection and the refusal of a whole input cr merge cannot attribute have to
// survive the wrapping a command adds on the way out.
func TestARejectedRecordExitsWithTheValidationCode(t *testing.T) {
	_, err := finding.Decode(
		finding.FanOutFile("test"),
		[]byte(`{"id":"f1","kind":"finding","role":"test","class":"missing-test",`+
			`"severity":"high","unit":"u1","summary":"The guard has no test.",`+
			`"anchor":{"path":"app/Models/User.php","side":"RIGHT","start_line":12,"line":14}}`),
		[]string{"u1"},
		finding.SourceAgent,
	)
	require.Error(t, err, "the record supplies no evidence")
	assert.Equal(t, ExitValidation, exitCodeFor(err))
	assert.Equal(t, ExitValidation, exitCodeFor(fmt.Errorf("recording findings: %w", err)))

	_, err = finding.DecodePerRole("merged.ndjson", nil, []string{"u1"})
	require.Error(t, err)
	assert.Equal(t, ExitValidation, exitCodeFor(err))
	assert.Equal(t, ExitValidation, exitCodeFor(fmt.Errorf("merging findings: %w", err)))
}

// agentRecord stands in for a record type of one of the eight §2.3.3 files. It
// carries head and round the only way any type can: by embedding state.Stamp.
type agentRecord struct {
	state.Stamp
	ID string `json:"id"`
}

// §2.3.3 has cr stamp head and round on write, so a record supplying either is
// bad input data rather than an unusable file, and §11.2 codes that 1 and not
// the 3 a malformed profile gets. The code must survive the wrapping a command
// adds on the way out.
func TestASuppliedStampFieldExitsWithTheValidationCode(t *testing.T) {
	_, err := state.DecodeStamped[agentRecord](
		state.FileClaims, []byte(`{"id":"c1","round":2}`), nil,
	)
	require.Error(t, err)
	assert.Equal(t, ExitValidation, exitCodeFor(err))
	assert.Equal(t, ExitValidation, exitCodeFor(fmt.Errorf("recording claims: %w", err)))
}

// §1.6.2 caps one round's comments at post.max_comments and blocks posting
// above it with exit code 1, naming the count so the user triages further.
// §11.2 runs that validation before the confirmation gate, so the block lands
// whether or not --confirm was given, and the code must survive the wrapping a
// command adds on the way out.
//
// The cap comes from the resolved configuration rather than from a literal
// here: §2.7 resolves post.max_comments across five layers, and a second path
// to the value would be a second answer to what the default 20 is.
//
// Nothing posts here because nothing can — `cr post` is not built — so the
// block is asserted where it is made, over the round's queued comments.
func TestQueueingPastTheCommentCapExitsWithTheValidationCode(t *testing.T) {
	resolved, err := config.Resolve(config.Sources{})
	require.NoError(t, err)
	maxComments := resolved.Int("post.max_comments")
	require.Equal(t, 20, maxComments, "§1.6.2 gives post.max_comments a default of 20")

	queued := make([]*finding.Finding, 0, maxComments+1)
	for i := 1; i <= maxComments+1; i++ {
		queued = append(queued, &finding.Finding{
			ID:   fmt.Sprintf("f%d", i),
			Kind: finding.KindFinding,
		})
	}

	blocked := finding.CommentCapFor(queued, maxComments).Err()
	require.Error(t, blocked, "twenty-one comments exceed a cap of twenty")
	assert.Equal(t, ExitValidation, exitCodeFor(blocked))
	assert.Equal(t, ExitValidation, exitCodeFor(fmt.Errorf("posting the review: %w", blocked)))
	assert.Contains(t, blocked.Error(), "21 comments", "the block names the count")
	assert.Contains(t, blocked.Error(), "post.max_comments 20", "and the cap it was measured against")

	require.NoError(t, finding.CommentCapFor(queued[:maxComments], maxComments).Err(),
		"the same round posts once the user has triaged one comment away")
}

// §2.4.2 aborts a profile tie with exit code 3 naming the tied profiles. The
// tie is not a malformed profile file — each one in it parsed and validated —
// so it arrives here as its own type and needs its own mapping; without it the
// abort would fall through to the malformed-invocation default and report 2,
// telling the user to fix a command line that was correct. The code must
// survive the wrapping a command adds on the way out.
func TestAProfileTieExitsWithTheFileCode(t *testing.T) {
	err := error(&profile.TieError{Profiles: []string{"laravel-pest", "symfony"}})
	assert.Equal(t, ExitFile, exitCodeFor(err))
	assert.Equal(t, ExitFile, exitCodeFor(fmt.Errorf("selecting a profile: %w", err)))
	assert.Contains(t, err.Error(), "laravel-pest")
	assert.Contains(t, err.Error(), "symfony")
}
