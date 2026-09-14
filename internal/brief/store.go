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
// Nothing else in the state directory is touched on a run that opened no
// round. §3.7 permits the derived inputs of §3.3 through §3.6, and claims.ndjson
// is §3.3.1's to write and mapping.ndjson §4.1.6's. On §9.3.3's increment both
// are also §9.3.4's, along with the stale sweep over findings.ndjson, and
// invalidate.go is where those three happen — inside this same critical
// section, so a reader never finds a round that was opened and not invalidated.
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

// write publishes the three files through the held lock, preceded on §9.3.3's
// increment by §9.3.4's invalidation.
//
// The units replace this round's lines of units.ndjson and leave every earlier
// round's lines in place (§9.3.5), so a same-head brief rewrites its own round's
// units and a new round's units are added beside the history its stale records
// and mapping name.
//
// meta.json is written last, and that is the whole of what makes an interrupted
// increment recoverable. The comparison §9.3.3 makes is against the head
// meta.json records, so while that file still names the closing round every
// re-run reaches the same decision and redoes the invalidation; once it names
// the opening round the comparison is equal and nothing will ever sweep again.
// Each of the three is idempotent, so redoing them costs nothing, and the price
// of the ordering is a round index that can skip a number after a crash —
// cheap beside a round that says it is new over records nobody staled.
func write(src *Sources, held *state.Lock, assembled *Brief) error {
	if assembled.round.opened() {
		if err := invalidate(held, assembled); err != nil {
			return err
		}
	}
	stamp := state.Stamp{Head: assembled.Head, Round: assembled.Round}
	if err := state.ReplaceStamped(held, state.FileUnits, stamp, records(assembled.Units)); err != nil {
		return err
	}
	if err := gh.WriteThreads(held, assembled.Threads); err != nil {
		return err
	}
	meta, err := metaOf(src, assembled)
	if err != nil {
		return err
	}
	return held.WriteMeta(meta)
}

// metaOf is the meta.json this round leaves behind: the identity, §3.2's key,
// §2.4's profile, §4.5.1's active roles, and §9.3's round and head.
//
// `active_roles` is written here, and this is the only place in cr that writes
// it. §4.5.1 settles the field out of the axis decision of §4.5.1 to §4.5.3 and
// the resolved profile of §2.4, and `cr brief` is the command that establishes
// both — §3.7.6 has it print the axes and §3.7.1 the profile. The alternative
// would be `cr review`, and that is round 8's `circular-definition` finding:
// §4.5.6 rejects a cell naming an inactive role and §10.2.2 counts a complete
// row of cells per active role, so the set has to stand whether or not a
// fan-out has ever run. One writer is what keeps `cr cells record` and
// `cr status` reading the same answer.
//
// `post_unresolved` is the field §3.7 decides nothing about: §8.4.4 sets it,
// only a reconciliation clears it, so whatever the file already carried is read
// back and carried through. Writing the document from the fields this package
// computes would clear a post whose outcome cr never learned, and `cr brief` is
// the command a user runs after exactly that. Only a same-head brief carries a
// set flag: roundOf refuses §9.3.3's increment over one, so a new round never
// opens under a send it holds no payload of.
//
// The mapping stamp is carried through for the same reason: `cr map record`
// writes it, and a same-head brief that dropped it would hold a round §4.6.5
// had already unblocked. A stamp carried past §9.3.3's increment names the
// round being closed, which state.Meta.MappingRecorded reads as no mapping.
//
// The claims stamp is carried through too, and past the increment it moves
// with the claims: §9.3.4 carries the closing round's claims forward
// unchanged, so a closing round that recorded its claims, an empty set
// included, opens a round whose claims are recorded.
func metaOf(src *Sources, assembled *Brief) (*state.Meta, error) {
	recorded, err := src.Layout.ReadMeta(src.Owner, src.Repo, src.PR)
	if err != nil && !errors.Is(err, fs.ErrNotExist) {
		return nil, err
	}
	claimsRound, claimsHead := recorded.ClaimsRound, recorded.ClaimsHead
	if assembled.round.opened() && recorded.ClaimsRecorded() {
		claimsRound, claimsHead = assembled.Round, assembled.Head
	}
	return &state.Meta{
		Owner:          src.Owner,
		Repo:           src.Repo,
		PR:             src.PR,
		IssueKey:       assembled.Issue.Key,
		ProfileID:      assembled.Profile.ID,
		ActiveRoles:    assembled.ActiveRoles,
		Round:          assembled.Round,
		Head:           assembled.Head,
		PostUnresolved: recorded.PostUnresolved,
		MappingRound:   recorded.MappingRound,
		MappingHead:    recorded.MappingHead,
		ClaimsRound:    claimsRound,
		ClaimsHead:     claimsHead,
	}, nil
}

// records wraps §3.4.6's units as the lines units.ndjson holds, so §2.3.3's
// head and round are stamped onto each of them on the way out.
func records(units []unit.Unit) []*unit.Record {
	stored := make([]*unit.Record, 0, len(units))
	for i := range units {
		stored = append(stored, &unit.Record{Unit: units[i]})
	}
	return stored
}
