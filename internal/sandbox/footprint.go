package sandbox

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"time"

	"github.com/deligoez/cr/internal/state"
)

// Footprint is what a sandbox on disk costs: where it is, how many bytes it
// holds, and since when. `cr status` reports it, because a sandbox keeps a
// checkout and the copies of gitignored files such as `.env` until
// `cr sandbox destroy` removes it, and nothing else cr prints says it is there.
type Footprint struct {
	// Path is the sandbox directory.
	Path string `json:"path"`
	// Bytes is the size of the regular files under it. A symbolic link is
	// not followed, so a link to a tree elsewhere counts nothing.
	Bytes int64 `json:"bytes"`
	// Since is when the sandbox was built: the post-setup baseline's
	// generation, which is the moment Create recorded it, or the
	// directory's modification time when no baseline names one.
	Since time.Time `json:"since"`
	// AgeSeconds is the time from Since to the moment of the report.
	AgeSeconds int64 `json:"age_seconds"`
}

// FootprintOf measures the pull request's sandbox, and reports one that is not
// there as nil rather than as a failure.
func FootprintOf(l state.Layout, owner, repo string, pr int, now time.Time) (*Footprint, error) {
	path := l.Sandbox(owner, repo, pr)
	info, err := os.Lstat(path)
	switch {
	case errors.Is(err, fs.ErrNotExist):
		return nil, nil
	case err != nil:
		return nil, fmt.Errorf("cannot inspect %s: %w", path, err)
	}
	size, err := regularBytes(path)
	if err != nil {
		return nil, err
	}
	since := info.ModTime()
	recorded, err := ReadBaseline(l, owner, repo, pr)
	if err != nil {
		return nil, err
	}
	if recorded != nil {
		if built, err := time.Parse(time.RFC3339Nano, recorded.Generation); err == nil {
			since = built
		}
	}
	return &Footprint{
		Path: path, Bytes: size, Since: since.UTC(), AgeSeconds: int64(now.Sub(since) / time.Second),
	}, nil
}

// regularBytes sums the sizes of the regular files under root. WalkDir does
// not follow a symbolic link, and a link is not a regular file, so neither the
// link nor what it points at is counted.
func regularBytes(root string) (int64, error) {
	var total int64
	err := filepath.WalkDir(root, func(_ string, d fs.DirEntry, err error) error {
		// A file a running suite removed between the listing and the
		// visit is simply no longer part of the sandbox.
		if errors.Is(err, fs.ErrNotExist) {
			return nil
		}
		if err != nil {
			return err
		}
		if !d.Type().IsRegular() {
			return nil
		}
		info, err := d.Info()
		if errors.Is(err, fs.ErrNotExist) {
			return nil
		}
		if err != nil {
			return err
		}
		total += info.Size()
		return nil
	})
	if err != nil {
		return 0, fmt.Errorf("cannot measure %s: %w", root, err)
	}
	return total, nil
}
