package profile

import (
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"slices"
	"strings"
)

// fileExt is the suffix of a profile file in the profiles directory.
const fileExt = ".json"

// Selection is the outcome of §2.4 profile selection for one repository. It has
// exactly three states, and a caller must read them apart before using Profile:
//
//   - Selected, with Profile carrying the winner;
//   - a tie, with Tied naming the profiles that shared the greatest number of
//     matched marker files and Selected false;
//   - nothing matched, with Tied empty and Selected false.
//
// The tie is reported rather than broken here. §2.4.2 forbids picking one of
// the tied profiles, and the abort it requires — exit code 3 naming them — is a
// decision the cli layer makes, so this package hands it the names and stops.
// The same holds for the empty outcome: §2.4.4's report and axis disabling
// belong to the caller, not to the matcher.
type Selection struct {
	// Profile is the selected profile, meaningful only when Selected is true.
	Profile Profile
	// Selected is true when exactly one profile was chosen, whether by
	// configuration or by marker files.
	Selected bool
	// Tied names, in ascending id order, the profiles sharing the greatest
	// matched marker count when more than one did. It is empty in every
	// other state and never nil.
	Tied []string
}

// Select resolves the profile for the repository rooted at repoRoot, reading
// profile files from profilesDir.
//
// configured is the `profile` setting as §2.7 resolved it — the per-repository
// config is one of its layers — and naming it is the override §2.4.1 gives the
// automatic path: the named profile is loaded and no marker file is consulted,
// which is also the only way a profile with an empty `match.files` is ever
// used. A configured profile cr cannot read is a *MalformedError, so a name
// with no file behind it aborts with exit code 3 rather than falling back to a
// guess.
//
// Otherwise selection is automatic via `match.files`. repoRoot is the
// repository under review, so the marker files are looked up in the user's
// worktree: cr only stats them, and writes nothing there.
func Select(profilesDir, repoRoot, configured string) (Selection, error) {
	if configured != "" {
		p, err := Load(filepath.Join(profilesDir, configured+fileExt))
		if err != nil {
			return Selection{}, err
		}
		return Selection{Profile: p, Selected: true, Tied: []string{}}, nil
	}

	candidates, err := loadAll(profilesDir)
	if err != nil {
		return Selection{}, err
	}
	best := 0
	// winners indexes candidates rather than holding copies of them, so a
	// profile is copied once, when it is returned.
	winners := make([]int, 0, len(candidates))
	for i := range candidates {
		matched := markersPresent(repoRoot, candidates[i].Match.Files)
		switch {
		case matched == 0:
			// No marker of this profile is present, which includes the
			// profile that declares none: §2.4.3 rests on an empty
			// `match.files` matching nothing, so zero markers is zero
			// matches and never a match on everything.
			continue
		case matched > best:
			best = matched
			winners = append(winners[:0], i)
		case matched == best:
			winners = append(winners, i)
		}
	}

	switch len(winners) {
	case 0:
		return Selection{Tied: []string{}}, nil
	case 1:
		return Selection{Profile: candidates[winners[0]], Selected: true, Tied: []string{}}, nil
	}
	tied := make([]string, 0, len(winners))
	for _, winner := range winners {
		tied = append(tied, candidates[winner].ID)
	}
	slices.Sort(tied)
	return Selection{Tied: tied}, nil
}

// markersPresent counts the marker files of one profile that exist in the
// repository at repoRoot. A marker is counted once however often the profile
// lists it, so a repeated entry cannot inflate the count §2.4.2 compares, and
// an empty entry counts for nothing: it addresses the repository root, which
// always exists and would make every profile match.
func markersPresent(repoRoot string, markers []string) int {
	seen := make(map[string]struct{}, len(markers))
	present := 0
	for _, marker := range markers {
		if marker == "" {
			continue
		}
		if _, repeated := seen[marker]; repeated {
			continue
		}
		seen[marker] = struct{}{}
		if _, err := os.Stat(filepath.Join(repoRoot, marker)); err == nil {
			present++
		}
	}
	return present
}

// loadAll reads every profile file in dir, in ascending file name order. A
// directory that does not exist holds no profile, exactly like an empty one: cr
// has nothing to select from, which §2.4.4 already covers. Every file in it is
// validated, so a malformed profile aborts the command per §2.5 item 3 instead
// of quietly dropping out of the candidate set.
func loadAll(dir string) ([]Profile, error) {
	entries, err := os.ReadDir(dir)
	switch {
	case errors.Is(err, fs.ErrNotExist):
		return nil, nil
	case err != nil:
		return nil, &MalformedError{File: dir, Problem: "cannot be listed: " + err.Error()}
	}
	profiles := make([]Profile, 0, len(entries))
	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), fileExt) {
			continue
		}
		p, err := Load(filepath.Join(dir, entry.Name()))
		if err != nil {
			return nil, err
		}
		profiles = append(profiles, p)
	}
	return profiles, nil
}
