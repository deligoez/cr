package cli

import (
	"fmt"
	"strconv"
	"strings"

	"github.com/spf13/cobra"

	"github.com/deligoez/cr/internal/axis"
	"github.com/deligoez/cr/internal/finding"
	"github.com/deligoez/cr/internal/role"
	"github.com/deligoez/cr/internal/rule"
	"github.com/deligoez/cr/internal/state"
	"github.com/deligoez/cr/internal/text"
)

// mergeResult is what `cr merge` has to report about the file it wrote: how
// many records reached it, and what each of §6.5.1's passes took out on the
// way.
//
// The drops are reported apart rather than counted into one number. §6.4.4 and
// §9.3.6 remove a finding for different reasons — one the reviewer set aside,
// one the author has already read — and a reviewer told only that some findings
// went missing could not tell a round that repeated itself from a round whose
// judgements were being honoured.
//
// The counts by role, axis, severity and grade are §6.5.1's other half. They
// are four cuts of one set rather than four measurements — the records the
// output file holds, counted four ways — so each cut sums to Merged, and a
// reader who doubts one can check it against the others and against the file.
type mergeResult struct {
	// Output is the path `-o` named, so the caller can hand it to
	// `cr record` without reconstructing it.
	Output string `json:"output"`
	// Merged is how many records the file holds.
	Merged int `json:"merged"`
	// Waived is §6.4.4's report: how many findings an active waiver
	// silenced, and which waivers silenced them.
	Waived finding.Drops `json:"waived"`
	// AlreadyPosted is §9.3.6's report: how many findings the posted index
	// already holds an entry for, and which records hold them.
	AlreadyPosted finding.PostedDrops `json:"already_posted"`
	// Overlaps is §6.4.3's overlap summary over the groups §6.4.1 formed.
	Overlaps finding.Overlaps `json:"overlaps"`
	// Counts is §6.5.1's four breakdowns over the records the file holds.
	Counts mergeCounts `json:"counts"`
	// Honesty carries the three counts above as the sentences §11.1
	// exempts from `--quiet`.
	Honesty []string `json:"honesty"`
}

// Text names the file and how many records reached it, then §6.5.1's four
// breakdowns, then the three disclosures. The records themselves are in the
// file the line names, so printing them into a terminal would repeat what the
// caller can already read.
func (r *mergeResult) Text(w *writer) string {
	var out strings.Builder
	fmt.Fprintf(&out, "merged %s record(s) into %s", w.accent(strconv.Itoa(r.Merged)), r.Output)
	for _, line := range r.Counts.lines() {
		fmt.Fprintf(&out, "\n%s", line)
	}
	out.WriteString(w.disclose("\n", "", r.Honesty...))
	return out.String()
}

// newMergeResult reports one merge, asking each pass for its own sentence.
//
// The disclosures are asked of the types that own them rather than assembled
// here, as `cr record` asks its own: a wording built at the call site cannot
// come to disagree with the data beside it.
func newMergeResult(output string, merged *mergeOutcome) *mergeResult {
	disclosures := []finding.HonestyDisclosure{merged.waived, merged.posted, merged.overlaps}
	honesty := make([]string, 0, len(disclosures))
	for _, disclosed := range disclosures {
		honesty = append(honesty, disclosed.Disclosure())
	}
	return &mergeResult{
		Output: output, Merged: len(merged.records),
		Waived: merged.waived, AlreadyPosted: merged.posted, Overlaps: merged.overlaps,
		Counts: merged.counts, Honesty: honesty,
	}
}

// newMergeCmd registers §11's
// `cr merge <files...> -o <out> --repo <owner/repo> --pr <n>`, the merge of
// §6.5.
//
// It takes the pull request as a flag rather than as the positional every other
// PR-scoped command uses, because its positionals are the files being merged
// and §6.5.1 states the invocation that way. The repository and the pull
// request are both required there for a reason that is not bookkeeping: §6.4.2
// needs the resolved role corpus and §6.4.4 the repository-wide waivers, and
// §7.4.1 scopes `not-here` waivers to the pull request — a merge that could not
// read them would resurface exactly what the reviewer set aside. `--repo` is
// §11.1's global flag and is registered on the root command.
func newMergeCmd(out *writer) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "merge <files...>",
		Short: "Merge and deduplicate per-role findings",
		Args:  cobra.MinimumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			return runMerge(cmd, out, args)
		},
	}
	cmd.Flags().StringP("output", "o", "", "write the merged findings to this file")
	cmd.Flags().Int("pr", 0, "the pull request the findings belong to")
	_ = cmd.MarkFlagRequired("output")
	_ = cmd.MarkFlagRequired("pr")
	return cmd
}

// runMerge is §6.5.1: the per-role files are read, the four passes are applied,
// and the result is written where `-o` points.
//
// Nothing is written until every input has been read and every pass has run,
// which is the contract `cr record` states for the same reason: §6.5.2 rejects
// a record with exit code 1, and a rejection that had already written half the
// output would leave the agent a file it is about to hand back corrected.
func runMerge(cmd *cobra.Command, out *writer, files []string) error {
	pr, err := prFlagOf(cmd)
	if err != nil {
		return err
	}
	owner, repo, err := repoOf(cmd)
	if err != nil {
		return err
	}
	output, err := cmd.Flags().GetString("output")
	if err != nil {
		return err
	}
	layout, err := state.Default()
	if err != nil {
		return err
	}
	// Briefed rather than ReadMeta: §6.1.3 checks every record's unit
	// against the units of the round, and §6.2 grades against the head that
	// round was opened at.
	round, err := briefedRound(layout, owner, repo, pr)
	if err != nil {
		return err
	}
	// §9.3.2. §6.5.1 has `cr merge` write its drop counts into the current
	// round's `summary.json`, which §2.3's table lists as per-PR state, so
	// this command is one of §9.3.2's writers whichever task lands that
	// write. It would be refused here for a second reason even without it:
	// what it produces is the input to `cr record`, which §9.3.2 refuses at
	// a moved head — so a merge that ran could only manufacture a file
	// nothing downstream is allowed to accept.
	if err := round.RefuseStale(); err != nil {
		return err
	}
	merged, err := mergeRecords(layout, owner, repo, pr, &round, files)
	if err != nil {
		return err
	}
	body, err := finding.MergedRecords(merged.records)
	if err != nil {
		return err
	}
	// §2.2 keeps cr's own state under `~/.cr`. This file is not cr's state:
	// it is the caller's, written where the caller pointed, and it is the
	// one thing `cr merge` produces. It still goes through internal/state,
	// which is where every write cr performs is spelled.
	if err := state.WriteNamedFile(output, body); err != nil {
		return err
	}
	// §6.5.1's drop counts and §10.3's raised count, into the round's
	// summary.json. They are written after the output file, so the summary
	// describes a merge that produced one rather than a merge that was
	// about to.
	if err := writeMergeCounts(layout, owner, repo, pr, round.Round, merged, body); err != nil {
		return err
	}
	return out.emit(newMergeResult(output, merged))
}

// summaryAlreadyPosted is §9.3.6's drop count, by the key
// `rounds/<n>/summary.json` holds it under. §6.5.1 makes `cr merge` its author.
//
// It is a key of its own rather than a number added into summaryWaived. §6.4.4
// and §9.3.6 remove a finding for different reasons — one the reviewer set
// aside, one the author has already read — and §10.3 asks for a history that
// can be reconstructed from state alone, which a single suppression count would
// not give: a reader could not tell a round that repeated itself from a round
// whose judgements were being honoured.
const summaryAlreadyPosted = "already_posted"

// summaryMergedHash is the key the round summary holds mergedDigest of the file
// `cr merge` last wrote under.
//
// It is how `cr record` answers §6.5.1's question — is this file `cr merge`'s
// output — without taking the answer from the agent. §6.1.4 lets `duplicate_of`
// in on that output alone, and neither the command's entry point nor the
// file's name can tell it from a file the agent wrote: `-o` points anywhere,
// and the agent names what it hands `cr record`. The digest is the merge's own
// statement of what it produced, in the document §10.3 has it create, and like
// every other count of its section it is replaced whole by the next run, so a
// file an earlier merge of the round wrote no longer matches.
const summaryMergedHash = "merged_hash"

// mergedDigest is the digest summaryMergedHash holds: §1.4's normalised hash of
// the file's text, the one hash cr computes anywhere. Normalising first means
// an edit that only moves whitespace keeps the digest, and none such can change
// which record `duplicate_of` names.
func mergedDigest(body []byte) (string, error) {
	return text.NormalisedHash(string(body))
}

// writeMergeCounts puts `cr merge`'s share of §10.3's counts into the round's
// summary.json, which §10.3 has this command create, beside the digest of the
// output file body holds.
//
// The three are the head of §10.3's list: what the roles raised, and the two
// passes that took findings back out before anything was recorded. Every later
// count in the document is read against them.
//
// They are written on every run, at zero as well, for the reason
// finding.Drops.Disclosure is printed at zero: §10.1.6 reports an absent count
// as "the merge has not run for this round", which is a different fact from a
// merge that ran and matched nothing.
func writeMergeCounts(
	l state.Layout, owner, repo string, pr, round int, merged *mergeOutcome, body []byte,
) error {
	digest, err := mergedDigest(body)
	if err != nil {
		return err
	}
	held, err := l.LockPR(owner, repo, pr)
	if err != nil {
		return err
	}
	intake, keys := newMergeIntake(merged)
	if err := writeMergeIntake(held, l, owner, repo, pr, round, intake, keys, digest); err != nil {
		// The lock is released on the way out of every branch, and the
		// write's own failure is what the caller is told about.
		_ = held.Unlock()
		return err
	}
	return held.Unlock()
}

// writeMergeIntake is writeMergeCounts under the lock: the merge's keys into
// intake.json first, then its own share and digest into the summary, and then
// ownerIntake's counts over that share, the shares `cr record` kept, and the
// round's stored records, read again under the lock.
func writeMergeIntake(
	held *state.Lock, l state.Layout, owner, repo string, pr, round int, intake *mergeIntake, keys intakeKeys,
	digest string,
) error {
	if err := state.UpdateRoundSection(held, round, state.FileIntake, summaryMergeIntake, keys); err != nil {
		return err
	}
	if err := writeSummary(held, round, ownerMerge, []summaryCount{
		{key: summaryMergeIntake, value: intake},
		{key: summaryMergedHash, value: digest},
	}); err != nil {
		return err
	}
	in, err := readIntake(l, owner, repo, pr, round)
	if err != nil {
		return err
	}
	stored, err := roundFindingsOf(l, owner, repo, pr, round)
	if err != nil {
		return err
	}
	return writeIntakeCounts(held, round, in, stored)
}

// mergeOutcome is what §6.5.1's four passes left: the records that reach the
// output file, and what each drop took out of them.
type mergeOutcome struct {
	records []*finding.Finding
	// raised is how many records the per-role files held, before any of
	// §6.5.1's passes removed one: §10.3's `raised`.
	raised int
	waived finding.Drops
	posted finding.PostedDrops
	// waivedKeys and postedKeys are the intake keys of the records each
	// drop took out, which the round summary counts them by.
	waivedKeys []intakeKey
	postedKeys []intakeKey
	overlaps   finding.Overlaps
	counts     mergeCounts
}

// mergeRecords reads every per-role file and applies §6.5.1's four passes in
// the order §6.5.1 lists them.
//
// The order is the contract rather than the shape a function body happened to
// take. The grade is computed first because §6.4.2 ranks a duplicate group by
// it. §6.4.4 and §9.3.6 drop before §6.4.3 marks, because MarkDuplicates writes
// a representative's id into every other record of its group, and dropping a
// representative afterwards would leave those records naming an id nothing in
// the round holds.
func mergeRecords(
	l state.Layout, owner, repo string, pr int, round *state.Round, files []string,
) (*mergeOutcome, error) {
	formed, err := roundUnitsOf(l, owner, repo, pr, round.Round)
	if err != nil {
		return nil, err
	}
	records, err := readPerRole(l, owner, repo, pr, &round.Meta, files, formed)
	if err != nil {
		return nil, err
	}
	corpus, err := role.Resolve(l.RepoRolesDir(owner, repo), l.RolesDir())
	if err != nil {
		return nil, err
	}
	if err := gradeMerged(l, owner, repo, pr, round, formed, corpus, records); err != nil {
		return nil, err
	}
	// §6.4.4, over both of the files §7.4.4 writes. §7.4.1's key reads
	// each record's anchored lines out of the round's trees.
	trees := keyTrees(owner, repo, pr, round.Head)
	waivers, err := finding.ActiveWaivers(l, owner, repo, pr)
	if err != nil {
		return nil, err
	}
	unwaived, waived, err := finding.DropWaived(trees, records, waivers)
	if err != nil {
		return nil, err
	}
	// §9.3.6, read across rounds because that is the whole of what the
	// index is for: a comment the author received in round 1 must not be
	// raised again in round 2, and an index narrowed to the current round
	// would always be empty at the moment this consults it.
	index, err := finding.PostedIndex(l, owner, repo, pr)
	if err != nil {
		return nil, err
	}
	kept, posted, err := finding.DropPosted(trees, unwaived, index)
	if err != nil {
		return nil, err
	}
	// §6.4.1 and §6.4.2, and the `duplicate_of` §6.5.1 lets this command
	// write. The state §6.4.3 names is `cr record`'s to stamp.
	return &mergeOutcome{
		records:    kept,
		raised:     len(records),
		waived:     waived,
		posted:     posted,
		waivedKeys: droppedKeys(records, unwaived),
		postedKeys: droppedKeys(unwaived, kept),
		overlaps:   finding.MarkDuplicates(kept, role.Order(corpus)),
		// §6.5.1's four breakdowns, over the records that reach the
		// output file and after every pass that could remove one.
		counts: mergeCountsOf(corpus, kept),
	}, nil
}

// mergeCounts is §6.5.1's report over one merge: the records the output file
// holds, counted by role, by axis, by severity and by grade.
//
// Each cut gives a row to every value of its vocabulary, at zero as well, and a
// row of its own to a value the vocabulary does not list. That is what makes
// the four sums equal — they are one set counted four ways — so a reader can
// check any cut against the merged count and against the file.
//
// The grade is the one of the four that exists only here. §6.5.1 has `cr merge`
// compute it in memory for these counts and keeps it out of the output, so this
// is the only place the round's grades are visible before `cr record` stamps
// them.
type mergeCounts struct {
	// ByRole counts §2.5.5's corpus order, which is the order §6.4.2
	// picks a duplicate group's representative in.
	ByRole []tally `json:"by_role"`
	// ByAxis counts §1.5's four, in that section's order.
	ByAxis []tally `json:"by_axis"`
	// BySeverity counts §6.1's four, highest first.
	BySeverity []tally `json:"by_severity"`
	// ByGrade counts §6.2's three, strongest first.
	ByGrade []tally `json:"by_grade"`
}

// mergeCountsOf counts §6.5.1's four dimensions over the merged records.
//
// The role vocabulary is the resolved corpus rather than the roles that filed
// something, so a role that looked and found nothing is a row at zero instead
// of an absence — which is the difference between a round in which a lens
// reported nothing and a round in which it never ran.
func mergeCountsOf(corpus []role.Resolved, records []*finding.Finding) mergeCounts {
	roles := make([]string, 0, len(corpus))
	for _, resolved := range corpus {
		roles = append(roles, resolved.Role.ID)
	}
	filed := make([]string, 0, len(records))
	axes := make([]string, 0, len(records))
	severities := make([]string, 0, len(records))
	grades := make([]string, 0, len(records))
	for _, record := range records {
		filed = append(filed, record.Role)
		axes = append(axes, record.Axis)
		severities = append(severities, string(record.Severity))
		grades = append(grades, string(record.Grade))
	}
	return mergeCounts{
		ByRole:     tallyOf(roles, filed),
		ByAxis:     tallyOf(axis.IDs(), axes),
		BySeverity: tallyOf(namesOf(finding.Severities()), severities),
		ByGrade:    tallyOf(namesOf(finding.Grades()), grades),
	}
}

// lines renders the four breakdowns for a terminal, one dimension to a line, in
// the order §6.5.1 names them.
func (c *mergeCounts) lines() []string {
	return []string{
		tallyLine("role", c.ByRole),
		tallyLine("axis", c.ByAxis),
		tallyLine("severity", c.BySeverity),
		tallyLine("grade", c.ByGrade),
	}
}

// readPerRole reads every §4.6.2 fan-out output file the caller named, in the
// order they were named, and holds each to §6.1.3, §6.1.4 and §6.2.3.
//
// Every input is one role's own output, so finding.DecodePerRole refuses a name
// that binds its records to no role: §6.1.3 rejects a record whose `role` is
// not the role whose file it arrived in, and a file carrying no role in its
// name leaves nothing to compare against — the whole file's `role` fields, and
// with them the axis §6.2 grades on, would revert to being taken on the agent's
// word.
//
// The paths are the caller's and are not derived here, which settles the open
// question review-prompt-emission left: `cr review` writes those files under
// state.DirFanOut — `fanout/<round>/<unit>/review-<role>.ndjson` inside the
// pull request's state directory, beside §2.3's table rather than inside
// `rounds/<n>/`, because `rounds/<n>/` holds the four artefacts §2.3 names and
// cr writes, while these are files a role writes. `cr merge` is told which
// files to read and binds each one's records by its base name alone, so a role
// that wrote elsewhere is merged exactly as one that did not, and the directory
// is `cr review`'s to fix rather than a second decision made here.
//
// The citations are resolved here, beside the decode, because §6.2.3's
// rejection names the line of the file the entry arrived in and the body is
// what counts those lines. Resolving them at all is what makes the grade
// §6.4.2 ranks by a real answer: without the hash cr stamps, §6.2's `cited` row
// can never match, every record would grade `argued`, and "the highest grade"
// would silently stop distinguishing anything.
//
// Every anchor is bound to the unit its record names, and a LEFT one to lines
// the diff removed, by the check `cr record` makes, refuseForeignAnchors, so
// an anchor that refusal would meet at `cr record` is refused here, naming the
// line of the role's own file, before any output exists to hand on.
//
// The anchors are stamped here too, for the reason the citations are: §6.4.4
// and §9.3.6 match on §7.4.1's key, which hashes the anchor's context window
// with its lines at the head. A record whose window the role typed, or left
// out, would match no waiver or posted-index entry, and an already-posted
// finding would reach the draft again.
//
// The ids are held to §6.1 last, across every input and against the pull
// request's stored records, through the check `cr record` makes: two roles'
// files both carrying f1 would otherwise merge into a file where
// `duplicate_of` can name an id two records hold, and a refusal here comes
// before the output file and summary.json are written.
func readPerRole(
	l state.Layout, owner, repo string, pr int, round *state.Meta, files []string, formed []roundUnit,
) ([]*finding.Finding, error) {
	units := roundUnitIDs(formed)
	records := make([]*finding.Finding, 0)
	inputs := make([]idInput, 0, len(files))
	for _, file := range files {
		body, err := readInput(file,
			"§4.6 has each role write its findings to the output path "+
				"`cr review` printed; pass those paths")
		if err != nil {
			return nil, err
		}
		read, err := finding.DecodePerRole(file, body, units)
		if err != nil {
			return nil, err
		}
		if err := refuseForeignAnchors(owner, repo, pr, round, file, body, formed, read); err != nil {
			return nil, err
		}
		if err := stampAnchors(owner, repo, pr, round, file, body, read); err != nil {
			return nil, err
		}
		if err := resolveCitations(file, body, round.Head, read); err != nil {
			return nil, err
		}
		records = append(records, read...)
		inputs = append(inputs, idInput{file: file, body: body, records: read})
	}
	if err := refuseHeldIDs(l, owner, repo, pr, round.Round, units, inputs); err != nil {
		return nil, err
	}
	return records, nil
}

// gradeMerged computes §6.2's grade over the merged records, in memory.
//
// §6.5.1 says why it is computed at all and how far it goes: `cr merge`
// computes the grade for its counts, and its output carries no computed field
// except `duplicate_of`. §6.4.2 is the second reader — a duplicate group's
// representative is the highest grade before it is anything else — and both
// readers are inside this command. finding.MergedRecords takes the field back
// out on the way to the file.
//
// It is the same computation `cr record` makes, through the same helpers, so
// the representative this merge picks is the record `cr record` will grade
// highest. Two spellings of §6.2 that could disagree is exactly what one shared
// path prevents.
func gradeMerged(
	l state.Layout, owner, repo string, pr int, round *state.Round,
	formed []roundUnit, corpus []role.Resolved, records []*finding.Finding,
) error {
	// §6.2.5, before the grade reads it: a citation cr's own rule machinery
	// produced counts as evidence even inside the record's own unit, and
	// §6.1.4 refuses the field on the wire.
	ledger, err := rule.ReadStats(l, owner, repo)
	if err != nil {
		return err
	}
	rule.StampOrigins(ledger, round.Head, records)
	evidence, err := readRoundEvidence(l, owner, repo, pr)
	if err != nil {
		return err
	}
	// §6.1's `axis` row, before the grade rather than after it: §6.2's
	// `cited` row reads the axis, and §4.4.2 withholds that grade from the
	// test axis entirely.
	stampAxes(corpus, records)
	gradeRecords(&round.Meta, formed, evidence, records)
	return nil
}

// prFlagOf reads the pull request `cr merge` takes as a flag.
//
// It applies parsePR's rule rather than a second one. §6.5.1 spells the pull
// request as `--pr <number>` because this command's positionals are the files
// being merged, and a number that is not a pull request is the same fault
// whichever way it was typed — so it is refused with the same sentence.
func prFlagOf(cmd *cobra.Command) (int, error) {
	n, err := cmd.Flags().GetInt("pr")
	if err != nil {
		return 0, err
	}
	return parsePR(strconv.Itoa(n))
}
