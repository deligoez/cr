package rule

import "github.com/deligoez/cr/internal/finding"

// Citation renders one hit as the `citations` entry §2.6.1.3 requires: the
// matched path, the matched line, and `origin: rule`.
//
// The origin is not a parameter, and that is the point of the method rather
// than an economy of it. §6.2.5 has cr stamp `origin` and rejects a record
// arriving with any value in it, because the field is what buys §6.2's `cited`
// row for a citation inside the record's own unit — a location the record is
// already about, which adds nothing a human could check the summary against
// unless cr's own machinery put it there. An agent that could write `origin`
// could name any rule and point anywhere inside its own unit and be graded as
// though a detector had found it. So the only way to obtain a rule-origin
// citation is to have run §2.6.1.1's detection and be holding its Hit.
//
// No content hash is set. §6.2.3 has cr compute and store that at record time,
// against the head the record is being written for, and §6.2's `cited` row
// reads its presence as the mark of an entry cr actually resolved. Writing one
// here would claim a resolution that has not happened.
//
// The rule id is left out. §2.6 item 3 puts it on the record's own `rule` field,
// and §6.2.5 matches a citation to a hit by head, rule id, path and line
// together — so an id inside the citation would be a second copy of one of the
// four halves of that match, able to disagree with the record carrying it.
func (h *Hit) Citation() finding.Citation {
	return finding.Citation{Path: h.Path, Line: h.Line, Origin: finding.OriginRule}
}
