package cli

import (
	"errors"
	"strconv"

	"github.com/deligoez/cr/internal/axis"
	"github.com/deligoez/cr/internal/brief"
	"github.com/deligoez/cr/internal/config"
	"github.com/deligoez/cr/internal/coverage"
	"github.com/deligoez/cr/internal/draft"
	"github.com/deligoez/cr/internal/finding"
	"github.com/deligoez/cr/internal/gh"
	"github.com/deligoez/cr/internal/git"
	"github.com/deligoez/cr/internal/intent"
	"github.com/deligoez/cr/internal/mapping"
	"github.com/deligoez/cr/internal/note"
	"github.com/deligoez/cr/internal/post"
	"github.com/deligoez/cr/internal/probe"
	"github.com/deligoez/cr/internal/profile"
	"github.com/deligoez/cr/internal/render"
	"github.com/deligoez/cr/internal/review"
	"github.com/deligoez/cr/internal/role"
	"github.com/deligoez/cr/internal/rule"
	"github.com/deligoez/cr/internal/sandbox"
	"github.com/deligoez/cr/internal/state"
	"github.com/deligoez/cr/internal/suggestion"
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

// usageHint is §12.4's next actionable step for the one thing §11.2's code 2
// means: the command line was wrong.
//
// It is what an error no row below claims takes, which is why it names the help
// rather than a file or a state directory. Anything that is not a malformed
// invocation belongs in the table instead — a file failure arriving here would
// tell the reader to retype an invocation that was right, which is the defect
// unreadable-input-exit-code was opened for.
const usageHint = "§11.2 codes a malformed invocation 2; check the command line against " +
	"`cr <command> --help`"

// mapped is one row of §11.2's mapping: which errors it claims, the code it
// gives them, and §12.4's next actionable step for whoever met one.
//
// The hint travels with the code because the two answer one question from
// either side — the code says what kind of failure this was, the hint says what
// to do about it — and a hint kept anywhere else would be free to drift from
// the code beside it. init below refuses a row that carries none, at package
// load rather than at the moment a hint is printed.
type mapped struct {
	// claims reports whether this row answers for err.
	claims func(error) bool
	// code is the §11.2 code.
	code int
	// hint is §12.4's next actionable step.
	hint string
}

// is builds a row's matcher for one error type.
//
// errors.As rather than a type assertion, for the reason every branch of this
// file used it before the table: a command wraps what it returns, and §11.2's
// answer is about the cause rather than about the outermost message.
func is[T error]() func(error) bool {
	return func(err error) bool {
		var target T
		return errors.As(err, &target)
	}
}

// codes maps an error onto the code §11.2 gives its cause. The mapping lives
// here rather than in the package that raises the error, so a validator below
// cli never has to name an exit code itself.
//
// It is a table rather than a chain of branches because it is one flat mapping,
// and invariant 5 pins what it returns: it is cleared by refactoring, never by
// raising a limit and never by renumbering to shorten it. exitCodeFor carried
// every branch itself until unreadable-input-exit-code, and was pinned at
// cognitive 58 over 173 statements against limits of 60 and 40 behind a
// //nolint this shape does not need.
//
// Order is significant in one direction. A row whose type wraps another mapped
// type must come first, because errors.As follows Unwrap: *state.
// NotBriefedError carries the *state.FileError that failed to read meta.json
// and is a state conflict rather than a file failure, so it sits above.
// *state.FileError is last of all, being the one every other file failure may
// carry underneath.
var codes = []mapped{
	// §1.5: an axis field outside the closed set is a bad configuration
	// file, which §11.2 codes as ExitFile.
	{is[*axis.InvalidError](), ExitFile,
		"§1.5 closes the axis set at intent, correctness, convention and test; " +
			"correct the axis field of the file the message names"},
	// §2.5 item 3: a malformed profile file aborts with exit code 3, naming
	// the file and the offending field.
	{is[*profile.MalformedError](), ExitFile,
		"repair the profile file the message names: the field it names, or the " +
			"file itself when it cannot be read"},
	// A §2.4 field the command needs and the profile does not set —
	// `tests.cmd` for §5.2.1's run, `tests.filter_flag` for its `--filter`
	// — or §2.4.4's no profile at all. Nothing is malformed and the
	// invocation is right; what refuses is the configuration, which §11.2
	// codes 3 alongside the malformed file above.
	{is[*profile.UnavailableError](), ExitFile,
		"set the field the message names in the resolved profile; " +
			"`cr config --resolved` shows the effective configuration"},
	// §5.2.1 runs the command the profile names, and cr has never heard of
	// it. A runner that could not be started is the external command
	// failure §3.1.3 fixes the shape of — and not a test result: a suite
	// that ran and failed returns its exit code as a value, because
	// §5.3.4's ladder is built on reading one.
	{is[*sandbox.RunError](), ExitFile,
		"the profile's test command could not be started; check that it is " +
			"installed and runnable inside the sandbox"},
	// §2.4.2: profiles matching the same number of marker files abort with
	// exit code 3 naming them, rather than one of them being picked. It is
	// not a malformed file — each tied profile is valid on its own — but
	// §2.4.2 fixes the same code.
	{is[*profile.TieError](), ExitFile,
		"the profiles the message names match equally well; give one of them " +
			"a marker file the other does not have"},
	// §2.5 item 3 names the role file alongside the profile one, on the
	// same code. It covers a key outside §2.5's table too: a role file cr
	// cannot read as written is unusable whether the fault is a missing
	// field or an invented one, and the invented one is the case §2.5's
	// division of labour turns on.
	{is[*role.MalformedError](), ExitFile,
		"repair the role file the message names: the key it names, or the file " +
			"itself when it cannot be read; §2.5's table is the whole of what a role may carry"},
	// §2.6.5: a malformed rule file aborts the command with exit code 3,
	// alongside the profile and role files above. It covers a key outside
	// §2.6's table too, for the reason the role one does: a rule file cr
	// cannot read as written is unusable whether the fault is a missing
	// field or an invented one, and the invented one is the case §2.6's
	// separation of rules from roles turns on.
	{is[*rule.MalformedError](), ExitFile,
		"repair the rule file the message names: the key it names, or the file " +
			"itself when it cannot be read; §2.6's table is the whole of what a rule may carry"},
	// §2.7: a CR_ variable or config key addressing a protected decision is
	// a configuration failure, which §11.2 codes as ExitFile. It is not a
	// validation failure: the run never reached input data, and it is not a
	// usage error, because nothing about the invocation can be corrected.
	{is[*config.ProtectedError](), ExitFile,
		"§2.7 protects this decision from configuration; remove the variable or " +
			"key the message names"},
	// §2.7: a layer cr could not use as written — a config file it could not
	// read or parse, a value not of its setting's type, a render.lang outside
	// the two v0.1 enumerates — is a configuration failure, which §11.2
	// codes 3. It sits above the unknown language it may carry, and hintFor
	// asks it for its own step, which names the file or variable, the layer
	// and the key: with five layers, one sentence for all of them would
	// leave the reader to find which one said it.
	{is[*config.LayerError](), ExitFile,
		"correct or remove the setting the message names, in the layer and file it names"},
	// §8.1.1 takes the language of every author-facing body from
	// render.lang, and §8.1.4 builds the question label in per language. A
	// value outside the two v0.1 enumerates has no built-in label, so
	// §6.3's forcing would reach the reader through nothing at all. It is a
	// configuration failure, which §11.2 codes 3 — the run never reached
	// input data, and nothing about the invocation can be corrected.
	{is[*render.UnknownLangError](), ExitFile,
		"set `render.lang` to a language §8.1.4's label table has a row for"},
	// §8.1.4's table has a row for every language Resolve admits and every
	// grade §6.2 computes, so a question it cannot label was read from
	// state cr did not write. That is a file cr cannot use, which §11.2
	// codes 3 beside the unknown language.
	{is[*render.NoLabelError](), ExitFile, state.UnusableHint},
	// A `probed` record naming a probe probes.ndjson does not hold was read
	// from state cr did not write, as a question the label table cannot
	// label was. §11.2 codes that 3 beside it.
	{is[*draft.MissingProbeError](), ExitFile, state.UnusableHint},
	// A record whose rule-origin citation names a rule the corpus no longer
	// resolves has no rationale for §8.1.6's region to quote per §2.6 item
	// 4. The rule is a file cr required and did not find, which §11.2 codes
	// 3 as it codes §2.6.5's unusable rule file.
	{is[*draft.MissingRationaleError](), ExitFile,
		"restore the rule the message names to a §2.6 layer (`cr rules list` shows the corpus), " +
			"so the comment can quote its rationale"},
	// §8.1.3 rejects a body that is empty or carries a `<!-- cr:` sequence
	// with exit code 1. Every file read and parsed; what is refused is
	// prose inside one record's block, which §11.2 codes 1 alongside the
	// record rejections below.
	{is[*render.BodyError](), ExitValidation,
		"edit that record's body in the draft; §8.1.3 refuses an empty one and " +
			"one carrying cr's own marker sequence"},
	// §8.2.4 fixes the code a suggestion failing §8.2's validation is
	// refused with, and it is the body refusal's: every file was read, and
	// what cannot be posted is one record's replacement range, which §11.2
	// codes 1 alongside the record rejections.
	{is[*suggestion.RangeError](), ExitValidation,
		"move that record's suggestion inside the range §8.2 admits, or drop " +
			"the suggestion block"},
	// §8.4.1 extends that pre-validation to every comment position, and a
	// plain comment anchored outside the diff is refused the same way: every
	// file was read, and what cannot be posted is one record's position.
	{is[*PositionError](), ExitValidation,
		"move that record's marker in draft.md back onto lines this round's diff " +
			"carries, or delete its block"},
	// §8.4.4's unknown outcome carries the gh.CommandError the call failed
	// with, so it sits above that row: gh ran, and what is unsettled is
	// whether the review exists, which §11.2 codes 4 as a partial post.
	{is[*UnknownOutcomeError](), ExitState,
		"run `cr post <pr> --reconcile`, which adopts the review the call created or clears " +
			"post_unresolved for a retry; a second `cr post --confirm` is refused until then"},
	// §3.1.3 codes a non-zero exit from an external command 3 and surfaces
	// its stderr. It fixes that for the tracker command, and git is one of
	// the same three external tools, so it fails through the same mapping
	// rather than a second one.
	{is[*git.CommandError](), ExitFile,
		"the git command the message names failed; run it yourself to see what " +
			"it reports"},
	// gh is the third of the external tools §3.1.3 governs, and a GraphQL
	// error reaches cr the same way a refusal does: gh exits non-zero with
	// the message on stderr. Both are code 3.
	{is[*gh.CommandError](), ExitFile,
		"the gh command the message names failed; check `gh auth status`, and " +
			"run the command yourself to see what it reports"},
	// gh ran, returned zero, and GitHub answered something cr cannot use.
	// That is §3.1.3's failure one step later and takes its code: the
	// command line is right, and no retyping of it changes the answer.
	{is[*gh.AnswerError](), ExitFile,
		"check that the pull request exists and that `gh auth status` reports an " +
			"account which can see it"},
	// §3.1.3 is written about this command in particular: a non-zero exit
	// fails with exit code 3 and surfaces the command's stderr. git and gh
	// borrow the clause; the tracker is what it was written for.
	{is[*intent.CommandError](), ExitFile,
		"the tracker command failed; run it yourself, or pass `--intent-file` " +
			"to read the issue from a file per §3.1.4"},
	// An intent.cmd §3.1.1 does not describe is a configuration failure,
	// which §11.2 codes 3 alongside the command failure it would otherwise
	// have become. Nothing about the invocation can be corrected, so it is
	// not a usage error.
	{is[*intent.MalformedCommandError](), ExitFile,
		"correct `intent.cmd` as the message describes, or pass `--intent-file`"},
	// §3.1.4's file stands in for the tracker command, so a file cr cannot
	// read fails the way the command it replaced would have: §11.2's code 3
	// covers the file and the external command in one row, and the run
	// reaches no issue text either way.
	{is[*intent.FileError](), ExitFile,
		"check that the path `--intent-file` names exists and is readable"},
	// §3.2 makes intent.key_pattern overridable without saying what an
	// uncompilable override does. §2.6.1.2 already fixed it for cr's other
	// configured regex — abort with exit code 3, naming what it came from —
	// and a profile's count pattern follows the same rule, so the third
	// configured expression does too.
	{is[*intent.KeyPatternError](), ExitFile,
		"correct the expression the message names so it compiles"},
	// `cr note` typed a key §3.2's pattern would never resolve, so the note
	// would land in a store no round loads. The configuration and the store
	// are fine; what is wrong is the key, §11.2's 1.
	{is[*intent.KeyShapeError](), ExitValidation,
		"name the issue key the way `intent.key_pattern` matches it, e.g. CR-1; " +
			"`cr config --resolved` shows the pattern in force"},
	// §3.2 leaves the key empty when none of its four sources yields one
	// and has the run continue, so this is recorded state rather than an
	// unusable file or a mistyped command line. What fails is the claim
	// recording itself: §3.3 forms every claim id out of the key, which
	// §11.2 codes 1 alongside note.NoIssueKeyError, the same fault reached
	// from §3.6.2.
	{is[*intent.NoIssueKeyError](), ExitValidation,
		"§3.3 forms every claim id out of the issue key; run `cr brief` with " +
			"`--issue` so the round has one"},
	// §3.3.1 rejects a claim with exit code 1. The file was found, read,
	// and parsed, so nothing about it failed as a file; what is wrong is
	// the agent's data inside it, exactly as it is for the record rejection
	// below.
	{is[*intent.RejectedClaimError](), ExitValidation,
		"correct the field the message names on the line it names, then record the file again"},
	// §3.3 closes the `source` row at four values, and a claim naming a
	// fifth is refused while its line is being decoded rather than after.
	// It is the same file and the same fault as the rejection above, so it
	// takes the same code; without this row a claim file with one mistyped
	// source would report 2, telling the user to retype a correct command
	// line.
	{is[*intent.UnknownClaimSourceError](), ExitValidation,
		"§3.3 closes `source` at four values; correct that claim's source in the file"},
	// §6.1 rejects a class that is not kebab-case. Like a supplied computed
	// field, the fault is the agent's data inside a file that was read and
	// parsed without trouble, which §11.2 codes 1 rather than the 3 an
	// unusable file gets.
	{is[*finding.InvalidClassError](), ExitValidation,
		"§6.1 wants a kebab-case class; correct that record's class in the file"},
	// §6.1.3 rejects a record missing a required field, naming one unit no
	// round has, or claiming a role other than the one whose output file it
	// arrived in, with exit code 1. The file read and parsed; the fault is
	// the agent's data inside it.
	{is[*finding.RejectedRecordError](), ExitValidation,
		"correct the record the message names in the file and run the command again"},
	// §6.3.3 rejects any attempt to post a record graded argued with kind
	// finding, with exit code 1, naming the record id. It is the invariant
	// refusing rather than the input: the record is well-formed, and what
	// fails is the register it would reach the author in.
	{is[*finding.ArguedAssertionError](), ExitValidation,
		"a record graded `argued` may only ask; make it a question, or probe or " +
			"cite it into a grade that may assert"},
	// §7.2.1 has `cr post` refuse to run when a marker is malformed, naming
	// the line, and §7.2 codes every marker edit it does not admit 1. The
	// draft read; what is wrong is what the reviewer typed into it.
	{is[*draft.MalformedMarkerError](), ExitValidation,
		"repair the marker on the line the message names, or run `cr draft` again"},
	// §7.2 codes every marker edit its table does not admit 1, naming the
	// record id, and §7.2.3 codes an unknown id the same. The draft read
	// and the marker parsed; what is refused is what the reviewer asked for
	// in it.
	{is[*draft.MarkerEditError](), ExitValidation,
		"§7.2's table is the whole of what a marker may be edited to; correct " +
			"that record's block in the draft"},
	// §7.2's immutable `id`, changed to the id of another record the round
	// renders. The way forward is the one edit that undoes it, and running
	// `cr draft` again cannot help: it reads the same file.
	{is[*draft.MarkerIDEditError](), ExitValidation,
		"put back the id cr wrote on the marker line the message names; a block never " +
			"changes record, and `cr draft` reads the same file again"},
	// §5.4.4's floor and §5.4.5's ceiling, refused by `cr record` before
	// anything is stored and by `cr post` before the payload is built. The
	// record parsed and every field §6.1 requires is there; what is wrong
	// is the severity the agent chose beside the experiment it rests on,
	// which §11.2 codes 1.
	{is[*finding.GapSeverityError](), ExitValidation,
		"change that record's severity to one §5.4's bounds admit for the probe " +
			"it rests on"},
	// The same clause, one level up: an input cr merge cannot bind to a
	// role leaves every role field in it unchecked. It is the file's
	// contents that are unusable rather than the file, so it codes 1 with
	// the rest of §6.1.3 rather than 3.
	{is[*finding.UnattributableFileError](), ExitValidation,
		"pass the per-role output paths `cr review` printed, which name the role " +
			"that wrote each"},
	// §1.6.2 blocks posting above post.max_comments with exit code 1,
	// naming the count. Every record in the round may be well-formed, so
	// what fails is the payload as a whole, and §11.2 runs that validation
	// before the confirmation gate: the block lands whether or not
	// --confirm was given.
	{is[*finding.CommentCapExceededError](), ExitValidation,
		"discard records in the draft until the count is within " +
			"`post.max_comments`, or raise the cap"},
	// §4.1.6 rejects a pair naming an unknown claim or unit id with exit
	// code 1. The file was found, read, and parsed, so nothing about it
	// failed as a file; what is wrong is the agent's judgement inside it,
	// exactly as it is for the cell rejection below.
	{is[*mapping.RejectedPairError](), ExitValidation,
		"correct the pair the message names; `cr status` lists the round's claims and units"},
	// §4.4.1 closes the classification at three words, and
	// testadequacy.Coverage refuses a fourth while the cell is being
	// decoded. It is the same file and the same fault as the cell rejection
	// below, so it takes the same code; without this row a cells file with
	// one mistyped classification would report 2, telling the user to
	// retype a correct command line.
	{is[*testadequacy.InvalidClassificationError](), ExitValidation,
		"§4.4.1 closes the classification at three words; correct that cell in the file"},
	// §4.5.6 rejects a cell naming an unknown unit id or an inactive role
	// with exit code 1, and §4.5.5's own field rules fail through the same
	// type. The file was found, read, and parsed, so nothing about it
	// failed as a file; what is wrong is the agent's data inside it,
	// exactly as it is for the claim and record rejections above.
	{is[*coverage.RejectedCellError](), ExitValidation,
		"correct the cell the message names; `cr status` lists the round's units " +
			"and active roles"},
	// §9.1: a transition its table does not list MUST be rejected with exit
	// code 4, naming the record and its current state. It is the first
	// thing mapped onto ExitState, and it is a state conflict rather than
	// bad input: the file parsed, the record is well-formed, and the
	// command line is right. What refuses is where the record already
	// stands, which no retyping and no edit to the input can change.
	{is[*finding.IllegalTransitionError](), ExitState,
		"`cr status` shows the state the record is in; §9.1's table lists the " +
			"transitions it admits from there"},
	// §3.7 makes `cr brief` the writer of meta.json, units.ndjson and
	// threads.ndjson, and §4.1.6, §4.5.6, §9.3.1 and §3.5.3 all read them
	// as authoritative. A command that found no round is therefore not
	// looking at a file it could be pointed at differently: the command
	// line is right and every file it named was read. What refuses is where
	// the pull request stands, which §11.2 codes 4 alongside the illegal
	// transition above — and the refusal names `cr brief`, because
	// recomputing the units instead would answer §4.1.6 against a set no
	// round recorded.
	//
	// It is above *state.FileError, which it carries when meta.json is the
	// file that was not there.
	{is[*state.NotBriefedError](), ExitState,
		"run `cr brief <pr>` to open a round on the pull request"},
	// §9.3's two halves of one conflict, mapped to one code because they are
	// the same answer to the same question: the head this round was opened
	// at is not the head the pull request has now, or cr could not learn
	// what the pull request has now. §9.3.2 refuses every write to per-PR
	// state in the first case, and §9.3.1 leaves cr no way to skip the
	// comparison in the second. Neither is bad input — the command line is
	// right and every file it named was read — and both are undone by
	// §9.3.3's `cr brief` rather than by retyping, which is what §11.2
	// codes 4 alongside the unbriefed pull request above.
	{is[*state.StaleRoundError](), ExitState,
		"run `cr brief <pr>` to open the round the current head belongs to"},
	{is[*state.NoCurrentHeadError](), ExitState,
		"run `cr brief <pr>` once cr can read the pull request's current head"},
	// §4.6.1's prompt shows a unit the round recorded, and the diff at the
	// recorded head no longer gives it. The command line is right and
	// nothing the user named is malformed; what refuses is where the round
	// stands, which §11.2 codes 4 beside the unbriefed pull request above,
	// and the refusal names the `cr brief` that records the units again.
	{is[*review.StaleUnitError](), ExitState,
		"run `cr brief <pr>` to record the units the diff gives now"},
	// §9.3.5 scopes a round's records to the round, and §3.3 forms every
	// claim id out of the issue key, so a key rewritten under a round is a
	// round whose claims name an issue it no longer has. The command line
	// is right and every file it named was read; what refuses is that the
	// recorded round and the key §3.2 just resolved disagree, which §11.2
	// codes 4 beside the moved head above.
	{is[*brief.KeyRewriteError](), ExitState,
		"pass the issue key the round recorded, which the message names"},
	// §4.6.5 runs the fan-out in two passes and refuses the remaining axes
	// until the first has produced a mapping. The command line is right and
	// nothing it named is malformed; what refuses is that the round has not
	// been through its intent pass, which is where the round stands and
	// §11.2 codes 4 beside the stale unit above.
	{is[*review.MappingRequiredError](), ExitState,
		"run the intent pass and record its mapping with `cr map record` first"},
	// The same refusal reached from `cr cells record`: a cell for a role off
	// the intent axis, in a round with no mapping, was filled without the
	// prompt §4.6.5 withholds. The file is well formed; what refuses is where
	// the round stands, so it takes the 4 `cr review` gets for it.
	{is[*coverage.MappingRequiredError](), ExitState,
		"run the intent pass and record its mapping with `cr map record`, then record the cells again"},
	// §4.6.6 is the other side of that refusal: a round whose intent axis
	// is unavailable has no mapping to record, so `cr map record` is not
	// accepted. The command line is right and the file may be well formed;
	// what refuses is that the round resolved no issue key, which §11.2
	// codes 4 beside it.
	{is[*mapping.NotAcceptedError](), ExitState,
		"record nothing: run `cr review <pr>`, or `cr brief <pr> --issue <KEY>` if the pull request has an issue"},
	// §5.1.3 runs the commands the profile names, and cr has never heard of
	// any of them. A refusal is therefore the external command failure
	// §3.1.3 fixes the shape of — exit code 3 with the command's stderr
	// surfaced — and not a malformed invocation: the command line was right
	// and what failed was the tool the profile asked for.
	{is[*sandbox.SetupError](), ExitFile,
		"the profile's setup command failed; run it yourself in a checkout to " +
			"see what it reports"},
	// §5.1.1 creates the sandbox worktree and §5.1.5 removes it, and a
	// sandbox that is already there is neither a file cr could be pointed
	// at differently nor input it could refuse: the command line is right,
	// the pull request is briefed, and what refuses is that a checkout cr
	// did not just make is standing in the one place §5.1.1 puts one.
	// §11.2 codes that 4 alongside the illegal transition and the unbriefed
	// round.
	{is[*sandbox.ExistsError](), ExitState,
		"run `cr sandbox destroy <pr>` before creating the sandbox again"},
	// §8.4.2 fixes the code itself: a rejected review-creation call exits 4,
	// having marked nothing posted. It is deliberately not the 3
	// gh.CommandError takes above — the command ran and GitHub answered,
	// and what refuses is that the positions cr pre-validated per §8.4.1
	// are not the positions GitHub has, a disagreement about the pull
	// request rather than a broken tool.
	{is[*post.RejectedError](), ExitState,
		"nothing was posted; correct or discard the records GitHub named in the " +
			"draft and run `cr post` again"},
	// §8.4.4 refuses a second send while a round's posting is unsettled. The
	// command line is right and the payload may be valid; what refuses is
	// that the earlier call may have created the review, which §11.2 codes 4
	// beside the rejection above — and the step is the reconciliation, never
	// a retry.
	{is[*UnresolvedPostError](), ExitState,
		"run `cr post <pr> --reconcile` to adopt the review the earlier call created, " +
			"or to clear post_unresolved for a retry"},
	// §8.3.1: a round whose review was created or adopted posts no second
	// one. Nothing about the invocation is wrong; what refuses is that the
	// round is already posted, which §11.2 codes 4 beside the empty review.
	{is[*PostedRoundError](), ExitState,
		"this round is posted and takes no second review; `cr status <pr>` reports where its " +
			"records stand, and `cr brief <pr>` opens the next round once the head moves"},
	// §8.3.1 and §8.4.4: a round holding no queued record has no review to
	// send, and a second, empty one would notify the author of nothing. The
	// command line is right and nothing is malformed; what refuses is where
	// the round's records stand, which §11.2 codes 4.
	{is[*EmptyReviewError](), ExitState,
		"nothing in this round is left to post; `cr status <pr>` reports where its records " +
			"stand, `cr draft <pr>` queues records recorded since, and `cr brief <pr>` opens " +
			"the next round once the head moves"},
	// §5.6.2 codes the lock timeout itself: cr waits up to
	// `probe.lock_timeout_seconds` and then fails with exit code 4, which
	// §11.2's table names in as many words — "state conflict, including
	// lock timeout". Nothing about the invocation is wrong and no file
	// failed; what refuses is that another cr run is already inside the
	// same repository and profile. The step is state.ProbeLockedHint, so the
	// row and the error cannot name two different ones.
	{is[*state.ProbeLockedError](), ExitState, state.ProbeLockedHint},
	// §5.4.2: an existing file where a gap probe's test would go aborts.
	// Nothing about the invocation is wrong and every file named was read;
	// what refuses is that the one path §2.4's template resolves to is
	// occupied, and cr would destroy the file there and then delete it.
	// §11.2 codes that 4 alongside the sandbox that is already standing.
	{is[*state.ProbeFileExistsError](), ExitState,
		"move the file the message names out of the sandbox, or recreate the sandbox"},
	// §5.4.2 fixes a gap probe's id before the run and §5.5.2 has a finding
	// reference a probe by it, so an id another run allocated in between is
	// a conflict between two cr runs rather than bad input. §11.2 codes
	// that 4, as it does the lock timeout the same collision usually shows
	// up as.
	{is[*probe.IDTakenError](), ExitState,
		"another cr run took that probe id; run `cr probe run` again"},
	// §2.2's state directory is opened by `cr brief`, and §3.6.2's answer
	// reads the issue key out of it. A pull request with no state is a file
	// cr expected and did not find, which §11.2 codes 3 alongside its other
	// file failures.
	//
	// It is above *state.FileError for the reason *state.NotBriefedError
	// is: it may carry one.
	{is[*note.NoStateError](), ExitFile,
		"run `cr brief <pr>` to open the pull request's state directory"},
	// §3.2 leaves the issue key empty when none of its four sources yields
	// one, and has the run continue, so this is recorded state rather than
	// an unusable file or a mistyped command line. What fails is the answer
	// itself: §3.6.2 has no store to keep it in, which §11.2 codes 1.
	{is[*note.NoIssueKeyError](), ExitValidation,
		"run `cr brief` with `--issue` so the pull request has an issue key to " +
			"keep notes under"},
	// §4.1.8 stamps `set_aside_note` on the gap entry §4.1.3 raised, and a
	// claim the round maps to a unit raises none. The state directory read
	// and parsed without trouble and the command line is the shape §11
	// gives it; what is wrong is the claim id the caller named, which §11.2
	// codes 1 alongside the unknown claim id §4.1.6 refuses a mapping pair
	// for.
	{is[*mapping.NoGapError](), ExitValidation,
		"name a claim with a gap; `cr status` lists them"},
	// §3.6.6 retracts a note by id. An id naming no note is spelled the way
	// §3.6.1 spells one, and the store read and parsed without trouble, so
	// nothing about the invocation or the file is wrong; what fails is the
	// retraction, which §11.2 codes 1 alongside note.NoIssueKeyError.
	{is[*note.UnknownNoteError](), ExitValidation,
		"`cr context <ISSUE-KEY>` lists the notes and their ids"},
	// §3.6.2's answer naming a record id the pull request's records do not
	// hold. The state read without trouble and the id is spelled as §6.1
	// spells one; what fails is the answer, §11.2's 1 beside the unknown note.
	{is[*note.UnknownRecordError](), ExitValidation,
		"answer a record the pull request holds, by the id `cr record` reported when it stored the record"},
	// §4.1.8's set-aside naming a note §3.6.6 retracted. The note exists and
	// the store read without trouble; what fails is a set-aside that could
	// not settle the claim, which §11.2 codes 1 beside the unknown note.
	{is[*RetractedSetAsideNoteError](), ExitValidation,
		"record why the claim is out of scope with `cr note`, then set it aside on that note's id"},
	// §7.4.7 removes a waiver by id, and the id names its own file. An id
	// spelled the way cr spells one that the file does not hold is the
	// retraction above's fault in the other store: the invocation is right
	// and the file read without trouble, and what fails is the removal,
	// which §11.2 codes 1 beside it.
	{is[*finding.UnknownWaiverError](), ExitValidation,
		"`cr waivers list --repo <owner/repo> --pr <n>` lists the waivers of both scopes and their ids"},
	// §3.6's store is a file cr found and could not use, which §11.2 codes
	// 3 with its other file failures. Without this row `cr note`,
	// `cr answer` and `cr context` all reported a corrupt store as a
	// malformed invocation, and no retyping of the command could ever have
	// fixed it.
	{is[*state.ContextStoreError](), ExitFile, state.UnusableHint},
	// §1.4 step 1 fixes the code itself: text that does not decode as UTF-8
	// fails with exit code 1. It is the first content fault in the tree
	// that is not a field of a parsed record — the file was found and read
	// whole, and what is unusable is the bytes in it, which §11.2 codes 1
	// rather than the 3 a file cr cannot use as a file gets.
	{is[*text.InvalidUTF8Error](), ExitValidation,
		"§1.4 requires UTF-8; re-encode the text the message names"},
	// §5.3.2 derives a mutation probe's target from the patch, and §5.5
	// makes it a required row of the record. A patch no target follows from
	// is therefore input cr cannot run, and the fault is the agent's data
	// inside a file that was found, read, and parsed without trouble —
	// which §11.2 codes 1 alongside the record rejections above.
	{is[*probe.TargetError](), ExitValidation,
		"supply a patch whose change §5.3.2 can derive a target line from"},
	// §5.6.4 caps probe executions per round and §11.2 gives the refusal
	// the code §1.6.2's comment cap already takes: the invocation is right
	// and every file named was read, and what refuses is the round's
	// budget. Raising probe.max_per_round is the user's decision through
	// §2.7, as triaging down to post.max_comments is.
	{is[*probe.RoundCapReachedError](), ExitValidation,
		"the round's probe budget is spent; raising `probe.max_per_round` is the " +
			"user's decision"},
	// §5.5 fixes the result vocabulary per kind and says it must not be
	// shared, and the check refuses a record on the way into probes.ndjson.
	// Nothing about the invocation can be retyped to fix it, and no file
	// failed as a file; what is refused is a record, which §11.2 codes 1
	// alongside the record rejections above.
	{is[*probe.OutsideVocabularyError](), ExitValidation,
		"use a result from the vocabulary §5.5 gives the probe's kind"},
	// §5.5 takes a gap probe's target from `--target` and has it validated
	// as §6.2.3 validates a citation, and §6.2.3 codes its own refusals 1.
	// The flag was read and the head was opened without trouble; what is
	// wrong is the location the agent typed, which is its data exactly as a
	// citation's path and line are.
	{is[*probe.InvalidTargetError](), ExitValidation,
		"pass a `--target` path and line the round's head holds"},
	// §5.3.1's patch is written by an agent, and a path in it that resolves
	// outside the sandbox would have cr write into the repository under
	// review — which §5.1.4 and invariant 2 both forbid. The file was
	// found, read, and parsed, so nothing about it failed as a file; what
	// is wrong is the data inside it, which §11.2 codes 1 alongside the
	// record rejections above. It is deliberately not §5.3.4's first rung:
	// a patch aimed out of the tree did not fail to apply, it was refused
	// before anything could be applied at all.
	{is[*state.OutsideSandboxError](), ExitValidation,
		"correct the patch so every path it names resolves inside the sandbox"},
	// §6.1.4: a record supplying a field cr writes itself is rejected with
	// exit code 1. The file was found, read, and parsed, so nothing about
	// it failed as a file; what is wrong is the input data inside it, which
	// §11.2 codes 1 rather than the 3 a malformed profile gets.
	{is[*state.ReservedFieldError](), ExitValidation,
		"drop the field the message names from that record; §6.1.4 has cr write it"},
	// §6.1.4's and §3.3's fences read one value under each key, and the
	// decode keeps parts of both copies of a key given twice, so the line is
	// refused rather than judged on a value it is not stored holding. The
	// file was read and parsed; what is wrong is the data inside it, §11.2's 1.
	{is[*state.RepeatedKeyError](), ExitValidation,
		"give each field of that record once; the message names the key given more than once"},
	// A line of the file a caller handed a recording command that is not one
	// JSON object, or gives a field the wrong type. The file was found and
	// read; what is wrong is the data inside it, §11.2's 1. A line of cr's own
	// stored file that does not decode is not this error: it reaches the
	// file-failure floor below with state.UnusableHint, §11.2's 3.
	{is[*state.MalformedLineError](), ExitValidation,
		"correct the line the message names so it is one JSON object whose fields carry " +
			"the types the command's record schema gives them"},
	// §5.3.1's patch is written by an agent, and one that is not a unified
	// diff was found and read; what is wrong is the data inside it, §11.2's 1.
	{is[*git.MalformedPatchError](), ExitValidation,
		"correct the `--patch` file at the line the message names so it is a unified diff " +
			"with --- and +++ headers and @@ hunks; if it came from `git diff`, re-run it with --no-ext-diff"},
	// §5.6.1's probe lock is named after the checkout's absolute path and
	// the profile id meta.json records, and internal/state refuses a pair
	// whose lock file would leave the probe locks directory. filepath.Abs
	// has cleaned the path, so what climbs out is the profile id, read from
	// cr's own state as a file cr cannot use as written: §11.2's 3. It is
	// above *state.OutsideRootError, which it carries, because `--repo`
	// answers neither half.
	{is[*state.ProbeLockOutsideError](), ExitFile,
		"the probe lock is named after the profile_id in the pull request's meta.json, " +
			"which is one profile's file name; " + state.UnusableHint},
	// §2.2 keeps all of cr's state under one root, and internal/state
	// refuses an owner and repository whose paths would leave it. splitRepo
	// refuses such a `--repo` first, so what reaches this row came from
	// somewhere else, and naming the repository on the command line is the
	// step. §11.2 codes that 2, as it does the detection failure below.
	{is[*state.OutsideRootError](), ExitUsage,
		"pass --repo <owner/repo>, where neither half is . or .. or holds a separator"},
	// §11.1 makes `--repo` the override for repository detection, so a
	// repository detection cannot name is answered by the flag. The command
	// line lacked the one argument that would have settled it, which §11.2
	// codes 2.
	{is[*RepositoryDetectionError](), ExitUsage,
		"run cr inside a clone whose one remote is its GitHub repository, or pass --repo <owner/repo>"},
	// A file cr had to read, write or use and could not: an input the
	// caller named, a file of §2.2's tree a command required, or a §2.3
	// write that did not land. §11.2 codes all three 3.
	//
	// It is last, because every row above may carry one underneath and each
	// of them has a more exact answer than "a file failed". The hint here is
	// a floor rather than a path: state.FileFailure refuses to build one
	// without a step of its own, and hintFor asks the error before the table.
	{is[*state.FileError](), ExitFile,
		"check that the file the message names exists and that its directory is " +
			"readable and writable"},
}

// exitCodeFor maps an error onto the code §11.2 gives its cause. An unmapped
// cause is a malformed invocation, which is the one thing §11.2's code 2 means.
func exitCodeFor(err error) int {
	if row := rowFor(err); row != nil {
		return row.code
	}
	return ExitUsage
}

// hintFor is §12.4's next actionable step for one error.
//
// The row that decides the code decides the hint, with two exceptions: when
// that row is the file-failure floor, the *state.FileError answers for itself,
// which is what lets it name the command that writes the particular file that
// was missing rather than one sentence for every file; and when that row claims
// a *config.LayerError, the error names the file or variable, the layer and the
// key to correct, which one sentence for every layer could not. A row above the
// floor keeps its own even when the error it claims carries a file failure
// inside — *state.NotBriefedError's `cr brief <pr>` is the step, not the bare
// read that found no meta.json. An error no row claims takes the usage hint,
// because a malformed invocation is what exitCodeFor concluded about it.
func hintFor(err error) string {
	row := rowFor(err)
	if row == nil {
		return usageHint
	}
	var layer *config.LayerError
	if errors.As(err, &layer) && row.claims(layer) {
		return layer.Hint()
	}
	// A zero *state.FileError built outside FileFailure carries no step of
	// its own, and takes the floor's rather than an empty one.
	var file *state.FileError
	if row == &codes[len(codes)-1] && errors.As(err, &file) && file.Hint() != "" {
		return file.Hint()
	}
	return row.hint
}

// rowFor is the first row that claims err, and nil when none does. The code and
// the hint are both read off it, so the two can never come from different rows.
func rowFor(err error) *mapped {
	for i := range codes {
		if codes[i].claims(err) {
			return &codes[i]
		}
	}
	return nil
}

// init refuses a table row that names no next actionable step, and a table
// whose last row is not the file-failure floor hintFor defers to.
//
// §12.4 requires every error to carry a hint, and a row is where a mapped
// error's comes from. The check runs at package load for the reason output.go's
// omittedFields panics there: a rule written only in a comment is a rule the
// next edit reads past.
func init() {
	for i := range codes {
		if codes[i].hint == "" {
			panic("§12.4: row " + strconv.Itoa(i) + " of the exit code table names no next actionable step")
		}
	}
	floor := state.FileFailure("read", "floor", "floor", nil)
	if !codes[len(codes)-1].claims(floor) {
		panic("§11.2: the exit code table's last row must be the *state.FileError floor")
	}
}
