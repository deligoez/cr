package brief

import (
	"errors"
	"io/fs"
	"os"
	"strconv"

	"github.com/deligoez/cr/internal/draft"
	"github.com/deligoez/cr/internal/finding"
	"github.com/deligoez/cr/internal/git"
	"github.com/deligoez/cr/internal/migrate"
	"github.com/deligoez/cr/internal/probe"
	"github.com/deligoez/cr/internal/state"
	"github.com/deligoez/cr/internal/unit"
)

// carrier is §9.4's migration as §9.3.4's increment runs it: every file of the
// new head it reads is read once, and every closing round's draft is read once,
// however many records the sweep carries.
type carrier struct {
	src       *Sources
	assembled *Brief
	// files are the new head's files read so far, by path. An absent file
	// is held as a nil slice with exists false, so it is not asked twice.
	files map[string]headFile
	// bodies are the carried agent regions per round, per §7.1.7.
	bodies map[int]map[string]string
}

type headFile struct {
	lines  []string
	exists bool
}

func newCarrier(src *Sources, assembled *Brief) *carrier {
	return &carrier{
		src: src, assembled: assembled,
		files: make(map[string]headFile), bodies: make(map[int]map[string]string),
	}
}

// read is the new head's tree, which is the only tree §9.4.2 lets a migration
// read.
func (c *carrier) read(path string) (lines []string, exists bool, err error) {
	if held, read := c.files[path]; read {
		return held.lines, held.exists, nil
	}
	lines, exists, err = git.FileAtRevision(c.src.RepoDir, c.assembled.Head, path)
	if err != nil {
		return nil, false, err
	}
	c.files[path] = headFile{lines: lines, exists: exists}
	return lines, exists, nil
}

// candidates are §9.4.3's files in its order: the anchor's own, then every
// other file the new round's diff touches, which are the paths its RIGHT
// units name.
func (c *carrier) candidates(own string) ([]migrate.File, error) {
	paths := []string{own}
	for i := range c.assembled.Units {
		touched := &c.assembled.Units[i]
		if touched.Side == git.Right && !contains(paths, touched.Path) {
			paths = append(paths, touched.Path)
		}
	}
	files := make([]migrate.File, 0, len(paths))
	for _, path := range paths {
		lines, exists, err := c.read(path)
		if err != nil {
			return nil, err
		}
		if exists {
			files = append(files, migrate.File{Path: path, Lines: lines})
		}
	}
	return files, nil
}

func contains(paths []string, path string) bool {
	for _, held := range paths {
		if held == path {
			return true
		}
	}
	return false
}

// unitFor is the new round's RIGHT unit holding the whole of a migrated
// anchor, and "" when none does. A comment outside the diff cannot be posted,
// so §9.4.5 carries no record there.
func (c *carrier) unitFor(path string, start, line int) *unit.Unit {
	for i := range c.assembled.Units {
		candidate := &c.assembled.Units[i]
		if candidate.Side == git.Right && candidate.Contains(path, start) && candidate.Contains(path, line) {
			return candidate
		}
	}
	return nil
}

// editedBodies are the agent regions a round's draft held that differ from
// that round's rendered.json: the bodies §7.1.6 would have preserved, which
// §7.1.7 carries. A round nobody drafted has none.
func (c *carrier) editedBodies(round int) (map[string]string, error) {
	if held, read := c.bodies[round]; read {
		return held, nil
	}
	edited := make(map[string]string)
	c.bodies[round] = edited
	layout, owner, repo, pr := c.src.Layout, c.src.Owner, c.src.Repo, c.src.PR
	written, err := os.ReadFile(layout.RoundFile(owner, repo, pr, round, state.FileDraft))
	if errors.Is(err, fs.ErrNotExist) || (err == nil && len(written) == 0) {
		return edited, nil
	} else if err != nil {
		return nil, err
	}
	// Bodies reads a draft's text, not its path.
	bodies, err := draft.Bodies(string(written))
	if err != nil {
		return nil, err
	}
	body, err := layout.ReadRound(owner, repo, pr, round, state.FileRendered)
	if err != nil && !errors.Is(err, fs.ErrNotExist) {
		return nil, err
	}
	rendered := map[string]string{}
	if len(body) > 0 {
		if rendered, err = draft.DecodeRendered(state.FileRendered, body); err != nil {
			return nil, err
		}
	}
	for id, region := range bodies {
		if region != rendered[id] {
			edited[id] = region
		}
	}
	return edited, nil
}

// carry is §9.4 over one `draft` or `queued` record: where its anchor lands at
// the new head, whether §9.4.5 carries it, and — when it does — the record as
// it moves, with §9.4.8's evidence read again.
//
// The record handed back is nil when the record stays behind. Every such record
// is reported all the same, because §9.4.7 reports every decline.
func (c *carrier) carry(record *finding.Finding) (migrate.Record, *finding.Finding, error) {
	line := migrate.Record{FromRound: record.Round, Round: c.assembled.Round, Head: c.assembled.Head}
	if record.Anchor.Side != git.Right {
		line.Outcome = migrate.Outcome{
			Record: record.ID,
			From:   record.Anchor.Path + ":" + strconv.Itoa(record.Anchor.Line),
		}
		if record.Anchor.Side == git.Left {
			line.Key = migrate.KeyLeft
		}
		return line, nil, nil
	}
	files, err := c.candidates(record.Anchor.Path)
	if err != nil {
		return migrate.Record{}, nil, err
	}
	line.Outcome = migrate.Anchor(record.ID, &record.Anchor, files)
	if !line.Placed {
		return line, nil, nil
	}
	holding := c.unitFor(line.Path, line.StartLine, line.Line)
	if holding == nil {
		return line, nil, nil
	}

	moved := *record
	moved.Anchor.Path, moved.Anchor.StartLine, moved.Anchor.Line = line.Path, line.StartLine, line.Line
	trees := finding.Trees{
		Head: c.read,
		MergeBase: func(path string) ([]string, bool, error) {
			return git.FileAtRevision(c.src.RepoDir, c.assembled.MergeBase, path)
		},
	}
	// §9.2's window and waiver key are read again at the new place. The
	// content hash comes back the same, because the place was chosen for
	// carrying it.
	if err := finding.StampAnchor(trees, state.FileFindings, 0, &moved.Anchor); err != nil {
		return migrate.Record{}, nil, err
	}
	// The citations are the agent's (§2.1.3), and the carry leaves every
	// entry the agent wrote; what it re-reads is the hash cr stamped on
	// each. moved shares its entries with record, which is the sweep's own
	// decoding of the line and read for nothing after this.
	if err := c.restamp(moved.Citations); err != nil {
		return migrate.Record{}, nil, err
	}
	moved.Unit = holding.ID
	line.Probe, moved.Probe = record.Probe, ""
	// §6.2 again, at the new head, over what the carry left: no probe, the
	// citations that still read what they were stamped with, and the unit
	// the anchor now sits in. Through the ratchet, so a grade can only
	// fall. `cr draft` regrades only the records a triage moved (§7.2.2
	// leaves the rest to `cr post`), so without this the draft would show
	// the reviewer an assertion whose evidence the carry had just cleared.
	finding.Regrade(&moved, finding.Resolved(
		holding, &moved.Anchor, nil, c.assembled.Head, probe.Baseline{}, probe.ClaimMapping{}))
	bodies, err := c.editedBodies(record.Round)
	if err != nil {
		return migrate.Record{}, nil, err
	}
	line.Body = bodies[record.ID]
	line.Carried = true
	return line, &moved, nil
}

// restamp is §9.4.8's reading of a carried record's citations at the new head:
// an entry keeps its content hash only when the line it names still holds the
// content that hash was taken from. Any other entry loses the hash, which §6.2's
// `cited` row reads as a citation no resolution reached — so a grade can rest
// on it no longer.
func (c *carrier) restamp(citations []finding.Citation) error {
	for i := range citations {
		entry := &citations[i]
		if entry.ContentHash == "" {
			continue
		}
		stamped := entry.ContentHash
		entry.ContentHash = ""
		lines, exists, err := c.read(entry.Path)
		if err != nil {
			return err
		}
		if !exists || entry.Line < 1 || entry.Line > len(lines) {
			continue
		}
		again := *entry
		if again.StampContentHash(lines[entry.Line-1]) == nil && again.ContentHash == stamped {
			entry.ContentHash = stamped
		}
	}
	return nil
}
