package brief

import (
	"errors"
	"io/fs"

	"github.com/deligoez/cr/internal/gh"
	"github.com/deligoez/cr/internal/state"
	"github.com/deligoez/cr/internal/unit"
)

// persist records the derived inputs §3.7 permits and the rest of cr requires:
// meta.json's round and recorded head, units.ndjson, and threads.ndjson.
//
// The three are one critical section under one hold of the §2.3.1 lock, because
// they are one fact about the pull request. §2.3.3 stamps the units with the
// head they were computed at and §9.3.1 compares meta.json's recorded head
// against the current one, so a reader arriving between a units write and a
// meta write would find a round whose head says one thing and whose units were
// computed from another — and would have no way to tell.
//
// Nothing else in the state directory is touched. §3.7 permits the derived
// inputs of §3.3 through §3.6, and claims.ndjson is §3.3.1's to write and
// mapping.ndjson §4.1.6's; clearing either is §9.3.4's business on a round
// increment, which this does not perform.
func persist(src *Sources, assembled *Brief) error {
	// The whole §2.3 table is created first, and not only the three files
	// written below. Every one of the thirteen is a file some later command
	// reads, an absent one reads as a failure rather than as no records,
	// and `cr brief` is the command §3.7 makes responsible for the pull
	// request's state directory existing at all. EnsurePR leaves an
	// existing file exactly as it is, so this discards no recorded round.
	if err := src.Layout.EnsurePR(src.Owner, src.Repo, src.PR); err != nil {
		return err
	}
	held, err := src.Layout.LockPR(src.Owner, src.Repo, src.PR)
	if err != nil {
		return err
	}
	// Joined rather than branched: the lock is released whether or not the
	// writes succeeded, and neither failure is traded away for the other.
	return errors.Join(write(src, held, assembled), held.Unlock())
}

// write publishes the three files through the held lock.
func write(src *Sources, held *state.Lock, assembled *Brief) error {
	meta, err := metaOf(src, assembled)
	if err != nil {
		return err
	}
	if err := held.WriteMeta(meta); err != nil {
		return err
	}
	stamp := state.Stamp{Head: assembled.Head, Round: assembled.Round}
	if err := state.WriteStamped(held, state.FileUnits, stamp, records(assembled.Units)); err != nil {
		return err
	}
	return gh.WriteThreads(held, assembled.Threads)
}

// metaOf is the meta.json this round leaves behind: the identity, §3.2's key,
// §2.4's profile, and §9.3's round and head.
//
// Whatever the file already carried in the two fields no part of §3.7 decides —
// `active_roles`, which §4.5.1 settles and §4.6 reads, and `post_unresolved`,
// which §8.4.4 sets and only a reconciliation clears — is read back and carried
// through. Writing the document from the fields this package computes would
// clear a post whose outcome cr never learned, and `cr brief` is the command a
// user runs after exactly that.
func metaOf(src *Sources, assembled *Brief) (*state.Meta, error) {
	recorded, err := src.Layout.ReadMeta(src.Owner, src.Repo, src.PR)
	if err != nil && !errors.Is(err, fs.ErrNotExist) {
		return nil, err
	}
	return &state.Meta{
		Owner:          src.Owner,
		Repo:           src.Repo,
		PR:             src.PR,
		IssueKey:       assembled.Issue.Key,
		ProfileID:      assembled.Profile.ID,
		ActiveRoles:    recorded.ActiveRoles,
		Round:          assembled.Round,
		Head:           assembled.Head,
		PostUnresolved: recorded.PostUnresolved,
	}, nil
}

// records wraps §3.4.6's units as the lines units.ndjson holds, so §2.3.3's
// head and round are stamped onto each of them on the way out.
func records(units []unit.Unit) []*unit.Record {
	stored := make([]*unit.Record, 0, len(units))
	for _, formed := range units {
		stored = append(stored, &unit.Record{Unit: formed})
	}
	return stored
}
