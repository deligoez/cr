package cli

import (
	"github.com/deligoez/cr/internal/finding"
	"github.com/deligoez/cr/internal/git"
	"github.com/deligoez/cr/internal/rule"
	"github.com/deligoez/cr/internal/state"
	"github.com/deligoez/cr/internal/unit"
)

// suggestRuleFixes is §2.6.2.1's generation, applied to the records `cr record`
// has accepted and before it stores them.
//
// `cr record` is the moment §2.6.2 describes. §2.6.1.5 has a hit reach nothing
// until the agent confirms it, and a record naming the rule is what that
// confirmation looks like, so this is the first point at which cr knows which
// hits a human stands behind. It is also before drafting, which is where
// §2.6.2.2 puts the validation: a suggestion the draft rendered and the post
// refused would be a replacement the author was shown and never offered.
//
// Nothing here is written for a record that has none of it. A round whose
// records name no rule, and a corpus whose named rules carry no `fix` block,
// both return before the diff is read — so the command asks GitHub and git for
// nothing when there is nothing to generate.
func suggestRuleFixes(
	l state.Layout, owner, repo string, pr int, round *state.Meta, records []*finding.Finding,
) error {
	confirmed := rulesNamedBy(records)
	if len(confirmed) == 0 {
		return nil
	}
	matchers, err := roundMatchers(l, owner, repo, round.ProfileID)
	if err != nil {
		return err
	}
	fixing := make([]rule.Matcher, 0, len(matchers))
	for at := range matchers {
		if matchers[at].Fix != nil && confirmed[matchers[at].Rule.ID] {
			fixing = append(fixing, matchers[at])
		}
	}
	if len(fixing) == 0 {
		return nil
	}
	// The same reading `cr rules check` makes, over the same hunks: §2.6.2.1
	// rewrites the line a hit matched, and the matched text is what a rule
	// ledger does not keep. Re-evaluating is pure over the diff, so a hit
	// found here is a hit that command would report at this head — which
	// holds only while the diff is narrowed the way detectRound narrows it,
	// to the files that formed one of the round's units per §2.6.1.1.
	all, err := roundHunks(owner, repo, pr, round.Head)
	if err != nil {
		return err
	}
	formed, err := roundUnitsOf(l, owner, repo, pr, round.Round)
	if err != nil {
		return err
	}
	units := make([]unit.Unit, 0, len(formed))
	for i := range formed {
		units = append(units, formed[i].Unit)
	}
	hunks := rule.Reviewed(all, units)
	hits := rule.Evaluate(fixing, hunks)
	for _, record := range records {
		suggestOneFix(fixing, hits, hunks, record)
	}
	return nil
}

// rulesNamedBy collects the rule ids of the records a generated suggestion
// could reach: one naming a rule, per §2.6 item 3, and carrying no suggestion
// of its own.
//
// A record that already holds one is left out rather than overwritten. §6.1's
// table lets the agent write a suggestion, and §2.6.2.4 says plainly that cr
// cannot establish a generated replacement compiles, parses, or preserves
// behaviour — so replacing text a human wrote with text nothing stands behind
// would spend the author's trust to remove the one suggestion that had an
// author.
func rulesNamedBy(records []*finding.Finding) map[string]bool {
	named := make(map[string]bool)
	for _, record := range records {
		if record.Rule != "" && record.Suggestion == "" {
			named[record.Rule] = true
		}
	}
	return named
}

// suggestOneFix offers the record the fix generated from the hit it confirms,
// and stops at the first suggestion §8.2 admits.
//
// The hit is found through the record's own citations, which is §2.6.1.3's
// form of confirmation: a record naming the rule and citing the matched path
// and line is the agent saying it read that hit. The match is positional for
// §6.2.5's reason — a rule id alone would let a record name any rule and take
// the replacement generated for any line of it.
//
// The range the suggestion would replace is the record's anchor rather than
// the cited line, and Matcher.Suggest holds that range to §8.2 before anything
// is written. So a record citing a hit and anchored across two hunks, or on
// the LEFT side, keeps its finding and loses the replacement.
func suggestOneFix(
	matchers []rule.Matcher, hits []rule.Hit, hunks []git.Hunk, record *finding.Finding,
) {
	if record.Rule == "" || record.Suggestion != "" {
		return
	}
	for _, citation := range record.Citations {
		for h := range hits {
			hit := &hits[h]
			if hit.RuleID != record.Rule || hit.Path != citation.Path || hit.Line != citation.Line {
				continue
			}
			for m := range matchers {
				if matchers[m].Rule.ID == hit.RuleID && matchers[m].Suggest(record, hit, hunks) {
					return
				}
			}
		}
	}
}
