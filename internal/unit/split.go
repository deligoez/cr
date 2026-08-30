package unit

import "github.com/deligoez/cr/internal/git"

// Split applies §3.4.5's cap to the clusters §3.4.4 formed, returning the
// units the cap leaves behind in the order their clusters arrived.
//
// maxLines is `cluster.max_lines`. A cluster over the cap is split by taking
// its hunks in ascending start order and opening a new unit whenever adding
// the next hunk would exceed the limit, so a unit is closed by the hunk that
// would not fit and never by one that would.
//
// The hunks are taken in the order the cluster holds them, and that order
// already is ascending start order rather than being made so here: §3.4.4
// partitions one file's hunks on one side, git emits a file's hunks in
// ascending order, and every branch of §3.4.4 keeps the order of the partition
// it selects from. Sorting again would be a comparator whose result no input
// could differ from, which is a second answer to the question of what
// ascending means and one nothing would ever check.
//
// A hunk that alone exceeds the cap becomes one unit and is flagged
// `oversized`: it is the one place the limit does not bind, because there is
// nothing below a hunk to split. Splitting inside one would cut a unit at a
// line the diff never marked, and §3.4.6's hunk ranges would then name a span
// no hunk has.
func Split(clusters []Cluster, maxLines int) []Cluster {
	split := make([]Cluster, 0, len(clusters))
	for _, cluster := range clusters {
		split = append(split, cluster.split(maxLines)...)
	}
	return split
}

// split takes §3.4.5's cap to one cluster. A cluster already inside the cap
// runs the same loop as one over it and comes back out whole, so there is no
// branch on whether the cap binds — a cluster at exactly the cap is the case
// such a branch would be written wrong at, and here it is not a case at all.
func (c Cluster) split(maxLines int) []Cluster {
	units := make([]Cluster, 0, len(c.Hunks))
	var current []git.Hunk
	count := 0
	for _, hunk := range c.Hunks {
		lines := len(hunk.Changed)
		if len(current) > 0 && count+lines > maxLines {
			units, current, count = append(units, c.unit(current, false)), nil, 0
		}
		if lines > maxLines {
			units = append(units, c.unit([]git.Hunk{hunk}, true))
			continue
		}
		current, count = append(current, hunk), count+lines
	}
	if len(current) > 0 {
		units = append(units, c.unit(current, false))
	}
	return units
}

// unit records one of the clusters the split leaves. Path, Side and Formation
// come from the cluster being split, because §3.4.5 divides a cluster and does
// not re-form it: the branch of §3.4.4 that gathered these hunks is still the
// branch that gathered them, and §3.4.6 records that branch.
func (c Cluster) unit(hunks []git.Hunk, oversized bool) Cluster {
	return Cluster{
		Path:      c.Path,
		Side:      c.Side,
		Formation: c.Formation,
		Hunks:     hunks,
		Oversized: oversized,
	}
}
