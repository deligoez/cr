package cli

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"slices"
	"strconv"
	"strings"
	"time"

	"github.com/spf13/cobra"

	"github.com/deligoez/cr/internal/finding"
	"github.com/deligoez/cr/internal/proposal"
	"github.com/deligoez/cr/internal/state"
)

// The actors §10.4 names: who takes a step.
const (
	actorCr    = "cr"
	actorAgent = "agent"
	actorHuman = "human"
)

// nextStep is one step of §10.4: what it is, who takes it, why it is owed, the
// exact commands, and the items it is owed for — the missing cells, the
// blocking claims, the files not yet recorded.
type nextStep struct {
	Step     string   `json:"step"`
	Actor    string   `json:"actor"`
	Why      string   `json:"why"`
	Commands []string `json:"commands"`
	Items    []string `json:"items"`
}

// nextResult is `cr next`'s document. Next is the first of Steps, and null for
// a round that owes nothing.
type nextResult struct {
	Round int        `json:"round"`
	Head  string     `json:"head"`
	Next  *nextStep  `json:"next"`
	Steps []nextStep `json:"steps"`
	// Honesty carries §9.3.1's head comparison, for a round that has one.
	Honesty []string `json:"honesty"`
}

func (r *nextResult) Text(w *writer) string {
	if len(r.Steps) == 0 {
		return "round " + strconv.Itoa(r.Round) + ": nothing is owed" + w.disclose("\n", "", r.Honesty...)
	}
	text := "round " + strconv.Itoa(r.Round) + ": " + strconv.Itoa(len(r.Steps)) + " step(s) owed"
	for i := range r.Steps {
		step := &r.Steps[i]
		text += "\n" + w.accent(step.Step) + " (" + step.Actor + "): " + step.Why
		for _, command := range step.Commands {
			text += "\n  $ " + command
		}
		if len(step.Items) > 0 {
			text += "\n  " + strings.Join(step.Items, ", ")
		}
	}
	return text + w.disclose("\n", "", r.Honesty...)
}

// newNextCmd is §10.4: the steps the round still owes, read out of state.
//
// It writes nothing and judges nothing. Every step is a fact about files §2.3
// names — a stamp, a missing cell, a record id no line holds — and the step's
// commands are the ones §11 already has; which claims an issue states, what a
// unit's reviewer finds, and whether a draft is ready to send stay with the
// agent and the human, and the step says whose they are.
func newNextCmd(out *writer) *cobra.Command {
	return &cobra.Command{
		Use:   "next " + prPlaceholder,
		Short: "Report the steps the round still owes, and who takes each",
		Args:  prArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			pr, err := parsePR(args[0])
			if err != nil {
				return err
			}
			owner, repo, err := repoOf(cmd)
			if err != nil {
				return err
			}
			layout, err := state.Default()
			if err != nil {
				return err
			}
			result, err := nextOf(layout, owner, repo, pr)
			if err != nil {
				return err
			}
			return out.emit(result)
		},
	}
}

// nextOf reads §10.4's steps for one pull request.
func nextOf(l state.Layout, owner, repo string, pr int) (*nextResult, error) {
	round, err := briefedRound(l, owner, repo, pr)
	var unbriefed *state.NotBriefedError
	if errors.As(err, &unbriefed) {
		return owed(0, "", []nextStep{briefStep(owner, repo, pr, nil,
			"no round has been opened on this pull request")}, make([]string, 0)), nil
	}
	if err != nil {
		return nil, err
	}
	// §9.3.1: the comparison is disclosed, and a moved head is also the one
	// step owed.
	disclosed := []string{round.Disclosure()}
	if round.Stale() {
		// §10.4.1: nothing else the round owes is owed at a head that has
		// moved, and §9.3.2 refuses the writes the other steps would make.
		return owed(round.Round, round.Head, []nextStep{briefStep(owner, repo, pr, &round.Meta,
			fmt.Sprintf("the pull request's head is %s and round %d was recorded at %s", round.Current, round.Round, round.Head))},
			disclosed), nil
	}
	steps, err := roundSteps(l, owner, repo, pr, &round.Meta)
	if err != nil {
		return nil, err
	}
	return owed(round.Round, round.Head, steps, disclosed), nil
}

// owed wraps the steps into the document, the first of them as the next one.
func owed(round int, head string, steps []nextStep, disclosed []string) *nextResult {
	result := &nextResult{Round: round, Head: head, Steps: steps, Honesty: disclosed}
	if len(steps) > 0 {
		result.Next = &steps[0]
	}
	return result
}

// roundSteps are §10.4.2 through §10.4.8 for a round whose head has not moved.
func roundSteps(l state.Layout, owner, repo string, pr int, meta *state.Meta) ([]nextStep, error) {
	steps := make([]nextStep, 0, 8)
	axes, _, err := lensesWith(l, owner, repo, meta, noHalves)
	if err != nil {
		return nil, err
	}
	covered, unsettled, err := intentCoverageOf(l, owner, repo, pr, meta)
	if err != nil {
		return nil, err
	}
	target := prTarget(owner, repo, pr)
	if pass := intentPassOf(axes, covered.Claims, meta); pass != nil {
		steps = append(steps, intentSteps(target, meta, pass.ClaimsHeld(), pass.Mapped)...)
	}
	pending, err := unrecordedFiles(l, owner, repo, pr, meta.Round)
	if err != nil {
		return nil, err
	}
	if len(pending.reviews)+len(pending.proposals) > 0 {
		steps = append(steps, recordStep(l, owner, repo, pr, meta.Round, pending))
	}
	_, missing, err := statusCoverage(l, owner, repo, pr, meta)
	if err != nil {
		return nil, err
	}
	if len(missing) > 0 {
		steps = append(steps, nextStep{
			Step: "review", Actor: actorAgent,
			Why: fmt.Sprintf("%d cell(s) of §10.2.2 are missing; `cr review` emits the prompts for exactly those", len(missing)),
			Commands: []string{
				"cr review " + target,
				"cr cells record " + target + " <cells.ndjson>",
			},
			Items: missing,
		})
	}
	if len(unsettled) > 0 {
		steps = append(steps, nextStep{
			Step: "settle", Actor: actorAgent,
			Why: "§10.2.3: these claims are mapped to no unit; map them, or set them aside on a note that says why",
			Commands: []string{
				"cr map record " + target + " <mapping.ndjson>",
				"cr claims set-aside " + target + " <claim-id> --note <note-id>",
			},
			Items: unsettled,
		})
	}
	return recordsSteps(l, owner, repo, pr, meta.Round, steps)
}

// intentSteps are §10.4.2 and §10.4.3: the claims the agent extracts from the
// issue, and the intent pass that maps them.
func intentSteps(target string, meta *state.Meta, claimsHeld, mapped bool) []nextStep {
	steps := make([]nextStep, 0, 2)
	if !claimsHeld {
		steps = append(steps, nextStep{
			Step: "claims", Actor: actorAgent,
			Why:      "the intent axis is active and no claims are recorded for this round; extract them from the issue text `cr brief` printed, one verbatim span each",
			Commands: []string{"cr claims record " + target + " <claims.ndjson>" + intentFileFlag(meta)},
			Items:    make([]string, 0),
		})
	}
	if !mapped {
		steps = append(steps, nextStep{
			Step: "intent", Actor: actorAgent,
			Why: "the intent axis is active and no mapping is recorded for this round; §4.6.5 runs the intent pass before the other axes",
			Commands: []string{
				"cr review " + target + " --axis intent",
				"cr map record " + target + " <mapping.ndjson>",
				"cr cells record " + target + " <cells.ndjson>",
			},
			Items: make([]string, 0),
		})
	}
	return steps
}

// recordsSteps are §10.4.7 and §10.4.8, over the round's records and every
// posted one of the pull request.
func recordsSteps(l state.Layout, owner, repo string, pr, round int, steps []nextStep) ([]nextStep, error) {
	records, err := roundFindingsOf(l, owner, repo, pr, round)
	if err != nil {
		return nil, err
	}
	target := prTarget(owner, repo, pr)
	unsent := make([]string, 0)
	for _, record := range records {
		if slices.Contains(finding.UnsentStates(), record.State) {
			unsent = append(unsent, record.ID)
		}
	}
	if len(unsent) > 0 {
		steps = append(steps, nextStep{
			Step: "draft", Actor: actorHuman,
			Why: "these records are in draft or queued: render the draft, read and edit every block, then " +
				"validate the payload; sending it is the human's `--confirm`, which this report never gives",
			Commands: []string{"cr draft " + target, "cr post " + target},
			Items:    unsent,
		})
	}
	concerns, err := postedConcernsOf(l, owner, repo, pr)
	if err != nil {
		return nil, err
	}
	if len(concerns) > 0 {
		posted := make([]string, 0, len(concerns))
		for _, concern := range concerns {
			posted = append(posted, concern.ID)
		}
		steps = append(steps, nextStep{
			Step: "recheck", Actor: actorAgent,
			Why:      "these records are posted and still await a verdict; `cr recheck` reports what came back",
			Commands: []string{"cr recheck " + target},
			Items:    posted,
		})
	}
	return steps, nil
}

// briefStep is §10.4.1, carrying forward the issue key and intent file the
// round was briefed with, so the command is the one that reproduces it.
func briefStep(owner, repo string, pr int, meta *state.Meta, why string) nextStep {
	command := "cr brief " + prTarget(owner, repo, pr)
	if meta != nil && meta.IssueKey != "" {
		command += " --issue " + shellWord(meta.IssueKey)
	}
	if meta != nil {
		command += intentFileFlag(meta)
	}
	return nextStep{Step: "brief", Actor: actorCr, Why: why, Commands: []string{command}, Items: make([]string, 0)}
}

// intentFileFlag is the `--intent-file` the round was briefed with, which
// `cr claims record` and a later brief each need again: neither inherits it.
func intentFileFlag(meta *state.Meta) string {
	if meta.IntentFile == "" {
		return ""
	}
	return " --intent-file " + shellWord(meta.IntentFile)
}

// shellWord is one argument spelled so a POSIX shell reads it back as that one
// argument: as it is when it holds nothing a shell treats specially, and in
// single quotes otherwise. §10.4 prints commands to be pasted, and a path under
// a home directory with a space in it would otherwise paste as two.
func shellWord(word string) string {
	if word != "" && strings.Trim(word, safeShellBytes) == "" {
		return word
	}
	return "'" + strings.ReplaceAll(word, "'", `'\''`) + "'"
}

// safeShellBytes are the bytes no POSIX shell gives a meaning to inside a word.
const safeShellBytes = "abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ0123456789_@%+=:,./-"

// prTarget is a command's pull request and repository, spelled out so a
// command runs the same from any directory.
func prTarget(owner, repo string, pr int) string {
	return strconv.Itoa(pr) + " --repo " + shellWord(owner+"/"+repo)
}

// pendingFiles are the fan-out files §10.4.4 names, by kind.
type pendingFiles struct {
	reviews, proposals []string
}

// recordStep is §10.4.4: merging and recording what the roles wrote. The merged
// file goes beside the round's fan-out, which holds the agent's files rather
// than cr's, so the command writes nowhere §2.3 fences.
func recordStep(l state.Layout, owner, repo string, pr, round int, pending pendingFiles) nextStep {
	target := prTarget(owner, repo, pr)
	commands := make([]string, 0, 2+len(pending.proposals))
	if len(pending.reviews) > 0 {
		merged := shellWord(filepath.Join(l.PRDir(owner, repo, pr), state.DirFanOut, strconv.Itoa(round), "merged.ndjson"))
		files := make([]string, 0, len(pending.reviews))
		for _, file := range pending.reviews {
			files = append(files, shellWord(file))
		}
		commands = append(commands,
			fmt.Sprintf("cr merge %s -o %s --repo %s --pr %d", strings.Join(files, " "), merged, shellWord(owner+"/"+repo), pr),
			"cr record "+target+" "+merged)
	}
	for _, file := range pending.proposals {
		commands = append(commands, "cr proposals record "+target+" "+shellWord(file))
	}
	return nextStep{
		Step: "record", Actor: actorCr,
		Why:      "the roles wrote records or proposals this round does not hold yet",
		Commands: commands,
		Items:    append(slices.Clone(pending.reviews), pending.proposals...),
	}
}

// unrecordedFiles finds §10.4.4's files in the round's fan-out: a review file
// holding a record id the round does not hold, unless the round's last merge
// or record ran after the file was last written — a record either dropped as
// waived or already posted, which the round holds no line for — and a
// proposals file holding a proposal id the round does not hold.
func unrecordedFiles(l state.Layout, owner, repo string, pr, round int) (pendingFiles, error) {
	pending := pendingFiles{reviews: make([]string, 0), proposals: make([]string, 0)}
	dir := filepath.Join(l.PRDir(owner, repo, pr), state.DirFanOut, strconv.Itoa(round))
	units, err := os.ReadDir(dir)
	if errors.Is(err, fs.ErrNotExist) {
		return pending, nil
	}
	if err != nil {
		return pending, state.FileFailure("read", dir, fanOutReadHint, err)
	}
	held, err := heldIDs(l, owner, repo, pr, round)
	if err != nil {
		return pending, err
	}
	intake := lastIntake(l.RoundFile(owner, repo, pr, round, state.FileIntake))
	for _, entry := range units {
		if !entry.IsDir() {
			continue
		}
		if err := pending.scanUnit(filepath.Join(dir, entry.Name()), held, intake); err != nil {
			return pending, err
		}
	}
	return pending, nil
}

// scanUnit adds one unit directory's unrecorded files. Every file is looked at:
// one with nothing owed says nothing about the file beside it.
func (p *pendingFiles) scanUnit(dir string, held map[string]bool, intake time.Time) error {
	files, err := os.ReadDir(dir)
	if err != nil {
		return state.FileFailure("read", dir, fanOutReadHint, err)
	}
	for _, file := range files {
		if file.IsDir() {
			// A role writes files; a directory under a unit is nobody's
			// output, whatever it is named.
			continue
		}
		path := filepath.Join(dir, file.Name())
		_, review := finding.RoleForFile(path)
		isProposals := proposalsFile(file.Name())
		if !review && !isProposals {
			continue
		}
		unheld, err := holdsUnheld(path, held)
		if err != nil {
			return state.FileFailure("read", path, fanOutReadHint, err)
		}
		switch {
		case !unheld:
		case isProposals:
			p.proposals = append(p.proposals, path)
		case writtenAfter(path, intake):
			p.reviews = append(p.reviews, path)
		}
	}
	return nil
}

// fanOutReadHint is §12.4's next step for a fan-out file or directory `cr next`
// could not read.
const fanOutReadHint = "a role's file under the round's fan-out could not be read; " +
	"make it readable, or remove it if no role wrote it, and run `cr next` again"

// proposalsFile reports whether name is the §5.7 file a role writes its
// proposals to: `proposals-<role>.ndjson`, not anything that merely opens so.
func proposalsFile(name string) bool {
	prefix, suffix, _ := strings.Cut(proposal.FanOutFile("\x00"), "\x00")
	role, found := strings.CutPrefix(name, prefix)
	if !found {
		return false
	}
	role, found = strings.CutSuffix(role, suffix)
	return found && role != ""
}

// heldIDs are the record and proposal ids the round's own lines hold.
func heldIDs(l state.Layout, owner, repo string, pr, round int) (map[string]bool, error) {
	records, err := roundFindingsOf(l, owner, repo, pr, round)
	if err != nil {
		return nil, err
	}
	proposals, err := state.ReadStamped[proposal.Proposal](l, owner, repo, pr, state.FileProposals, round)
	if err != nil {
		return nil, err
	}
	held := make(map[string]bool, len(records)+len(proposals))
	for _, record := range records {
		held[record.ID] = true
	}
	for i := range proposals {
		held[proposals[i].ID] = true
	}
	return held, nil
}

// holdsUnheld reports whether a file the agent wrote holds an id the round
// does not. A line that does not decode counts as unheld: `cr merge` and the
// recording commands are what refuse it by line, and a step that names the
// file sends the reader to that refusal rather than hiding it.
func holdsUnheld(path string, held map[string]bool) (bool, error) {
	ids, malformed, err := state.FanOutIDs(path)
	if err != nil || malformed {
		return malformed, err
	}
	return slices.ContainsFunc(ids, func(id string) bool { return !held[id] }), nil
}

// lastIntake is when the round's last `cr merge` or `cr record` wrote
// intake.json, and the zero time when neither has run.
func lastIntake(path string) time.Time {
	info, err := os.Stat(path)
	if err != nil {
		return time.Time{}
	}
	return info.ModTime()
}

// writtenAfter reports whether path was last written after since.
func writtenAfter(path string, since time.Time) bool {
	info, err := os.Stat(path)
	return err == nil && info.ModTime().After(since)
}
