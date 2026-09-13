package cli

import (
	"fmt"

	"github.com/deligoez/cr/internal/finding"
	"github.com/deligoez/cr/internal/state"
)

// refuseRuleClasses holds every record naming a rule to §2.6's class row: the
// class of a record a rule produced is that rule's class.
//
// cr refuses a record whose class differs from its rule's, with exit code 1
// naming the line, rather than assigning the rule's class over the one the
// record carries. §6.1 makes both fields the agent's — `class` required and
// `rule` optional, neither computed — so a disagreement between them says one
// of the two is wrong without saying which. Assigning would decide the rule id
// is the right one and silently rewrite a field §6.1.4 leaves the agent to
// write, which is a judgement P5 keeps out of cr. Left unchecked, the record
// would split §6.4.1's dedup key and §7.4.1's waiver key away from every other
// record of the same rule.
//
// A record naming a rule the round's corpus does not hold is not checked here:
// there is no class to compare it with, and finding.ruleAttribution says why
// such an id is not refused. The corpus is resolved only when some record names
// a rule, so a round whose records name none reads no rule file.
//
// It runs where every refusal that names an input line runs, before §6.4.4's
// drops shorten the slice the line numbers index.
func refuseRuleClasses(
	l state.Layout, owner, repo, profileID, file string, body []byte, records []*finding.Finding,
) error {
	named := false
	for _, record := range records {
		named = named || record.Rule != ""
	}
	if !named {
		return nil
	}
	corpus, err := roundCorpus(l, owner, repo, profileID)
	if err != nil {
		return err
	}
	classes := make(map[string]string, len(corpus))
	for i := range corpus {
		classes[corpus[i].Rule.ID] = corpus[i].Rule.Class
	}
	at := state.RecordLines(body)
	for i, record := range records {
		class, held := classes[record.Rule]
		if !held || record.Class == class {
			continue
		}
		return &finding.RejectedRecordError{
			File: file, Line: at[i], Field: "class",
			Problem: fmt.Sprintf(
				"reads %q, and record %s names rule %q, whose class is %q; §2.6's class row gives a record "+
					"its rule's class, so write %q or name the rule that produced the record",
				record.Class, record.ID, record.Rule, class, class),
		}
	}
	return nil
}
