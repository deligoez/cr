package cli

import (
	"errors"

	"github.com/deligoez/cr/internal/axis"
	"github.com/deligoez/cr/internal/config"
	"github.com/deligoez/cr/internal/coverage"
	"github.com/deligoez/cr/internal/finding"
	"github.com/deligoez/cr/internal/gh"
	"github.com/deligoez/cr/internal/git"
	"github.com/deligoez/cr/internal/intent"
	"github.com/deligoez/cr/internal/mapping"
	"github.com/deligoez/cr/internal/note"
	"github.com/deligoez/cr/internal/profile"
	"github.com/deligoez/cr/internal/render"
	"github.com/deligoez/cr/internal/role"
	"github.com/deligoez/cr/internal/sandbox"
	"github.com/deligoez/cr/internal/state"
	"github.com/deligoez/cr/internal/testadequacy"
	"github.com/deligoez/cr/internal/text"
)

// Exit codes, fixed by spec/0.1.0.md §11.2. Never renumber these.
const (
	// ExitOK signals success.
	ExitOK = 0
	// ExitValidation signals invalid input data.
	ExitValidation = 1
	// ExitUsage signals a malformed invocation.
	ExitUsage = 2
	// ExitFile signals a file, configuration, or external command failure.
	ExitFile = 3
	// ExitState signals a state conflict, including lock timeout and partial post.
	ExitState = 4
)

// exitCodeFor maps an error onto the code §11.2 gives its cause. The mapping
// lives here rather than in the package that raises the error, so a validator
// below cli never has to name an exit code itself. An unmapped cause is a
// malformed invocation.
func exitCodeFor(err error) int {
	var invalidAxis *axis.InvalidError
	if errors.As(err, &invalidAxis) {
		// §1.5: an axis field outside the closed set is a bad
		// configuration file, which §11.2 codes as ExitFile.
		return ExitFile
	}
	var malformedProfile *profile.MalformedError
	if errors.As(err, &malformedProfile) {
		// §2.5 item 3: a malformed profile file aborts with exit
		// code 3, naming the file and the offending field.
		return ExitFile
	}
	var unavailableField *profile.UnavailableError
	if errors.As(err, &unavailableField) {
		// A §2.4 field the command needs and the profile does not set —
		// `tests.cmd` for §5.2.1's run, `tests.filter_flag` for its
		// `--filter` — or §2.4.4's no profile at all. Nothing is
		// malformed and the invocation is right; what refuses is the
		// configuration, which §11.2 codes 3 alongside the malformed
		// file above.
		return ExitFile
	}
	var runnerFailed *sandbox.RunError
	if errors.As(err, &runnerFailed) {
		// §5.2.1 runs the command the profile names, and cr has never
		// heard of it. A runner that could not be started is the
		// external command failure §3.1.3 fixes the shape of — and not a
		// test result: a suite that ran and failed returns its exit code
		// as a value, because §5.3.4's ladder is built on reading one.
		return ExitFile
	}
	var profileTie *profile.TieError
	if errors.As(err, &profileTie) {
		// §2.4.2: profiles matching the same number of marker files
		// abort with exit code 3 naming them, rather than one of them
		// being picked. It is not a malformed file — each tied profile
		// is valid on its own — but §2.4.2 fixes the same code.
		return ExitFile
	}
	var malformedRole *role.MalformedError
	if errors.As(err, &malformedRole) {
		// §2.5 item 3 names the role file alongside the profile one, on
		// the same code. It covers a key outside §2.5's table too: a
		// role file cr cannot read as written is unusable whether the
		// fault is a missing field or an invented one, and the invented
		// one is the case §2.5's division of labour turns on.
		return ExitFile
	}
	var protectedName *config.ProtectedError
	if errors.As(err, &protectedName) {
		// §2.7: a CR_ variable or config key addressing a protected
		// decision is a configuration failure, which §11.2 codes as
		// ExitFile. It is not a validation failure: the run never
		// reached input data, and it is not a usage error, because
		// nothing about the invocation can be corrected.
		return ExitFile
	}
	var unknownLang *render.UnknownLangError
	if errors.As(err, &unknownLang) {
		// §8.1.1 takes the language of every author-facing body from
		// render.lang, and §8.1.4 builds the question label in per
		// language. A value outside the two v0.1 enumerates has no
		// built-in label, so §6.3's forcing would reach the reader
		// through nothing at all. It is a configuration failure, which
		// §11.2 codes 3 — the run never reached input data, and nothing
		// about the invocation can be corrected.
		return ExitFile
	}
	var gitCommand *git.CommandError
	if errors.As(err, &gitCommand) {
		// §3.1.3 codes a non-zero exit from an external command 3 and
		// surfaces its stderr. It fixes that for the tracker command,
		// and git is one of the same three external tools, so it fails
		// through the same mapping rather than a second one.
		return ExitFile
	}
	var ghCommand *gh.CommandError
	if errors.As(err, &ghCommand) {
		// gh is the third of the external tools §3.1.3 governs, and a
		// GraphQL error reaches cr the same way a refusal does: gh
		// exits non-zero with the message on stderr. Both are code 3.
		return ExitFile
	}
	var trackerCommand *intent.CommandError
	if errors.As(err, &trackerCommand) {
		// §3.1.3 is written about this command in particular: a
		// non-zero exit fails with exit code 3 and surfaces the
		// command's stderr. git and gh borrow the clause; the tracker
		// is what it was written for.
		return ExitFile
	}
	var malformedTracker *intent.MalformedCommandError
	if errors.As(err, &malformedTracker) {
		// An intent.cmd §3.1.1 does not describe is a configuration
		// failure, which §11.2 codes 3 alongside the command failure
		// it would otherwise have become. Nothing about the invocation
		// can be corrected, so it is not a usage error.
		return ExitFile
	}
	var unreadableIntentFile *intent.FileError
	if errors.As(err, &unreadableIntentFile) {
		// §3.1.4's file stands in for the tracker command, so a file cr
		// cannot read fails the way the command it replaced would have:
		// §11.2's code 3 covers the file and the external command in one
		// row, and the run reaches no issue text either way.
		return ExitFile
	}
	var badKeyPattern *intent.KeyPatternError
	if errors.As(err, &badKeyPattern) {
		// §3.2 makes intent.key_pattern overridable without saying what
		// an uncompilable override does. §2.6.1.2 already fixed it for
		// cr's other configured regex — abort with exit code 3, naming
		// what it came from — and a profile's count pattern follows the
		// same rule, so the third configured expression does too.
		return ExitFile
	}
	var noIntentKey *intent.NoIssueKeyError
	if errors.As(err, &noIntentKey) {
		// §3.2 leaves the key empty when none of its four sources
		// yields one and has the run continue, so this is recorded
		// state rather than an unusable file or a mistyped command
		// line. What fails is the claim recording itself: §3.3 forms
		// every claim id out of the key, which §11.2 codes 1 alongside
		// note.NoIssueKeyError, the same fault reached from §3.6.2.
		return ExitValidation
	}
	var rejectedClaim *intent.RejectedClaimError
	if errors.As(err, &rejectedClaim) {
		// §3.3.1 rejects a claim with exit code 1. The file was found,
		// read, and parsed, so nothing about it failed as a file; what
		// is wrong is the agent's data inside it, exactly as it is for
		// the record rejection below.
		return ExitValidation
	}
	var unknownClaimSource *intent.UnknownClaimSourceError
	if errors.As(err, &unknownClaimSource) {
		// §3.3 closes the `source` row at four values, and a claim
		// naming a fifth is refused while its line is being decoded
		// rather than after. It is the same file and the same fault as
		// the rejection above, so it takes the same code; without this
		// branch a claim file with one mistyped source would report 2,
		// telling the user to retype a correct command line.
		return ExitValidation
	}
	var invalidClass *finding.InvalidClassError
	if errors.As(err, &invalidClass) {
		// §6.1 rejects a class that is not kebab-case. Like a supplied
		// computed field, the fault is the agent's data inside a file
		// that was read and parsed without trouble, which §11.2 codes 1
		// rather than the 3 an unusable file gets.
		return ExitValidation
	}
	var rejectedRecord *finding.RejectedRecordError
	if errors.As(err, &rejectedRecord) {
		// §6.1.3 rejects a record missing a required field, naming one
		// unit no round has, or claiming a role other than the one whose
		// output file it arrived in, with exit code 1. The file read and
		// parsed; the fault is the agent's data inside it.
		return ExitValidation
	}
	var unattributable *finding.UnattributableFileError
	if errors.As(err, &unattributable) {
		// The same clause, one level up: an input cr merge cannot bind
		// to a role leaves every role field in it unchecked. It is the
		// file's contents that are unusable rather than the file, so it
		// codes 1 with the rest of §6.1.3 rather than 3.
		return ExitValidation
	}
	var capExceeded *finding.CommentCapExceededError
	if errors.As(err, &capExceeded) {
		// §1.6.2 blocks posting above post.max_comments with exit code
		// 1, naming the count. Every record in the round may be
		// well-formed, so what fails is the payload as a whole, and
		// §11.2 runs that validation before the confirmation gate:
		// the block lands whether or not --confirm was given.
		return ExitValidation
	}
	var rejectedPair *mapping.RejectedPairError
	if errors.As(err, &rejectedPair) {
		// §4.1.6 rejects a pair naming an unknown claim or unit id with
		// exit code 1. The file was found, read, and parsed, so nothing
		// about it failed as a file; what is wrong is the agent's
		// judgement inside it, exactly as it is for the cell rejection
		// below.
		return ExitValidation
	}
	var invalidClassification *testadequacy.InvalidClassificationError
	if errors.As(err, &invalidClassification) {
		// §4.4.1 closes the classification at three words, and
		// testadequacy.Coverage refuses a fourth while the cell is being
		// decoded. It is the same file and the same fault as the cell
		// rejection below, so it takes the same code; without this
		// branch a cells file with one mistyped classification would
		// report 2, telling the user to retype a correct command line.
		return ExitValidation
	}
	var rejectedCell *coverage.RejectedCellError
	if errors.As(err, &rejectedCell) {
		// §4.5.6 rejects a cell naming an unknown unit id or an
		// inactive role with exit code 1, and §4.5.5's own field rules
		// fail through the same type. The file was found, read, and
		// parsed, so nothing about it failed as a file; what is wrong
		// is the agent's data inside it, exactly as it is for the claim
		// and record rejections above.
		return ExitValidation
	}
	var illegalTransition *finding.IllegalTransitionError
	if errors.As(err, &illegalTransition) {
		// §9.1: a transition its table does not list MUST be rejected
		// with exit code 4, naming the record and its current state. It
		// is the first thing mapped onto ExitState, and it is a state
		// conflict rather than bad input: the file parsed, the record
		// is well-formed, and the command line is right. What refuses
		// is where the record already stands, which no retyping and no
		// edit to the input can change.
		return ExitState
	}
	var notBriefed *state.NotBriefedError
	if errors.As(err, &notBriefed) {
		// §3.7 makes `cr brief` the writer of meta.json,
		// units.ndjson and threads.ndjson, and §4.1.6, §4.5.6,
		// §9.3.1 and §3.5.3 all read them as authoritative. A
		// command that found no round is therefore not looking at a
		// file it could be pointed at differently: the command line
		// is right and every file it named was read. What refuses is
		// where the pull request stands, which §11.2 codes 4
		// alongside the illegal transition above — and the refusal
		// names `cr brief`, because recomputing the units instead
		// would answer §4.1.6 against a set no round recorded.
		return ExitState
	}
	var setupFailed *sandbox.SetupError
	if errors.As(err, &setupFailed) {
		// §5.1.3 runs the commands the profile names, and cr has never
		// heard of any of them. A refusal is therefore the external
		// command failure §3.1.3 fixes the shape of — exit code 3 with
		// the command's stderr surfaced — and not a malformed
		// invocation: the command line was right and what failed was
		// the tool the profile asked for.
		return ExitFile
	}
	var sandboxExists *sandbox.ExistsError
	if errors.As(err, &sandboxExists) {
		// §5.1.1 creates the sandbox worktree and §5.1.5 removes it,
		// and a sandbox that is already there is neither a file cr
		// could be pointed at differently nor input it could refuse:
		// the command line is right, the pull request is briefed, and
		// what refuses is that a checkout cr did not just make is
		// standing in the one place §5.1.1 puts one. §11.2 codes that 4
		// alongside the illegal transition and the unbriefed round.
		return ExitState
	}
	var probeLocked *state.ProbeLockedError
	if errors.As(err, &probeLocked) {
		// §5.6.2 codes the lock timeout itself: cr waits up to
		// `probe.lock_timeout_seconds` and then fails with exit code 4,
		// which §11.2's table names in as many words — "state conflict,
		// including lock timeout". Nothing about the invocation is
		// wrong and no file failed; what refuses is that another cr run
		// is already inside the same repository and profile.
		return ExitState
	}
	var noPRState *note.NoStateError
	if errors.As(err, &noPRState) {
		// §2.2's state directory is opened by `cr brief`, and §3.6.2's
		// answer reads the issue key out of it. A pull request with no
		// state is a file cr expected and did not find, which §11.2
		// codes 3 alongside its other file failures.
		return ExitFile
	}
	var noIssueKey *note.NoIssueKeyError
	if errors.As(err, &noIssueKey) {
		// §3.2 leaves the issue key empty when none of its four sources
		// yields one, and has the run continue, so this is recorded
		// state rather than an unusable file or a mistyped command
		// line. What fails is the answer itself: §3.6.2 has no store to
		// keep it in, which §11.2 codes 1.
		return ExitValidation
	}
	var unknownNote *note.UnknownNoteError
	if errors.As(err, &unknownNote) {
		// §3.6.6 retracts a note by id. An id naming no note is spelled
		// the way §3.6.1 spells one, and the store read and parsed
		// without trouble, so nothing about the invocation or the file
		// is wrong; what fails is the retraction, which §11.2 codes 1
		// alongside note.NoIssueKeyError.
		return ExitValidation
	}
	var contextStore *state.ContextStoreError
	if errors.As(err, &contextStore) {
		// §3.6's store is a file cr found and could not use, which
		// §11.2 codes 3 with its other file failures. Without this
		// branch `cr note`, `cr answer` and `cr context` all reported a
		// corrupt store as a malformed invocation, and no retyping of
		// the command could ever have fixed it.
		return ExitFile
	}
	var invalidUTF8 *text.InvalidUTF8Error
	if errors.As(err, &invalidUTF8) {
		// §1.4 step 1 fixes the code itself: text that does not decode
		// as UTF-8 fails with exit code 1. It is the first content
		// fault in the tree that is not a field of a parsed record —
		// the file was found and read whole, and what is unusable is
		// the bytes in it, which §11.2 codes 1 rather than the 3 a
		// file cr cannot use as a file gets.
		return ExitValidation
	}
	var reservedField *state.ReservedFieldError
	if errors.As(err, &reservedField) {
		// §6.1.4: a record supplying a field cr writes itself is
		// rejected with exit code 1. The file was found, read, and
		// parsed, so nothing about it failed as a file; what is wrong
		// is the input data inside it, which §11.2 codes 1 rather than
		// the 3 a malformed profile gets.
		return ExitValidation
	}
	return ExitUsage
}
