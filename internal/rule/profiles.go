package rule

import "slices"

// InProfile reports whether the rule applies under one resolved profile, per
// §2.6's `profiles` row: an empty list means all profiles, and any other list
// only the ones it names. The empty id of a round no profile matched (§2.4.4)
// is named by no list, so a scoped rule stays out of such a round.
//
// It is Applies' companion, asked of the same rule for the same reason, and it
// lives in its own file only because detect.go's imports are fenced by
// TestDetectionCanReadNothingButTheDiff. A command that asked one and not the
// other would report hits another command, on the same pull request, never
// shows.
func (r *Rule) InProfile(profileID string) bool {
	return len(r.Profiles) == 0 || slices.Contains(r.Profiles, profileID)
}

// ForProfile keeps the rules of a resolved corpus that apply under one
// profile, in corpus order. Every command that evaluates rules asks it.
//
// Scoping follows resolution rather than entering it: §2.6 item 2 overrides a
// lower layer's rule whole by id, so a per-repository rule scoped to another
// profile still shadows the global rule of the same id, and neither runs.
func ForProfile(corpus []Resolved, profileID string) []Resolved {
	kept := make([]Resolved, 0, len(corpus))
	for i := range corpus {
		if corpus[i].Rule.InProfile(profileID) {
			kept = append(kept, corpus[i])
		}
	}
	return kept
}
