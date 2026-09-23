package profile

import (
	"errors"
	"fmt"
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
// the tied profiles, so Select stops and Err turns the names into the abort
// that rule requires, which the cli layer maps onto exit code 3. The empty
// outcome is answered by Missing instead, because §2.4.4 continues the run: it
// hands back the report and the lenses that are out, never an error.
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

// Err returns the abort §2.4.2 requires of a tie, and nil in every other state.
// It is the second half of that rule: cr names the tied profiles and stops
// rather than picking one, and routing the tie through an error is what keeps
// Profile out of reach of a caller that never looked at Tied.
//
// A selection that matched nothing is not an error here. §2.4.4 answers that
// state with a report and every axis needing a profile disabled, which is a run
// that continues, so it belongs to Missing rather than to this method.
func (s *Selection) Err() error {
	if len(s.Tied) == 0 {
		return nil
	}
	return &TieError{Profiles: s.Tied}
}

// TieError reports the tie §2.4.2 forbids resolving: two or more profiles
// matched the same greatest number of marker files. It is deliberately not a
// *MalformedError. Every tied profile was read, parsed, and validated, so there
// is no file to open and no field to correct; what is ambiguous is their
// combination against this one repository, and the fix is naming a `profile` in
// the per-repository config, which the message says. The cli layer maps it onto
// the exit code 3 §2.4.2 requires.
type TieError struct {
	// Profiles names the tied profiles, in the ascending id order
	// Selection.Tied reports them in.
	Profiles []string
}

func (e *TieError) Error() string {
	return fmt.Sprintf(
		"profiles %s match the same number of marker files, and §2.4.2 forbids picking one of them; "+
			"set `profile` in the per-repository config to the one this repository is",
		strings.Join(e.Profiles, ", "),
	)
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
		case markersPresent(repoRoot, candidates[i].Match.Unless) > 0:
			// §2.4.2: a file the profile names in `match.unless` is
			// present, so the repository is one it does not fit.
			continue
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
