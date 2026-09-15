package cli

import (
	"fmt"
	"slices"
	"strings"

	"github.com/deligoez/cr/internal/finding"
	"github.com/deligoez/cr/internal/role"
	"github.com/deligoez/cr/internal/state"
)

// classOutside is one record of §2.5.6's report: a record whose role declares
// `classes` and whose `class` is not one of them.
type classOutside struct {
	Record string `json:"record"`
	Role   string `json:"role"`
	Class  string `json:"class"`
}

// classesOutside is §2.5.6 over the records one `cr record` stored, in the
// order they were stored. It reports and never refuses: the vocabulary is a
// role's, and a class outside it is still a class §6.1 accepts. A role that
// declares no `classes`, or that the corpus no longer resolves, reports
// nothing.
func classesOutside(l state.Layout, owner, repo string, records []*finding.Finding) ([]classOutside, error) {
	corpus, err := role.Resolve(l.RepoRolesDir(owner, repo), l.RolesDir())
	if err != nil {
		return nil, err
	}
	outside := make([]classOutside, 0)
	for _, record := range records {
		at := slices.IndexFunc(corpus, func(resolved role.Resolved) bool { return resolved.Role.ID == record.Role })
		if at < 0 || len(corpus[at].Role.Classes) == 0 || slices.Contains(corpus[at].Role.Classes, record.Class) {
			continue
		}
		outside = append(outside, classOutside{Record: record.ID, Role: record.Role, Class: record.Class})
	}
	return outside, nil
}

// classesOutsideLines is the report at a terminal, one line per record, each
// line opened by before.
func classesOutsideLines(before string, outside []classOutside) string {
	var out strings.Builder
	for _, entry := range outside {
		fmt.Fprintf(&out, "%s%s has class %s, which is not in role %s's classes (§2.5.6); it was recorded",
			before, entry.Record, entry.Class, entry.Role)
	}
	return out.String()
}
