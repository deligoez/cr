package finding

import (
	"errors"
	"fmt"
	"strconv"
	"strings"

	"github.com/deligoez/cr/internal/state"
)

// The letters a waiver's id is numbered under, one per file of §7.4.4.
//
// §7.4.7 removes a waiver by id from either scope, and when `--pr` is given
// both files are in play at once, so an id that named only a number would not
// say which of the two files to open. These make the id name its own file, as
// §3.6.1's `<ISSUE-KEY>#n<n>` makes a note id name its own store — there is
// then no second place for the id and the scope to disagree, and no way to
// remove a waiver out of a file it was never in.
const (
	repositoryWaiverPrefix  = "wr"
	pullRequestWaiverPrefix = "wp"
)

// WaiverProvenance is the four things §7.4.8 requires a waiver to record: the
// round, the pull request, the head, and the reason when one was given.
//
// It deliberately does not embed state.Stamp, which carries the same head and
// round for the eight files of §2.3.3. Those two fields are what §9.3.5 has
// every command filter on when it reads only the current round's records, and
// §9.3.5 exempts waivers from that scoping in as many words: a waiver applies
// across rounds. Carrying the pair under the name every round-scoped reader
// recognises would invite exactly the read the exemption forbids. Neither of
// §7.4.4's files is one of §2.3.3's eight either, so the stamp's writer-owned
// contract does not reach them — state.WriteStamped refuses `waivers.ndjson`
// by name, and the repository-wide file is not per-PR state at all.
//
// Only the reason is optional, so only the reason is omitted when empty; a
// waiver that recorded no round, no pull request or no head would be a silence
// nobody could account for, and validate refuses one rather than writing it.
type WaiverProvenance struct {
	// Round is the review round the waiver was written in.
	Round int `json:"round"`
	// PR is the pull request whose triage wrote it. §7.4.3 makes this
	// provenance for a repository-wide waiver and the scope itself for a
	// pull-request-scoped one, which is why appendWaiver reads the pull
	// request from here rather than from a parameter beside it: two
	// sources for one number are one number and one bug.
	PR int `json:"pr"`
	// Head is the commit the waived record was anchored against.
	Head string `json:"head"`
	// Reason is why the reviewer silenced it, when they gave one.
	Reason string `json:"reason,omitempty"`
}

// validate refuses provenance §7.4.8 would not accept.
func (p WaiverProvenance) validate() error {
	switch {
	case p.Round < 1:
		return fmt.Errorf(
			"round %d: §7.4.8 has a waiver record the round it was written in, and §9.3.3 numbers rounds from 1; run cr brief to open one",
			p.Round,
		)
	case p.PR < 1:
		return fmt.Errorf(
			"invalid pull request %d: §7.4.8 has a waiver record the pull request it came from, so name it, e.g. 42",
			p.PR,
		)
	case strings.TrimSpace(p.Head) == "":
		return errors.New(
			"the head is empty: §7.4.8 has a waiver record the head it was written against, so pass the commit the record was anchored to",
		)
	}
	return nil
}

// WaiverRecord is one line of either file of §7.4.4: the waiver itself, the id
// §7.4.7 removes it by, and the provenance §7.4.8 requires.
//
// The three parts are embedded rather than nested, so a stored line is flat —
// `path`, `side`, `class` and `content_hash` from §7.4.1's key, `disposition`
// from §7.4.5, and §7.4.8's four beside them. A reader opening the file sees
// what is waived and why in one object, which is what makes §7.4.4's "readable"
// worth anything.
//
// It carries no scope field, for the reason WaiverScope gives: the scope is
// derived from the disposition, and a stored scope that disagreed with the
// disposition it was derived from would be believed.
type WaiverRecord struct {
	// ID is the waiver's id, spelled so it names its own file.
	ID string `json:"id"`
	// Waiver is §7.4.1's key and §7.4.5's disposition.
	Waiver
	// WaiverProvenance is §7.4.8's four fields.
	WaiverProvenance
}

// Waive records w in the file its scope names, and returns the waiver as it now
// stands (§7.4.4).
//
// The scope is derived here and never taken from a caller: §7.4.3 forbids a
// `not-here` from silencing a finding outside its pull request, and a parameter
// would be somewhere that mistake could be made. Its cost is a finding that
// stops being raised, which nobody would ever see.
//
// Waiving a key the file already covers returns the waiver already there and
// writes nothing. §7.1.6 re-ingests a draft's deletions every time the draft is
// regenerated — "including deletions, which become discards and waivers at that
// moment" — so the same key arrives more than once in the ordinary course, and
// a second line would silence nothing new while making §7.4.7's listing repeat
// itself. Within one file the disposition follows the scope, so a second waiver
// over one key can differ only in its provenance, and the first reviewer's is
// the one that records when the decision was made.
func Waive(l state.Layout, owner, repo string, w *Waiver, prov WaiverProvenance) (WaiverRecord, error) {
	scope, err := w.Scope()
	if err != nil {
		return WaiverRecord{}, err
	}
	if err := prov.validate(); err != nil {
		return WaiverRecord{}, err
	}
	return appendWaiver(l, owner, repo, scope, &WaiverRecord{Waiver: *w, WaiverProvenance: prov})
}

// appendWaiver is §7.4.4's file selection and the only one there is.
//
// It is unexported and takes the scope Waive derived, so nothing outside this
// file can choose which of the two files a waiver lands in. The third arm is
// unreachable through Waive — Scope returns one of exactly two values or an
// error — and it is written anyway: a scope added later without a file of its
// own must fail here rather than fall into whichever arm happens to be last.
func appendWaiver(
	l state.Layout, owner, repo string, scope WaiverScope, draft *WaiverRecord,
) (WaiverRecord, error) {
	switch scope {
	case ScopeRepository:
		return appendRepositoryWaiver(l, owner, repo, draft)
	case ScopePullRequest:
		return appendPullRequestWaiver(l, owner, repo, draft)
	}
	return WaiverRecord{}, fmt.Errorf(
		"%q names neither file of §7.4.4: a waiver is written to %q or to %q, and its scope follows its disposition",
		scope, ScopeRepository, ScopePullRequest,
	)
}

// appendRepositoryWaiver writes one waiver to
// `~/.cr/waivers/<owner>/<repo>.ndjson` (§7.4.4).
func appendRepositoryWaiver(l state.Layout, owner, repo string, draft *WaiverRecord) (WaiverRecord, error) {
	lock, err := l.LockRepoWaivers(owner, repo)
	if err != nil {
		return WaiverRecord{}, err
	}
	recorded, err := appendRepositoryLocked(lock, draft)
	// Joined rather than branched: the lock is released whether or not the
	// waiver was written, and neither failure is traded away for the other.
	return recorded, errors.Join(err, lock.Unlock())
}

// appendRepositoryLocked is appendRepositoryWaiver's critical section. The
// read, the id allocation and the write are one, for the reason
// state.WaiverLock gives.
func appendRepositoryLocked(k *state.WaiverLock, draft *WaiverRecord) (WaiverRecord, error) {
	existing, err := state.WaiverRecords[WaiverRecord](k)
	if err != nil {
		return WaiverRecord{}, err
	}
	if already, covered := coveredBy(existing, draft.WaiverKey); covered {
		return already, nil
	}
	recorded := *draft
	recorded.ID = nextWaiverID(repositoryWaiverPrefix, existing)
	return recorded, state.WriteWaiverRecords(k, append(existing, recorded))
}

// appendPullRequestWaiver writes one waiver to that pull request's
// `waivers.ndjson` (§7.4.4, §2.3).
//
// The state directory is created before the lock rather than under it. §2.3.1
// admits no unlocked write to per-PR state, so state.EnsurePR takes the lock
// itself, and taking it here first would deadlock against that: a second
// flock(2) on one path from one process waits for the first. EnsurePR is
// idempotent and creates only what is missing, so running it ahead of the lock
// costs nothing and leaves the file below always there to be read.
func appendPullRequestWaiver(l state.Layout, owner, repo string, draft *WaiverRecord) (WaiverRecord, error) {
	pr := draft.PR
	if err := l.EnsurePR(owner, repo, pr); err != nil {
		return WaiverRecord{}, err
	}
	lock, err := l.LockPR(owner, repo, pr)
	if err != nil {
		return WaiverRecord{}, err
	}
	recorded, err := appendPullRequestLocked(l, lock, owner, repo, draft)
	return recorded, errors.Join(err, lock.Unlock())
}

// appendPullRequestLocked is appendPullRequestWaiver's critical section. The
// read is made under the lock even though §2.3.2 does not require one, because
// what has to be atomic is the read, the id allocation and the write together.
func appendPullRequestLocked(
	l state.Layout, k *state.Lock, owner, repo string, draft *WaiverRecord,
) (WaiverRecord, error) {
	existing, err := PullRequestWaivers(l, owner, repo, draft.PR)
	if err != nil {
		return WaiverRecord{}, err
	}
	if already, covered := coveredBy(existing, draft.WaiverKey); covered {
		return already, nil
	}
	recorded := *draft
	recorded.ID = nextWaiverID(pullRequestWaiverPrefix, existing)
	return recorded, state.WriteRecords(k, state.FileWaivers, append(existing, recorded))
}

// RepositoryWaivers returns every waiver recorded for a repository: the file
// §7.4.4 writes a `wrong` disposition to, and the one §7.4.7 lists when no
// pull request is named. It takes no lock, per §2.3.2.
func RepositoryWaivers(l state.Layout, owner, repo string) ([]WaiverRecord, error) {
	return state.ReadWaiverRecords[WaiverRecord](l, owner, repo)
}

// PullRequestWaivers returns every waiver recorded for one pull request: the
// file §7.4.4 writes a `not-here` disposition to. It takes no lock, per §2.3.2.
//
// The file is one of §2.3's table, so it is created with the state directory
// and is always there for a pull request cr has been briefed on. One it has not
// been briefed on has no round to have waived anything in, and the missing file
// is reported rather than read as an empty one.
func PullRequestWaivers(l state.Layout, owner, repo string, pr int) ([]WaiverRecord, error) {
	return state.ReadRecords[WaiverRecord](l, owner, repo, pr, state.FileWaivers)
}

// ActiveWaivers returns every waiver that can silence a finding on one pull
// request: §7.4.4's repository-wide file first, then that pull request's own.
//
// Both are read because §6.4.4 drops a finding matching an active waiver and
// §6.5.1 requires the pull request for exactly this reason — "a merge that
// could not read them would resurface exactly what the reviewer set aside".
func ActiveWaivers(l state.Layout, owner, repo string, pr int) ([]WaiverRecord, error) {
	wide, err := RepositoryWaivers(l, owner, repo)
	if err != nil {
		return nil, err
	}
	here, err := PullRequestWaivers(l, owner, repo, pr)
	if err != nil {
		return nil, err
	}
	return append(wide, here...), nil
}

// WaivedBy reports the waiver covering a record, if one does.
//
// This is the lookup half of §7.4.6. What it does not do is drop the record or
// count the drop: §6.4.4 puts both at merge time, where the round summary of
// §10.3 is written, and a lookup that removed things would have to know which
// of §6.4's passes it was in.
//
// Matching is equality over the whole of §7.4.1's key, per WaiverKey: the same
// class at the same unchanged code on the same side of the same file. §7.4.2
// makes that narrowness the point — the waiver stops suppressing once the code
// changes, which is when the judgement behind it should be revisited.
func WaivedBy(waivers []WaiverRecord, record *Finding) (WaiverRecord, bool) {
	return coveredBy(waivers, WaiverKeyOf(record))
}

// coveredBy reports the first waiver keyed exactly like key.
//
// Indexed rather than ranged by value: a stored waiver is a wide struct, and
// nothing here writes to one.
func coveredBy(waivers []WaiverRecord, key WaiverKey) (WaiverRecord, bool) {
	for i := range waivers {
		if waivers[i].WaiverKey == key {
			return waivers[i], true
		}
	}
	return WaiverRecord{}, false
}

// RemoveWaiver deletes one waiver from the file its id names, and returns the
// waiver it removed (§7.4.7).
//
// **It deletes; it does not mark.** §3.6.6's retraction marks a note instead,
// and the reason it gives does not reach a waiver. A note is marked because
// §3.3.2's claims and §8.1.6's provenance cite one by id, so a deleted line
// would free a number a stored record still points at. Nothing cites a waiver:
// §7.4.1 matches one by key and never by id, so a reissued number strands no
// record. Its whole cost is that a listing read before a removal can name a
// different waiver after one — a wrongly removed silence, which surfaces as the
// finding being raised again and is retriaged in the open.
//
// The trust economy then points the other way from §3.6.6's. A tombstone in a
// suppression file is one forgotten filter away from a silence that never ends,
// and a silence is the one failure nobody observes — the finding simply stops
// being raised. §7.4.4 requires that neither disposition be a one-way door, and
// a line that is gone cannot suppress anything.
//
// The reviewer's decision is not lost with it. §7.2's discard already wrote a
// triage event that §7.3 counts, and that record is what the history is read
// from; the waiver file is the live set of silences, not the log of them.
func RemoveWaiver(l state.Layout, owner, repo string, pr int, id string) (WaiverRecord, error) {
	switch {
	case waiverIDIsUnder(repositoryWaiverPrefix, id):
		return removeRepositoryWaiver(l, owner, repo, id)
	case waiverIDIsUnder(pullRequestWaiverPrefix, id):
		return removePullRequestWaiver(l, owner, repo, pr, id)
	}
	return WaiverRecord{}, fmt.Errorf(
		"waiver id %q: a repository-wide waiver is %s<n> and a pull-request-scoped one is %s<n>, e.g. %s2; run `cr waivers list --repo %s/%s` for the ids the files hold",
		id, repositoryWaiverPrefix, pullRequestWaiverPrefix, pullRequestWaiverPrefix, owner, repo,
	)
}

// removeRepositoryWaiver deletes one waiver from the repository-wide file.
func removeRepositoryWaiver(l state.Layout, owner, repo, id string) (WaiverRecord, error) {
	lock, err := l.LockRepoWaivers(owner, repo)
	if err != nil {
		return WaiverRecord{}, err
	}
	removed, err := removeRepositoryLocked(lock, id)
	return removed, errors.Join(err, lock.Unlock())
}

// removeRepositoryLocked is removeRepositoryWaiver's critical section. The read
// and the write are one for the reason the appends' are: both republish the
// whole file, so a removal computed outside the lock would be published over a
// waiver a concurrent triage had just appended.
func removeRepositoryLocked(k *state.WaiverLock, id string) (WaiverRecord, error) {
	existing, err := state.WaiverRecords[WaiverRecord](k)
	if err != nil {
		return WaiverRecord{}, err
	}
	kept, removed, found := withoutWaiver(existing, id)
	if !found {
		return WaiverRecord{}, &UnknownWaiverError{ID: id}
	}
	return removed, state.WriteWaiverRecords(k, kept)
}

// removePullRequestWaiver deletes one waiver from a pull request's file. The
// state directory is created before the lock, for the reason
// appendPullRequestWaiver gives.
func removePullRequestWaiver(l state.Layout, owner, repo string, pr int, id string) (WaiverRecord, error) {
	if pr < 1 {
		return WaiverRecord{}, fmt.Errorf(
			"waiver %s is scoped to a pull request, so name one with --pr: §7.4.4 stores it in that pull request's waivers.ndjson",
			id,
		)
	}
	if err := l.EnsurePR(owner, repo, pr); err != nil {
		return WaiverRecord{}, err
	}
	lock, err := l.LockPR(owner, repo, pr)
	if err != nil {
		return WaiverRecord{}, err
	}
	removed, err := removePullRequestLocked(l, lock, owner, repo, pr, id)
	return removed, errors.Join(err, lock.Unlock())
}

// removePullRequestLocked is removePullRequestWaiver's critical section.
func removePullRequestLocked(
	l state.Layout, k *state.Lock, owner, repo string, pr int, id string,
) (WaiverRecord, error) {
	existing, err := PullRequestWaivers(l, owner, repo, pr)
	if err != nil {
		return WaiverRecord{}, err
	}
	kept, removed, found := withoutWaiver(existing, id)
	if !found {
		return WaiverRecord{}, &UnknownWaiverError{ID: id}
	}
	return removed, state.WriteRecords(k, state.FileWaivers, kept)
}

// withoutWaiver splits a file's waivers into the ones that stay and the one id
// names. The result is never nil, so an emptied file serialises as no lines
// rather than as a null slice (§12.3).
func withoutWaiver(existing []WaiverRecord, id string) (kept []WaiverRecord, removed WaiverRecord, found bool) {
	kept = make([]WaiverRecord, 0, len(existing))
	for i := range existing {
		if !found && existing[i].ID == id {
			removed, found = existing[i], true
			continue
		}
		kept = append(kept, existing[i])
	}
	return kept, removed, found
}

// UnknownWaiverError reports an id no waiver in the named file carries.
//
// It exists so §7.4.7's removal can tell "there is no such waiver" from "the
// file could not be read": the first is the reviewer naming the wrong id, and
// the second is a store to go and open.
type UnknownWaiverError struct {
	// ID is the id that named nothing, exactly as it was given.
	ID string
}

func (e *UnknownWaiverError) Error() string {
	return fmt.Sprintf(
		"no waiver %s is recorded: run `cr waivers list` for the ids the files hold", e.ID,
	)
}

// nextWaiverID allocates the id for a new waiver in one of §7.4.4's files.
//
// existing MUST be every waiver that file holds. The count runs to the highest
// id rather than to the number of lines, so a waiver appended beside a gap does
// not collide with one already there.
//
// It counts over what the file holds now, so §7.4.7's deletion does free the
// number the removed waiver had. That is deliberate rather than overlooked, and
// RemoveWaiver says what it costs: nothing cites a waiver by id, so a reissued
// number points no stored record at the wrong thing — which is the harm §3.6.1
// avoids reuse for, and the reason a note is marked where a waiver is deleted.
func nextWaiverID(prefix string, existing []WaiverRecord) string {
	highest := 0
	// Indexed rather than ranged by value: only the id is read here.
	for i := range existing {
		// `>=` would behave identically — it would assign the value
		// already held — so no test can tell the two apart.
		if n, ok := parseWaiverID(prefix, existing[i].ID); ok && n > highest {
			highest = n
		}
	}
	return prefix + strconv.Itoa(highest+1)
}

// waiverIDIsUnder reports whether id is one of the ids prefix numbers.
func waiverIDIsUnder(prefix, id string) bool {
	_, ok := parseWaiverID(prefix, id)
	return ok
}

// WaiverIDScope reports the scope whose file an id names, and false for an id
// spelled under neither prefix.
//
// It exists for the caller that has to know which file a removal will write
// before it makes the removal: §9.3.2 refuses a write to per-PR state over a
// moved head, and only a pull-request-scoped id's file is per-PR state. It reads
// the same two predicates RemoveWaiver dispatches on, so the answer and the
// file RemoveWaiver opens cannot disagree.
func WaiverIDScope(id string) (WaiverScope, bool) {
	switch {
	case waiverIDIsUnder(repositoryWaiverPrefix, id):
		return ScopeRepository, true
	case waiverIDIsUnder(pullRequestWaiverPrefix, id):
		return ScopePullRequest, true
	}
	return WaiverScope{}, false
}

// parseWaiverID reads the n of a `<prefix><n>` waiver id.
//
// It accepts only the canonical spelling: wp7 is an id, wp+7, wp07 and wp-7 are
// not, and neither is wp0, because waivers are numbered from one. An id under
// the other prefix is the other file's, and an id cr did not write is no
// evidence about what is taken, so neither contributes to the next allocation
// rather than being read as some number near it.
func parseWaiverID(prefix, id string) (int, bool) {
	rest, found := strings.CutPrefix(id, prefix)
	if !found {
		return 0, false
	}
	n, err := strconv.Atoi(rest)
	if err != nil || rest != strconv.Itoa(n) || n < 1 {
		return 0, false
	}
	return n, true
}
