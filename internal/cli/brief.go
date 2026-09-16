package cli

import (
	"errors"
	"fmt"
	"os"
	"strconv"
	"strings"

	"github.com/spf13/cobra"

	"github.com/deligoez/cr/internal/brief"
	"github.com/deligoez/cr/internal/config"
	"github.com/deligoez/cr/internal/gh"
	"github.com/deligoez/cr/internal/intent"
	"github.com/deligoez/cr/internal/state"
	"github.com/deligoez/cr/internal/unit"
)

// The two seams `cr brief` reaches the outside world through.
//
// They are variables rather than calls written into the command for one
// reason: §3.7's payload is assembled out of a pull request cr cannot invent
// and a repository cr cannot create, so a test of the command that could not
// substitute either would be a test that either reached the network or tested
// nothing. internal/gh already makes the transport injectable through
// gh.WithRunner; this is the same seam, one level up, and the working
// directory beside it.
//
// Neither widens what cr can do. gh.New is the read client, and §2.1.2's
// boundary refuses a write whichever runner is behind it.
var (
	// ghClient reads the pull request and its threads.
	ghClient = gh.New
	// repoDir is the repository under review: the directory cr was run
	// from. §11.1 makes `--repo` the override for repository detection and
	// not for the checkout, and §2.2 forbids writing inside it either way.
	repoDir = os.Getwd
)

// briefResult is what `cr brief` reports: §3.7's payload, whole.
//
// The payload is embedded rather than summarised because §3.7 has cr assemble
// and print six things, and every one of them is a fact the agent orients on.
// A count of the units would leave the agent to re-derive the units themselves,
// which §3.7 wrote this command to stop it doing.
type briefResult struct {
	*brief.Brief
	// Honesty is §4.5.4's report, rendered as the sentences §11.1 exempts
	// from `--quiet`. It is a field on the payload rather than a second
	// stream, so the reader of the JSON document and the reader of the
	// terminal are told the same thing by the same values.
	Honesty []string `json:"honesty"`
	// closed is how many of Honesty's sentences, at its front, say that
	// the pull request is closed or merged: the terminal prints those
	// beside the pull request's identity rather than among the axes.
	closed int
	// links is how many of Honesty's sentences, at its end, name a link
	// the issue text carries and cr did not read: the terminal prints
	// those under the issue text rather than among the axes.
	links int
}

// newBriefResult renders §3.7's payload for printing, and with it §4.5.4's
// report of every lens that did not run.
//
// The disclosures are asked of the payload rather than assembled here. The
// coverage.Lenses they come from is the one `cr review` and `cr status` report,
// and a wording built at the call site would be a second answer that can
// disagree with the data beside it.
func newBriefResult(assembled *brief.Brief) *briefResult {
	disclosed := assembled.Disclosures()
	closure := closureDisclosure(assembled.Owner, assembled.Repo, assembled.PR, assembled.PullRequest())
	honesty := append(make([]string, 0, len(closure)+len(disclosed)), closure...)
	for _, entry := range disclosed {
		honesty = append(honesty, entry.Disclosure())
	}
	honesty = append(honesty, assembled.StaleProfile()...)
	// Every link in the issue text, as linked and not read: §3.1 reads the
	// issue text alone, so a requirement stated only behind one reaches no
	// claim, and nothing else would say so. A terminal hyperlink's target is
	// one of them though cleaning took it out of the text.
	links := assembled.IssueLinks()
	for _, link := range links {
		honesty = append(honesty, intent.LinkDisclosure(link))
	}
	return &briefResult{Brief: assembled, Honesty: honesty, closed: len(closure), links: len(links)}
}

// Text renders §3.7's six items in the order §3.7 numbers them.
//
// Nothing here ranks, scores, weighs, or summarises. Every line is a value cr
// fetched or computed — an identity, a hash, a range, a reason some other
// section already fixed — because §3.7 has the brief print "without judgement"
// and invariant 1 leaves cr no way to form one. The rendering's only decisions
// are order and indentation.
func (r *briefResult) Text(w *writer) string {
	var out strings.Builder
	r.identity(w, &out)
	r.issue(w, &out)
	r.claims(w, &out)
	r.units(w, &out)
	r.threadsAndNotes(w, &out)
	r.axes(w, &out)
	return strings.TrimRight(out.String(), "\n")
}

// identity is §3.7.1: the pull request, its head, its merge base, and the
// resolved profile together with the layer that selected it.
func (r *briefResult) identity(w *writer, out *strings.Builder) {
	fmt.Fprintf(out, "%s %s round %d\n",
		w.accent("pull request"), r.Owner+"/"+r.Repo+"#"+strconv.Itoa(r.PR), r.Round)
	out.WriteString(w.disclose("  ", "\n", r.Honesty[:r.closed]...))
	fmt.Fprintf(out, "  head       %s\n", r.Head)
	// §9.3.4's sweep, said where the round it opened is: a reader shown
	// round 2 is told in the same place which records the move closed.
	if len(r.Staled) > 0 {
		fmt.Fprintf(out, "  staled     %d open record(s) moved to stale, per §9.3.4: %s\n",
			len(r.Staled), strings.Join(r.Staled, ", "))
	}
	fmt.Fprintf(out, "  merge base %s\n", r.MergeBase)
	if r.Profile.Selected {
		fmt.Fprintf(out, "  profile    %s, selected by %s\n",
			r.Profile.ID, r.Profile.SelectionLayer)
		return
	}
	fmt.Fprintf(out, "  profile    none, %s\n", r.Profile.SelectionLayer)
}

// issue is §3.7.2: the key and the source it was resolved from, with the issue
// text, or the reason no key was found.
//
// The text is printed whole rather than trimmed to a first line. §3.3 has the
// agent draw every claim out of it as a verbatim span, and a preview would be
// text no span could be checked against.
func (r *briefResult) issue(w *writer, out *strings.Builder) {
	if r.Issue.Key == "" {
		fmt.Fprintf(out, "\n%s none: %s\n", w.accent("issue"), r.Issue.Reason)
		return
	}
	fmt.Fprintf(out, "\n%s %s, from the %s\n", w.accent("issue"), r.Issue.Key, r.Issue.Origin)
	for line := range strings.SplitSeq(strings.TrimRight(r.Issue.Text, "\n"), "\n") {
		fmt.Fprintf(out, "  | %s\n", line)
	}
	out.WriteString(w.disclose("  ", "\n", r.Honesty[len(r.Honesty)-r.links:]...))
}

// claims is §3.7.3: the claims of §3.3, and whether the issue text has drifted
// per §3.3.3.
//
// Drift is reported per claim as well as in one line, because the two answer
// different questions: the hashes say the issue text moved, and `span present`
// says whether this claim's own span survived it. §1.4 normalises before
// hashing, so a whitespace-only edit leaves the hashes equal and can still take
// a span away.
func (r *briefResult) claims(w *writer, out *strings.Builder) {
	drifted := "issue text unchanged since extraction"
	if r.Drift.Drifted {
		drifted = "issue text has drifted since extraction, per §3.3.3"
	}
	fmt.Fprintf(out, "\n%s %d recorded; %s\n",
		w.accent("claims"), len(r.Claims), drifted)
	sources := make(map[string]string, len(r.Claims))
	for i := range r.Claims {
		fmt.Fprintf(out, "  %s  %s\n", r.Claims[i].ID, r.Claims[i].Text)
		if r.Claims[i].Source == intent.ClaimFromNote {
			sources[r.Claims[i].ID] = "note " + r.Claims[i].NoteID
		}
	}
	for i := range r.Drift.Claims {
		id := r.Drift.Claims[i].ID
		fmt.Fprintf(out, "  %s  span %s\n", id, spanState(r.Drift.Claims[i].SpanOccurs, sources[id]))
	}
	if r.Issue.Key != "" {
		out.WriteString(paragraphLines(&r.Paragraphs, "  "))
	}
}

// paragraphLines renders the issue text's paragraphs no claim span overlaps:
// the count, then each one with its line range and its text. The text is
// printed whole, because a paragraph is listed so that a claim can be drawn
// from it, and a claim is drawn from a verbatim span.
func paragraphLines(paragraphs *intent.Paragraphs, indent string) string {
	var out strings.Builder
	fmt.Fprintf(&out, "%sissue paragraphs: %d total, %d overlapped by no claim span\n",
		indent, paragraphs.Total, len(paragraphs.Uncovered))
	for i := range paragraphs.Uncovered {
		uncovered := &paragraphs.Uncovered[i]
		fmt.Fprintf(&out, "%s  lines %d-%d\n", indent, uncovered.StartLine, uncovered.EndLine)
		for line := range strings.SplitSeq(uncovered.Text, "\n") {
			fmt.Fprintf(&out, "%s    | %s\n", indent, line)
		}
	}
	return out.String()
}

// spanState words §3.3.3's per-claim answer, naming the text the span was
// checked in: the issue text, or for a claim drawn from a note, that note.
func spanState(occurs bool, source string) string {
	if source == "" {
		source = "the issue text"
	}
	if occurs {
		return "still occurs in " + source
	}
	return "no longer occurs in " + source
}

// units is §3.7.4: the units of §3.4 with their file paths, hunk ranges, and
// unit hashes.
func (r *briefResult) units(w *writer, out *strings.Builder) {
	fmt.Fprintf(out, "\n%s %d\n", w.accent("units"), len(r.Units))
	for i := range r.Units {
		formed := &r.Units[i]
		fmt.Fprintf(out, "  %s  %s %s  %s  %s  by %s%s\n",
			formed.ID, formed.Path, formed.Side,
			ranges(formed.HunkRanges), formed.Hash, formed.Formation,
			oversized(formed.Oversized))
	}
	out.WriteString(filesLines(&r.Files, "  "))
}

// filesLines renders what §3.4 formed no unit from: §3.4.2's excluded count,
// and every file §3.4.7 lists with its kind. The count is printed at zero too,
// because a reader shown no line cannot tell a diff nothing was excluded from
// a report that never looked.
func filesLines(files *unit.Files, indent string) string {
	var out strings.Builder
	fmt.Fprintf(&out, "%s%d file(s) excluded by ignore.globs, per §3.4.2; %d listed and not clustered, per §3.4.7\n",
		indent, files.Excluded, len(files.Listed))
	for _, listed := range files.Listed {
		fmt.Fprintf(&out, "%s  %s (%s)\n", indent, listed.Path, listed.Kind)
	}
	return out.String()
}

// ranges renders a unit's hunk ranges, both ends inclusive and head-side, in
// the order §3.4.6 records them.
func ranges(hunks []unit.Range) string {
	spans := make([]string, 0, len(hunks))
	for _, span := range hunks {
		spans = append(spans, strconv.Itoa(span.Start)+"-"+strconv.Itoa(span.End))
	}
	return strings.Join(spans, ",")
}

// oversized names §3.4.5's one exception, and nothing at all otherwise: a flag
// printed on every unit would be noise on all but the rare one it is about.
func oversized(flagged bool) string {
	if flagged {
		return ", oversized per §3.4.5"
	}
	return ""
}

// threadsAndNotes is §3.7.5: the ingested threads of §3.5 and the notes of
// §3.6 for the issue key.
//
// A thread is printed with its author type and its anchor and with no class:
// §3.5.3 forbids cr to assign one or to decide suppression, so what is shown is
// where the thread hangs and who wrote it, and the agent decides the rest. The
// author's replies follow as §3.5.5's candidate notes, each with the command
// that would record it, because recording one is a human's call and not cr's.
func (r *briefResult) threadsAndNotes(w *writer, out *strings.Builder) {
	fmt.Fprintf(out, "\n%s %d\n", w.accent("threads"), len(r.Threads))
	for i := range r.Threads {
		thread := &r.Threads[i]
		fmt.Fprintf(out, "  %s  %s:%d %s  by %s (%s)%s\n",
			thread.ID, thread.Anchor.Path, thread.Anchor.Line, thread.Anchor.Side,
			thread.Comment.Author, thread.AuthorType, resolvedState(thread.Resolved))
	}
	fmt.Fprintf(out, "\n%s %d\n", w.accent("notes"), len(r.Notes))
	for i := range r.Notes {
		fmt.Fprintf(out, "  %s  %s\n", r.Notes[i].ID, provenanceOf(&r.Notes[i]))
		fmt.Fprintf(out, "    %s\n", r.Notes[i].Text)
	}
	fmt.Fprintf(out, "\n%s %d\n", w.accent("candidate notes"), len(r.CandidateNotes))
	for i := range r.CandidateNotes {
		offered := &r.CandidateNotes[i]
		fmt.Fprintf(out, "  %s  reply %s by %s\n", offered.Thread, offered.Reply.ID, offered.Reply.Author)
		fmt.Fprintf(out, "    %s\n", strings.TrimSpace(offered.Reply.Body))
		fmt.Fprintf(out, "    record with: %s\n", offered.Record)
	}
}

// resolvedState names §3.5.1's resolution state, which is recorded and never
// used to drop a thread.
func resolvedState(resolved bool) string {
	if resolved {
		return ", resolved"
	}
	return ""
}

// axes is §3.7.6: the active, disabled, and unavailable axes with their reasons
// per §4.5.
//
// The reasons are the disclosures of §4.5.4, printed whole. §11.1 exempts them
// from `--quiet`, and a reader told an axis is off without being told why has
// been told the count and not the fact.
func (r *briefResult) axes(w *writer, out *strings.Builder) {
	fmt.Fprintf(out, "\n%s active: %s\n", w.accent("axes"), listed(r.Axes.Active))
	out.WriteString(w.disclose("  ", "\n", r.Honesty[r.closed:len(r.Honesty)-r.links]...))
	if len(r.Honesty)-r.links == r.closed {
		fmt.Fprintf(out, "  every axis of §1.5 ran; nothing was disabled or unavailable\n")
	}
	// §4.5.1's role half, settled by the same two facts and read by §4.5.6
	// as the set a cell's role must sit in. It is printed here rather than
	// left to the JSON payload alone: a reader of the terminal is
	// otherwise told which lenses can look and not which reviewers do.
	fmt.Fprintf(out, "  roles active: %s\n", listed(r.ActiveRoles))
}

// listed renders a set of ids, and says so when there are none rather than
// printing an empty line a reader would have to interpret.
func listed(ids []string) string {
	if len(ids) == 0 {
		return "none"
	}
	return strings.Join(ids, ", ")
}

// newBriefCmd assembles and prints the orientation payload of §3.7, and records
// the derived inputs the rest of cr reads as authoritative.
//
// The command itself resolves nothing: it names the pull request, the state
// root, the configuration of §2.7, the repository directory, and §3.1's issue
// source, and hands all five to internal/brief. §3.7's six items and the writes
// that go with them are one act, and splitting them across a command and a
// package would put half of §3.7 in a file that also parses flags.
func newBriefCmd(out *writer) *cobra.Command {
	var issue, intentFile string
	var intentExtra []string

	cmd := &cobra.Command{
		Use:   "brief " + prPlaceholder,
		Short: "Print the orientation payload for a pull request",
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
			resolved, err := config.Resolve(config.Sources{
				Environ:      os.Environ(),
				GlobalConfig: layout.Config(),
				RepoConfig:   layout.RepoConfig(owner, repo),
			})
			if err != nil {
				return err
			}
			dir, err := repoDir()
			if err != nil {
				return err
			}
			// §3.1.4's file bypasses the tracker command rather than
			// outranking it, which intentSource is where cr settles.
			source, err := intentSource(layout, owner, repo, intentFile, intentExtra)
			if err != nil {
				return err
			}
			assembled, err := brief.Run(&brief.Sources{
				Layout:    layout,
				GH:        ghClient(),
				Config:    resolved,
				Owner:     owner,
				Repo:      repo,
				PR:        pr,
				RepoDir:   dir,
				IssueFlag: issue,
				Intent:    source,
			})
			if err != nil {
				return headNotFetched(cmd, owner, repo, pr, unsettledBrief(err))
			}
			return out.emit(newBriefResult(assembled))
		},
	}
	cmd.Flags().StringVar(&issue, "issue", "",
		"the issue key, which §3.2 consults before the branch, title, and body")
	intentFlags(cmd, &intentFile, &intentExtra)

	return cmd
}

// unsettledBrief reports brief.UnsettledPostError as §8.4.4's
// UnresolvedPostError, so a brief refused over an unsettled send takes the code
// and the hint `cr post --confirm` and `cr draft` take for the same round: exit
// 4, and `cr post --reconcile` named as the way forward. The brief's own words,
// which name both heads per §9.3.1, go in front. Every other error is returned
// as it came.
func unsettledBrief(err error) error {
	var unsettled *brief.UnsettledPostError
	if !errors.As(err, &unsettled) {
		return err
	}
	return fmt.Errorf("%s: %w", unsettled.Error(), &UnresolvedPostError{
		Owner: unsettled.Owner, Repo: unsettled.Repo, PR: unsettled.PR, Round: unsettled.Round,
	})
}
