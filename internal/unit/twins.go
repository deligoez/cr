package unit

import (
	"slices"
	"strings"

	"github.com/deligoez/cr/internal/git"
	"github.com/deligoez/cr/internal/text"
)

// Owns reports whether a hunk of the diff is one of the unit's: on its file and
// side, with a range the unit recorded, numbered on that side as §3.4.6 stores
// it.
func (u *Unit) Owns(hunk *git.Hunk) bool {
	if hunk.Path != u.Path || hunk.Side != u.Side {
		return false
	}
	start, end := hunk.SideRange()
	return slices.Contains(u.HunkRanges, Range{Start: start, End: end})
}

// MarkTwins is §4.6.8 over one round's units, in §3.4.6's id order: a unit
// whose hunks equal an earlier unit's, line for line after §1.4, is that unit's
// twin, and TwinOf names the earliest unit it repeats.
//
// A hunk is compared by its body — every context, added and removed line, each
// with its marker — and not by its header, whose line numbers and section
// heading say where the change sits rather than what it is. The measured case
// is two event-test files that received the identical change: their hunks sit
// at different lines of different files and read the same.
//
// hunks and texts are the diff's hunks and, index for index, their texts, as
// git.ParseHunks and git.HunkTexts return them. The error is §1.4 step 1's, for
// a hunk that is not valid UTF-8.
func MarkTwins(units []Unit, hunks []git.Hunk, texts []string) error {
	bodies := make([][]string, len(units))
	for i := range units {
		body, err := bodiesOf(&units[i], hunks, texts)
		if err != nil {
			return err
		}
		bodies[i] = body
		units[i].TwinOf = ""
		if len(body) == 0 {
			continue
		}
		for earlier := range i {
			if units[earlier].TwinOf == "" && slices.Equal(bodies[earlier], body) {
				units[i].TwinOf = units[earlier].ID
				break
			}
		}
	}
	return nil
}

// bodiesOf is the unit's hunks' bodies, each normalised per §1.4, in the order
// the diff gives them.
func bodiesOf(u *Unit, hunks []git.Hunk, texts []string) ([]string, error) {
	bodies := make([]string, 0, len(u.HunkRanges))
	for at := range hunks {
		if at >= len(texts) || !u.Owns(&hunks[at]) {
			continue
		}
		_, body, _ := strings.Cut(texts[at], "\n")
		normalised, err := text.Normalise(body)
		if err != nil {
			return nil, err
		}
		bodies = append(bodies, normalised)
	}
	return bodies, nil
}
