package testadequacy

import (
	"github.com/deligoez/cr/internal/git"
	"github.com/deligoez/cr/internal/profile"
	"github.com/deligoez/cr/internal/symbol"
)

// HeadReferences builds §4.4.1's References for one round out of §4.3.1's head
// symbol index: it reads each test file the round's hunks name at rev, and
// answers for it with the head symbols its lines name.
//
// It returns a nil References when index is nil, which is what Attach turns
// into the §4.5.4 entry naming why no index arrived. The nil is returned as the
// interface, never as a typed nil inside it: a nil map wrapped in References
// would compare non-nil, and Attach would report a lens that ran over an index
// nobody built.
//
// A test file the head does not hold — the one a pull request deletes — is left
// out, so References reports false for it and Attach names it, rather than
// answering with the empty list a file that references nothing would get. A git
// read that fails is an error, because the §4.5.4 entry says what cr could not
// read, and a failing git is not that.
func HeadReferences(
	dir, rev string, p *profile.Profile, index *symbol.Index, hunks []git.Hunk,
) (References, error) {
	if index == nil {
		return nil, nil
	}
	paths := testPaths(p, hunks)
	read := make(headReferences, len(paths))
	for _, path := range paths {
		lines, held, err := git.FileAtRevision(dir, rev, path)
		if err != nil {
			return nil, err
		}
		if held {
			read[path] = index.Referenced(path, lines)
		}
	}
	return read, nil
}

// headReferences is References over the test files one head holds, keyed by
// path.
type headReferences map[string][]string

// Referenced answers from the files HeadReferences read.
func (h headReferences) Referenced(path string) ([]string, bool) {
	named, held := h[path]
	return named, held
}
