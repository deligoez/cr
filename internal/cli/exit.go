package cli

import (
	"errors"

	"github.com/deligoez/cr/internal/axis"
	"github.com/deligoez/cr/internal/config"
	"github.com/deligoez/cr/internal/finding"
	"github.com/deligoez/cr/internal/gh"
	"github.com/deligoez/cr/internal/git"
	"github.com/deligoez/cr/internal/intent"
	"github.com/deligoez/cr/internal/note"
	"github.com/deligoez/cr/internal/profile"
	"github.com/deligoez/cr/internal/render"
	"github.com/deligoez/cr/internal/state"
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
	var profileTie *profile.TieError
	if errors.As(err, &profileTie) {
		// §2.4.2: profiles matching the same number of marker files
		// abort with exit code 3 naming them, rather than one of them
		// being picked. It is not a malformed file — each tied profile
		// is valid on its own — but §2.4.2 fixes the same code.
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
