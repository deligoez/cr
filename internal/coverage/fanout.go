package coverage

import "slices"

// fanOutPrefix and fanOutSuffix are the name §4.6.2 gives the file one role
// writes its coverage cell for one unit to, beside the records and proposals
// finding.FanOutFile and proposal.FanOutFile name.
//
// Found on tarfin-labs/backend#6328 with cr 0.13.0: every prompt named the
// path for records and for proposals and none for the cell.
const (
	fanOutPrefix = "cells-"
	fanOutSuffix = ".ndjson"
)

// FanOutFile is the file §4.6.2 has one role write its coverage cell to.
func FanOutFile(role string) string {
	return fanOutPrefix + role + fanOutSuffix
}

// Fields is §4.5.5's fields an agent writes, in the order the decoder lists
// them when it refuses one it does not know.
func Fields() []string {
	return slices.Clone(cellFields)
}

// Results is §4.5.5's closed set of results, in the order it names them.
func Results() []string {
	return slices.Clone(results)
}
