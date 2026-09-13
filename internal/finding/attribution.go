package finding

import (
	"encoding/json"
	"fmt"
)

// ruleField is §6.1's row for the rule id, and the key §2.6.1.3 keeps off a
// citation. It is spelled once so the record's row and the citation's refusal
// cannot come to name different things.
const ruleField = "rule"

// ruleAttributed reports whether a record says on its own face that a rule of
// §2.6 produced it.
//
// Two fields say so, and both are read here rather than one, because they say
// it about the same record for the same reason. §2.6.2.4 makes
// `suggestion_origin: rule` the mark of a suggestion a rule's `fix` produced,
// and §6.2.5 makes `origin: rule` the mark of a citation cr matched against its
// own detection output. Either one means the record has a rule behind it, and
// §2.6 item 3 then requires the id.
//
// Only the first of them can be true when a record arrives. §6.1.4 rejects a
// citation supplied with any `origin` at all, so the second becomes true only
// once §6.2.5's stamping has run, which is citation-origin-stamping's. Reading
// both here is what keeps that from needing a second reading of §2.6 item 3
// written somewhere else.
func ruleAttributed(record *Finding) bool {
	if record.SuggestionOrigin == OriginRule {
		return true
	}
	for at := range record.Citations {
		if record.Citations[at].Origin == OriginRule {
			return true
		}
	}
	return false
}

// ruleAttribution holds one record to §2.6 item 3: a record a rule produced
// carries the rule id.
//
// The refusal is a RejectedRecordError, the shape §6.1.3 gives every record
// fault and the one internal/cli maps onto §11.2's code 1. Nothing about the
// file failed and the command line was right; what is wrong is the record's own
// content, which claims a rule stands behind it and does not say which.
//
// An id nothing produced is not refused here, and cannot be. §6.2.5 is what
// answers a record naming a rule it never matched, by stamping `origin` from
// `rule-stats.ndjson` positionally rather than from the id the record supplied
// — so a forged id buys nothing rather than being caught. This item is the
// other direction: a record with a rule behind it and no id names none, and
// §7.3.2's per-rule statistics and §2.6.3.4's dead-rule report both count by id.
func (c checker) ruleAttribution(line int, supplied map[string]json.RawMessage, record *Finding) error {
	if err := c.citationCarriesNoRule(line, supplied[citationsField]); err != nil {
		return err
	}
	if !ruleAttributed(record) || written(supplied[ruleField]) {
		return nil
	}
	return &RejectedRecordError{
		File: c.file, Line: line, Field: ruleField,
		Problem: "is required by §2.6 item 3 of a record a rule produced, and this record " +
			"carries the marks of one without naming it",
	}
}

// citationsField is §6.1's row holding the citations array.
const citationsField = "citations"

// citationCarriesNoRule holds each citations entry to §2.6.1.3's separation:
// the rule id is carried on the record, and a citation carries the matched
// path, line, computed hash and computed origin and nothing else.
//
// Dropping the key silently would satisfy the section's letter — nothing in cr
// would ever read it — and lose what the section is for. §6.2.5 matches a
// citation to a hit on head, rule id, path and line together, and the id in
// that match is the record's; an id sitting inside the entry is a second copy
// of one quarter of the match, able to disagree with the record carrying it and
// invisible to anyone reading either. So the entry is refused rather than
// quietly trimmed, and the author is told which of the two places the id
// belongs in.
//
// The entry is named by its index, as checker.computedCitations names one, so a
// record carrying several citations still points at the one at fault.
func (c checker) citationCarriesNoRule(line int, citations json.RawMessage) error {
	entries, err := c.citationEntries(line, citations)
	if err != nil {
		return err
	}
	for at, entry := range entries {
		if _, carried := entry[ruleField]; !carried {
			continue
		}
		return &RejectedRecordError{
			File: c.file, Line: line,
			Field:   fmt.Sprintf("citations[%d].%s", at, ruleField),
			Problem: "is not a citation's to carry: §2.6.1.3 puts the rule id on the record",
		}
	}
	return nil
}
