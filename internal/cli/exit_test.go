package cli

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"testing"

	"github.com/deligoez/cr/internal/axis"
	"github.com/deligoez/cr/internal/config"
	"github.com/deligoez/cr/internal/finding"
	"github.com/deligoez/cr/internal/gh"
	"github.com/deligoez/cr/internal/git"
	"github.com/deligoez/cr/internal/intent"
	"github.com/deligoez/cr/internal/note"
	"github.com/deligoez/cr/internal/profile"
	"github.com/deligoez/cr/internal/role"
	"github.com/deligoez/cr/internal/state"
	"github.com/deligoez/cr/internal/text"
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

// §2.5 item 3 names the role file beside the profile one, on the same code, and
// the abort must survive the wrapping a command adds on the way out. The file
// here is malformed by carrying a key §2.5's table does not have, which is the
// fault that division of labour exists to catch: a role does not get to say
// where cr writes.
func TestAMalformedRoleExitsWithTheFileCode(t *testing.T) {
	_, err := role.Parse("roles/correctness.json", []byte(`{"output_path": "mine.ndjson"}`))
	require.Error(t, err)
	assert.Equal(t, ExitFile, exitCodeFor(err))
	assert.Equal(t, ExitFile, exitCodeFor(fmt.Errorf("loading roles: %w", err)))
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

// An intent.cmd §3.1.1 does not describe never becomes a command failure,
// because nothing is started, but §11.2 codes it 3 all the same: a bad
// configuration file and a refusing external command share the row. It is not
// a usage error either — no invocation can be corrected into a usable
// intent.cmd — and the code must survive the wrapping a command adds on the
// way out.
func TestAMalformedIntentCmdExitsWithTheFileCode(t *testing.T) {
	_, err := intent.Read(intent.Source{Cmd: []string{"jira", "issue", "view"}}, "CR-1")
	require.Error(t, err)
	assert.Equal(t, ExitFile, exitCodeFor(err))
	assert.Equal(t, ExitFile, exitCodeFor(fmt.Errorf("reading the issue: %w", err)))
}

// §3.1.4's file stands in for the tracker command, so it exits the way the
// command it replaced would have. §11.2 puts the file and the external command
// in one row, and the code must survive the wrapping a command adds on the way
// out.
//
// The error is raised here rather than constructed, because the mapping is
// only worth anything if the type the filesystem failure actually arrives as
// is the type internal/cli matches on.
func TestAnUnreadableIntentFileExitsWithTheFileCode(t *testing.T) {
	_, err := intent.Read(intent.Source{File: filepath.Join(t.TempDir(), "issue.txt")}, "CR-1")
	require.Error(t, err)
	assert.Equal(t, ExitFile, exitCodeFor(err))
	assert.Equal(t, ExitFile, exitCodeFor(fmt.Errorf("reading the issue: %w", err)))
}

// §3.2 leaves an uncompilable intent.key_pattern undefined; cr answers it the
// way §2.6.1.2 answers the same fault in a rule's detect.pattern, and §11.2
// codes that 3. It is a configuration failure rather than a usage one — no
// invocation can be corrected into a compiling pattern — and the code must
// survive the wrapping a command adds on the way out.
//
// The error is raised here rather than constructed, so the mapping is checked
// against the type the resolver actually returns.
func TestAnUncompilableKeyPatternExitsWithTheFileCode(t *testing.T) {
	_, err := intent.ResolveKey(intent.KeySources{Branch: "feature/CR-1-x"}, `[A-Z`)
	require.Error(t, err)
	assert.Equal(t, ExitFile, exitCodeFor(err))
	assert.Equal(t, ExitFile, exitCodeFor(fmt.Errorf("resolving the issue key: %w", err)))
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

// §3.6.6 retracts a note by id, and an id naming no note is not a mistyped
// command line: it is spelled the way §3.6.1 spells one, and the store was read
// and parsed without trouble. What failed is the retraction, which §11.2 codes
// 1 alongside a pull request that resolved to no issue key.
func TestAnUnknownNoteExitsWithTheValidationCode(t *testing.T) {
	unknown := &note.UnknownNoteError{ID: "CR-1#n9", IssueKey: "CR-1"}
	assert.Equal(t, ExitValidation, exitCodeFor(unknown))
	assert.Equal(t, ExitValidation, exitCodeFor(fmt.Errorf("retracting: %w", unknown)))
	assert.Contains(t, unknown.Error(), "cr context CR-1", "§12.4: the error names the next step")
}

// §9.1 makes its transition table exhaustive and codes everything outside it 4,
// naming the record and its current state. It is the only §11.2 code no error
// reached until now, and without this mapping every illegal transition would
// have reported 2 — a malformed invocation — telling the user to retype a
// command line that was correct about a record that was well-formed.
//
// The sample is chosen for the ways the table can be misread rather than for
// coverage; the exhaustive half is
// TestEveryTransitionIsTheTableAndNothingElse, over the whole cross product.
// Two of these are the reason §9.1's third column is part of the key: the
// states are a listed pair and only the actor is wrong, so a decision that read
// (from, to) alone would allow both.
func TestAnUnlistedTransitionExitsWithTheStateCode(t *testing.T) {
	for name, refused := range map[string]error{
		"a posted record asked back into the queue": finding.MayTransition(
			"f1", finding.Existing(finding.StatePosted), finding.StateQueued, finding.ActorDraft),
		"a queued record asked back into draft": finding.MayTransition(
			"f2", finding.Existing(finding.StateQueued), finding.StateDraft, finding.ActorRecord),
		"a reconcile discarding rather than adopting": finding.MayTransition(
			"f3", finding.Existing(finding.StateQueued), finding.StateDiscarded, finding.ActorPostReconcile),
		"a draft posted without being queued": finding.MayTransition(
			"f4", finding.Existing(finding.StateDraft), finding.StatePosted, finding.ActorPostConfirm),
		"a record created by anything but cr record": finding.MayTransition(
			"f5", finding.Creation, finding.StateDraft, finding.ActorBrief),
	} {
		t.Run(name, func(t *testing.T) {
			require.Error(t, refused)
			assert.Equal(t, ExitState, exitCodeFor(refused))
			assert.Equal(t, ExitState, exitCodeFor(fmt.Errorf("posting the review: %w", refused)))
		})
	}

	require.NoError(t,
		finding.MayTransition("f3", finding.Existing(finding.StateQueued), finding.StatePosted,
			finding.ActorPostReconcile),
		"the same actor and the same state post on adopt, which is the row §9.1 does list")
}

// Found by context-command: a §3.6 store cr read and could not parse exited 2,
// a malformed invocation, when nothing about the invocation was wrong and no
// retyping of it could help. §11.2 codes a file failure 3.
//
// All three commands that reach the store are exercised, because the fault was
// shared by all three and a mapping proven through one of them says nothing
// about the other two. Each is given a store that is on disk, is found, and
// holds a line that is not JSON — which is the state a half-written file or a
// hand-edit leaves behind — and the run is expected to name the file and the
// line, since that is the only thing the user can act on.
func TestACorruptContextStoreExitsWithTheFileCode(t *testing.T) {
	for name, args := range map[string][]string{
		"cr note":    {"note", "CR-7", "hearsay", "--source", "chat", "--pr", "9"},
		"cr answer":  {"answer", answeredPR, "f3", "answered", "--source", "chat", "--repo", answeredSlug},
		"cr context": {"context", "CR-7"},
	} {
		t.Run(name, func(t *testing.T) {
			layout := briefedHome(t, "CR-7")
			require.NoError(t, layout.EnsureContext("CR-7"))
			store := layout.ContextFile("CR-7")
			require.NoError(t, os.WriteFile(store,
				[]byte(`{"id":"CR-7#n1","text":"fine"}`+"\n"+`{"id": not json`+"\n"), 0o600))

			out, err := runIn(t, args...)
			require.Error(t, err)
			assert.Equal(t, ExitFile, exitCodeFor(err), "§11.2 codes a file failure 3")
			assert.Contains(t, err.Error(), store, "the error names the file to open")
			assert.Contains(t, err.Error(), "line 2", "and the line to open it at")
			assert.Empty(t, out, "a refused run prints no notes")
		})
	}
}

// Round 8's finding render-lang-domain-unbounded closes the domain of
// render.lang at the two languages §8.1.4 builds a question label in, and a
// value outside them aborts with exit code 3 naming the setting. §11.2 codes it
// 3 rather than 2: the command line is correct and no retyping of it can help,
// and rather than 1: the run never reached input data at all.
//
// It is run through `cr config` as well as mapped, because the abort has to
// survive the whole path a user takes to it — the environment layer of §2.7,
// the resolution, and the wrapping a command adds on the way out — and because
// a configuration cr refuses must print nothing that looks like an answer.
func TestAnUnknownRenderLanguageExitsWithTheFileCode(t *testing.T) {
	_, err := config.Resolve(config.Sources{Environ: []string{"CR_RENDER_LANG=de"}})
	require.Error(t, err)
	assert.Equal(t, ExitFile, exitCodeFor(err))
	assert.Equal(t, ExitFile, exitCodeFor(fmt.Errorf("resolving configuration: %w", err)))

	crHome(t)
	t.Setenv("CR_RENDER_LANG", "de")
	out, err := runIn(t, "config")
	require.Error(t, err)
	assert.Equal(t, ExitFile, exitCodeFor(err), "§11.2 codes a configuration failure 3")
	assert.Contains(t, err.Error(), "render.lang", "the abort names the setting")
	assert.Contains(t, err.Error(), `"de"`, "and the value it rejected")
	assert.Empty(t, out, "a configuration cr refuses prints no configuration")
}

// §1.4 step 1 fixes the code in the spec text itself: input that does not
// decode as UTF-8 fails with exit code 1. It is the first content fault in the
// tree that is not a field of a parsed record — every other ExitValidation
// above is the agent's data inside a file that read and parsed cleanly, and
// this one is the bytes of the text. §11.2 still codes it 1 rather than the 3
// an unusable file gets, because the file was found and read whole; what
// cannot be used is what it says. Without this mapping every undecodable issue
// body, diff hunk and comment would have reported 2 — a malformed invocation —
// telling the user to retype a command line that was correct.
//
// The error is raised rather than constructed, so the mapping is checked
// against the type the transform actually returns, and the input is built with
// string([]byte{…}) because that conversion copies bytes and replaces nothing.
func TestTextThatDoesNotDecodeExitsWithTheValidationCode(t *testing.T) {
	_, err := text.Normalise("a" + string([]byte{0xFF}) + "b")
	require.Error(t, err)
	assert.Equal(t, ExitValidation, exitCodeFor(err))
	assert.Equal(t, ExitValidation, exitCodeFor(fmt.Errorf("hashing the claim span: %w", err)))
	assert.Contains(t, err.Error(), "byte 1", "the refusal names where to look")
}

// §3.3.1 rejects a claim with exit code 1. The file was found, read, and
// parsed, so nothing about it failed as a file; what is wrong is the agent's
// data inside it, exactly as it is for a rejected record. Without these
// mappings a claims file with one missing row or one mistyped source would
// report 2 — a malformed invocation — telling the user to retype a command line
// that was correct.
//
// Both errors are raised rather than constructed, so each mapping is checked
// against the type the door actually returns: the required-row rejection comes
// back from the checker, and the closed `source` vocabulary refuses its value
// while the line is still being decoded.
func TestARejectedClaimExitsWithTheValidationCode(t *testing.T) {
	_, err := intent.DecodeClaims(
		state.FileClaims,
		[]byte(`{"id":"CR-1#c1","text":"An expired token is rejected.","source":"acceptance"}`),
		"CR-1", intent.SpanTexts{},
	)
	require.Error(t, err, "the claim supplies no span")
	assert.Equal(t, ExitValidation, exitCodeFor(err))
	assert.Equal(t, ExitValidation, exitCodeFor(fmt.Errorf("recording claims: %w", err)))

	_, err = intent.DecodeClaims(
		state.FileClaims,
		[]byte(`{"id":"CR-1#c1","text":"t","source":"spec","span":"s"}`),
		"CR-1", intent.SpanTexts{},
	)
	require.Error(t, err, "§3.3 closes the source row at four values")
	assert.Equal(t, ExitValidation, exitCodeFor(err))
	assert.Equal(t, ExitValidation, exitCodeFor(fmt.Errorf("recording claims: %w", err)))
	assert.Contains(t, err.Error(), "description, acceptance, comment, note",
		"§12.4: the refusal names what the user may choose from")
}
