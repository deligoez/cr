package review

import (
	"fmt"
	"slices"

	"github.com/deligoez/cr/internal/activation"
	"github.com/deligoez/cr/internal/axis"
	"github.com/deligoez/cr/internal/config"
	"github.com/deligoez/cr/internal/coverage"
	"github.com/deligoez/cr/internal/finding"
	"github.com/deligoez/cr/internal/gh"
	"github.com/deligoez/cr/internal/git"
	"github.com/deligoez/cr/internal/intent"
	"github.com/deligoez/cr/internal/mapping"
	"github.com/deligoez/cr/internal/note"
	"github.com/deligoez/cr/internal/profile"
	"github.com/deligoez/cr/internal/role"
	"github.com/deligoez/cr/internal/state"
	"github.com/deligoez/cr/internal/unit"
)

// Sources are the inputs one fan-out reads.
//
// Everything that touches the outside world is a field, for the reason
// brief.Sources gives: the GitHub client is injectable and the repository is a
// directory, so §4.6 can be exercised against a real repository with no token
// and no network. `cr review` emits prompts, and the state it reads was written
// by the commands that own it; what it writes is the fan-out directories and
// §2.6.1.6's ledger entries for the hits it found.
type Sources struct {
	// Layout resolves §2.2's paths and holds the per-PR state.
	Layout state.Layout
	// GH reads the pull request, for the base §3.4.1's merge base is taken
	// against.
	GH gh.Client
	// Config is §2.7's resolved configuration.
	Config config.Config
	// Owner, Repo, and PR name the pull request under review.
	Owner string
	Repo  string
	PR    int
	// RepoDir is the repository under review. Every read of it goes
	// through internal/git.
	RepoDir string
	// Axis narrows the fan-out to the roles of one axis, and is empty for
	// every axis.
	Axis string
}

// currentHead is §9.3.1's current head, read through the client already on
// these sources rather than through a seam of its own.
//
// gh/pr.go argues it must be GitHub's `headRefOid`: a local branch of the same
// name may sit anywhere, and on a fork it names a different history
// altogether, so a head taken from the checkout would make the round's
// staleness a property of what the user happened to have checked out. The same
// read supplies §3.4.1's base, so the answer the comparison uses and the answer
// the diff uses come from one place.
func (s *Sources) currentHead() (string, error) {
	opened, err := s.GH.PullRequest(s.Owner, s.Repo, s.PR)
	if err != nil {
		return "", err
	}
	return opened.Head, nil
}

// Fanout is what `cr review` reports: the round the prompts were emitted for,
// the prompts, and §4.5.4's report of the lenses that did not run.
type Fanout struct {
	// Round and Head are the round the prompts belong to.
	Round int    `json:"round"`
	Head  string `json:"head"`
	// Prompts are §4.6.1's prompts, in emission order.
	Prompts []Prompt `json:"prompts"`
	// Honesty is §4.5.4's report of every lens that did not run this
	// round — the disabled and unavailable axes, the halves of §4.3.1 and
	// §4.4.1, and §4.6.4's skipped roles — rendered as the sentences §11.1
	// exempts from `--quiet`. It is empty, and never nil, when every lens
	// looked.
	//
	// It holds the same kinds `cr status` puts in its own `honesty`,
	// because the two are one channel: a reader parsing that array is
	// reading §4.5.4's report whichever command produced it, and a kind
	// carried by one command and dropped by the other would be a lens
	// that never looked going unstated depending on what was run.
	Honesty []string `json:"honesty"`
	// Skipped is §4.6.4's report typed, as `cr status` carries it in
	// §10.1.3's lens list: every role of §2.5.5's corpus the round left
	// out, with the reason its prerequisites were unmet. Its sentences
	// are already in Honesty above; this is the same report as data, so
	// a caller need not parse them back out.
	//
	// It is the round's set and not this invocation's, exactly as
	// Expected is. `--axis` narrows which of the active roles get a
	// prompt, and a role left out of one pass of §4.6.5 has not been
	// skipped — reporting it here would tell the reader that a role which
	// is about to run did not, which is the opposite of the silence
	// §4.6.4 forbids.
	Skipped []coverage.SkippedRole `json:"skipped_roles"`
	// Expected is §4.6.3's set of cells the round is waiting for, so
	// §10.2.2 can be checked once the roles return. It is the round's
	// whole demand and not this invocation's: `--axis` narrows Prompts and
	// leaves this alone.
	Expected []coverage.Expected `json:"expected_cells"`
}

// StaleUnitError reports a unit units.ndjson recorded that the diff at the
// round's head no longer gives.
//
// The units are `cr brief`'s to record (§3.7), and every command of the round
// checks a unit id against that record, so a prompt has to show the unit the
// round holds. When the diff cr reads now cannot produce one of its hunks — the
// base moved, the checkout lost the head, a setting changed the clustering —
// a prompt built anyway would show a role code other than the code its cell
// will be counted against. §11.2 codes it 4: nothing about the invocation is
// wrong, and what refuses is where the round stands.
type StaleUnitError struct {
	// Unit is the unit id that could not be rebuilt.
	Unit string
	// Round and Head are the round that recorded it.
	Round int
	Head  string
	// Owner, Repo, and PR name the pull request, so the hint is runnable.
	Owner string
	Repo  string
	PR    int
}

func (e *StaleUnitError) Error() string {
	return fmt.Sprintf(
		"unit %s of round %d is not what the diff at head %s gives, so no prompt can show the code "+
			"its cell would be counted against; §3.7 has `cr brief` record the units: run "+
			"`cr brief %d --repo %s/%s` and review the round it records",
		e.Unit, e.Round, e.Head, e.PR, e.Owner, e.Repo)
}

// MappingRequiredError reports §4.6.5's refusal: the remaining axes cannot be
// emitted until the intent pass has produced a mapping for the round.
//
// §11.2 codes it 4 for the reason StaleUnitError is coded 4. Nothing about the
// invocation is wrong and no file the caller named is malformed; what refuses is
// where the round stands, and it is undone by running the intent pass rather
// than by retyping. The message names both halves of the way forward, because a
// reviewer who is told only that a mapping is missing has no way to know that
// `cr review --axis intent` is what produces one.
type MappingRequiredError struct {
	// Round and Head are the round that holds no mapping.
	Round int
	Head  string
	// Owner, Repo, and PR name the pull request, so the hint is runnable.
	Owner string
	Repo  string
	PR    int
	// Axis is the `--axis` the invocation named, and is empty for the
	// whole fan-out.
	Axis string
}

func (e *MappingRequiredError) Error() string {
	// The scope is the invocation's, not the spec's phrase, so a reader who
	// typed `--axis correctness` is told about the axis they asked for.
	scope := "the remaining axes"
	if e.Axis != "" {
		scope = "axis " + e.Axis
	}
	return fmt.Sprintf(
		"round %d at head %s has no mapping, so %s cannot be emitted; §4.6.5 runs the intent "+
			"pass first: run `cr review %d --repo %s/%s --axis intent`, store what it produces "+
			"with `cr map record %d --repo %s/%s <file.ndjson>`, and run this again",
		e.Round, e.Head, scope, e.PR, e.Owner, e.Repo, e.PR, e.Owner, e.Repo)
}

// Run gathers one round's attachments and emits §4.6.1's prompts over them.
//
// The round is the one `cr brief` recorded, read through state.Layout.Briefed,
// so a pull request no round has been opened on is refused with §11.2's code 4
// naming the command that opens one.
func Run(src *Sources) (*Fanout, error) {
	round, err := src.Layout.Briefed(src.Owner, src.Repo, src.PR, src.currentHead)
	if err != nil {
		return nil, err
	}
	// §9.3.2: a fan-out writes the round's fan-out directories and
	// §2.6.1.6's ledger entries, and every prompt it emits shows a unit
	// formed at the round's head. A head that moved under the round
	// refuses here, naming both heads.
	if err := round.RefuseStale(); err != nil {
		return nil, err
	}
	meta := round.Meta
	r := &Round{Round: meta.Round, Head: meta.Head, Proximity: src.Config.Int("threads.proximity_lines")}
	records, err := r.read(src, &meta)
	if err != nil {
		return nil, err
	}
	p, axes, err := gate(src, r, &meta)
	if err != nil {
		return nil, err
	}
	hunks, texts, err := diffOf(src, meta.Head)
	if err != nil {
		return nil, err
	}
	if r.Units, err = withHunks(src, &meta, records, hunks, texts); err != nil {
		return nil, err
	}
	halves, err := r.attach(src, p, hunks)
	if err != nil {
		return nil, err
	}
	if err := r.fanOut(src); err != nil {
		return nil, err
	}
	lenses, err := lensesOf(src, axes, halves, &meta)
	if err != nil {
		return nil, err
	}
	return &Fanout{
		Round: r.Round, Head: r.Head, Prompts: Emit(r),
		Honesty:  sentences(lenses),
		Expected: coverage.Expect(unitIDs(r.Units), r.Active), Skipped: lenses.Roles,
	}, nil
}

// gate loads the round's profile, re-derives the axis activation every §4.5.4
// report of this round is written from, and applies §4.6.5's refusal before a
// single attachment is gathered.
//
// The three are one step because the refusal is a function of the other two: it
// is owed only where an intent role exists to produce the mapping, and whether
// one exists is what the activation says. Running them apart would let a caller
// gather a round's attachments and then discover the round could not be
// reviewed.
func gate(src *Sources, r *Round, meta *state.Meta) (*profile.Profile, activation.Activation, error) {
	p, err := profileOf(src.Layout, meta.ProfileID)
	if err != nil {
		return nil, activation.Activation{}, err
	}
	axes := activation.OfRound(
		p, meta.ProfileID, meta.IssueKey, src.Config.String("intent.key_pattern"))
	return p, axes, refuseWithoutMapping(src, r, axes)
}

// refuseWithoutMapping is §4.6.5's last sentence: the remaining axes are refused
// until a mapping exists for the current round and head.
//
// Two invocations pass regardless. The intent pass itself is what produces the
// mapping, so refusing it would leave the round with no way forward; and a round
// whose intent axis did not run has no intent role and no mapping to produce, so
// §4.6.6 has the mapping treated as empty and there is nothing to wait for.
// §4.6.6 names §4.5.3's unavailability, and the activation is asked for the axis
// rather than for that one reason because a disabled intent axis leaves the
// round in exactly the same place: no role, no `cr map record`, and a refusal
// that could never be lifted.
//
// The round scoping and the head are already settled by the time this is
// reached. RefuseStale above refuses a round whose head moved, and r.Mapped is
// meta.json's mapping stamp compared against the round and head meta.json
// records, so a mapping recorded in an earlier round is not one this round has.
//
// Existence is read off that stamp and not off the round's files. §4.1.1 lets
// every unit be mapped to zero claims, so an empty mapping holds no pair and,
// over a round with no claim, derives no §4.1.7 entry either — byte-identical
// to a mapping nobody recorded. Round 9's mapping-existence-unobservable and
// round 12's empty-vs-absent-mapping are that gap, and `cr map record` stamping
// the round and head is what closes it: an inference over the files either
// unblocked a round nobody mapped or held one whose mapping was empty.
func refuseWithoutMapping(src *Sources, r *Round, axes activation.Activation) error {
	if r.Mapped || src.Axis == axis.Intent || !slices.Contains(axes.Active, axis.Intent) {
		return nil
	}
	return &MappingRequiredError{
		Round: r.Round, Head: r.Head,
		Owner: src.Owner, Repo: src.Repo, PR: src.PR, Axis: src.Axis,
	}
}

// lensesOf is §4.5.4's report for this round, assembled by the constructor
// `cr status` assembles its own with.
//
// coverage.RoundLenses is the one assembly, so the axes, halves and roles this
// reports and the ones §10.1.3 reports are one answer to one question. Two
// assemblies would each be internally consistent and could still carry
// different kinds, and a reader has no way to see that from either command alone.
//
// The activation is gate's, taken as an argument rather than re-derived here,
// so the axes §4.6.5's refusal was decided against and the axes this report is
// written from are one answer. The active role set is meta.json's rather than a
// recomputation, for the reason coverage.Skipped gives.
func lensesOf(
	src *Sources, axes activation.Activation, halves []finding.HonestyDisclosure, meta *state.Meta,
) (coverage.Lenses, error) {
	corpus, err := role.Resolve(src.Layout.RepoRolesDir(src.Owner, src.Repo), src.Layout.RolesDir())
	if err != nil {
		return coverage.Lenses{}, err
	}
	return coverage.RoundLenses(axes, halves, corpus, meta.ActiveRoles, meta.ProfileID), nil
}

// sentences renders §4.5.4's report as the lines §11.1 exempts from `--quiet`.
//
// The collecting is coverage.Lenses' rather than an append written out here,
// for the reason that type gives: a kind added to the collector reaches this
// command's honesty channel with it, instead of reaching whichever call site
// somebody remembered. §4.6.4's roles are the kind that arrived that way.
func sentences(lenses coverage.Lenses) []string {
	disclosed := lenses.Disclosures()
	out := make([]string, 0, len(disclosed))
	for _, entry := range disclosed {
		out = append(out, entry.Disclosure())
	}
	return out
}

// unitIDs is the round's unit ids in §3.4.6's order, which is the order the
// records were read in.
func unitIDs(units []Unit) []string {
	ids := make([]string, 0, len(units))
	for i := range units {
		ids = append(ids, units[i].ID)
	}
	return ids
}

// roleIDs is a role set's ids, in the order the set holds them.
func roleIDs(roles []role.Role) []string {
	ids := make([]string, 0, len(roles))
	for i := range roles {
		ids = append(ids, roles[i].ID)
	}
	return ids
}

// read fills the attachments that come out of the round's state: the active
// roles, the claims, the mapping, the threads, the notes, and §4.1's items. It
// returns the round's unit records, which the diff is matched against next.
func (r *Round) read(src *Sources, meta *state.Meta) ([]unit.Record, error) {
	active, places, err := activeRoles(src, meta.ActiveRoles)
	if err != nil {
		return nil, err
	}
	l, owner, repo, pr := src.Layout, src.Owner, src.Repo, src.PR
	records, err := state.ReadStamped[unit.Record](l, owner, repo, pr, state.FileUnits, r.Round)
	if err != nil {
		return nil, err
	}
	if r.Claims, err = state.ReadStamped[intent.Claim](
		l, owner, repo, pr, state.FileClaims, r.Round); err != nil {
		return nil, err
	}
	if r.Pairs, err = state.ReadStamped[mapping.Pair](
		l, owner, repo, pr, state.FileMapping, r.Round); err != nil {
		return nil, err
	}
	if r.Threads, err = gh.ReadThreads(l, owner, repo, pr); err != nil {
		return nil, err
	}
	if r.Held, err = state.ReadRecords[finding.Finding](l, owner, repo, pr, state.FileFindings); err != nil {
		return nil, err
	}
	notes, err := notesOf(l, meta.IssueKey)
	if err != nil {
		return nil, err
	}
	r.Notes = standingNotes(notes)
	r.Active = roleIDs(active)
	r.Places = places
	r.Roles = onAxis(active, src.Axis)
	// meta.json's stamp, not the pairs: an empty mapping holds no pair and
	// is still a mapping this round recorded, per round 9's
	// mapping-existence-unobservable.
	r.Mapped = meta.MappingRecorded()
	_, r.IntentUnavailable = intent.Recorded(meta.IssueKey, "").Unavailability()
	// §4.6.5's second pass, and the only invocation that emits over fewer
	// than every unit. It is the intent axis re-run once the mapping is
	// stored, which is exactly where "the units mapped to zero claims" is a
	// question the round can answer; an unscoped `cr review` is §4.6.1's
	// full cross product whether or not a mapping exists.
	r.SecondPass = r.Mapped && src.Axis == axis.Intent
	r.Unmapped, err = r.raise(src, records, notes, onAxis(active, axis.Intent))
	return records, err
}

// raise is §4.1.2 and §4.1.5 over the round, once a mapping has been recorded
// for it. Before one is, no unit is known to be unmapped (§4.6.5), and raising
// every unit as a question would put a question about each of them to the
// author on the strength of a mapping nobody has made yet. A round whose intent
// axis is unavailable raises nothing either, whatever meta.json's stamp says:
// §4.6.6 treats its mapping as empty, and an absent tracker is not an unmapped
// unit.
func (r *Round) raise(
	src *Sources, records []unit.Record, notes []note.Note, intentRoles []role.Role,
) ([]UnmappedUnit, error) {
	if !r.Mapped || r.IntentUnavailable {
		return []UnmappedUnit{}, nil
	}
	cells, err := state.ReadStamped[coverage.Cell](
		src.Layout, src.Owner, src.Repo, src.PR, state.FileCoverage, r.Round)
	if err != nil {
		return nil, err
	}
	ids := make([]string, 0, len(records))
	for i := range records {
		ids = append(ids, records[i].ID)
	}
	return Raise(&IntentRound{
		Round: r.Round, Units: ids, Pairs: r.Pairs, Notes: notes, Cells: cells,
		IntentRoles: roleIDs(intentRoles),
	}), nil
}

// activeRoles is §4.5.1's active set as `cr brief` settled it in meta.json,
// resolved against §2.5.5's corpus and kept in corpus order, together with the
// rows blockPlaces lays that corpus's id blocks out in.
func activeRoles(src *Sources, active []string) (roles []role.Role, places []string, err error) {
	corpus, err := role.Resolve(src.Layout.RepoRolesDir(src.Owner, src.Repo), src.Layout.RolesDir())
	if err != nil {
		return nil, nil, err
	}
	roles = make([]role.Role, 0, len(active))
	for i := range corpus {
		if slices.Contains(active, corpus[i].Role.ID) {
			roles = append(roles, corpus[i].Role)
		}
	}
	return roles, blockPlaces(corpus), nil
}

// onAxis keeps the roles of one axis, and every role when the axis is empty.
func onAxis(roles []role.Role, id string) []role.Role {
	kept := make([]role.Role, 0, len(roles))
	for i := range roles {
		if id == "" || roles[i].Axis == id {
			kept = append(kept, roles[i])
		}
	}
	return kept
}

// notesOf loads §3.6's store for the round's issue key, and no store at all
// when §3.2 resolved none: a lookup under no key would name the context
// directory plus an extension.
func notesOf(l state.Layout, key string) ([]note.Note, error) {
	if key == "" {
		return []note.Note{}, nil
	}
	return note.Load(l, key)
}

// diffOf takes §3.4.1's diff at the round's head and reads its hunks and their
// texts out of the one patch.
func diffOf(src *Sources, head string) ([]git.Hunk, []string, error) {
	pr, err := src.GH.PullRequest(src.Owner, src.Repo, src.PR)
	if err != nil {
		return nil, nil, err
	}
	diff, err := git.DiffAgainstMergeBase(src.RepoDir, pr.Base, head)
	if err != nil {
		return nil, nil, err
	}
	hunks, err := git.ParseHunks(diff.Patch)
	if err != nil {
		return nil, nil, err
	}
	texts, err := git.HunkTexts(diff.Patch)
	if err != nil {
		return nil, nil, err
	}
	return hunks, texts, nil
}

// withHunks gives every recorded unit the hunks its ranges name, and refuses a
// unit the diff cannot rebuild whole.
//
// A hunk belongs to a unit when it is on the unit's file and side and its
// head-side range is one the unit recorded — the coordinates §3.4.6 stores a
// range in, taken from git.Hunk.HeadRange as the record was.
func withHunks(
	src *Sources, meta *state.Meta, records []unit.Record, hunks []git.Hunk, texts []string,
) ([]Unit, error) {
	units := make([]Unit, 0, len(records))
	for i := range records {
		formed := Unit{Unit: records[i].Unit, Hunks: make([]git.Hunk, 0), Texts: make([]string, 0)}
		for at := range hunks {
			if belongs(&formed.Unit, &hunks[at]) {
				formed.Hunks = append(formed.Hunks, hunks[at])
				formed.Texts = append(formed.Texts, texts[at])
			}
		}
		if len(formed.Hunks) != len(formed.HunkRanges) {
			return nil, &StaleUnitError{
				Unit: formed.ID, Round: meta.Round, Head: meta.Head,
				Owner: src.Owner, Repo: src.Repo, PR: src.PR,
			}
		}
		units = append(units, formed)
	}
	return units, nil
}

// belongs reports whether a hunk is one of the unit's.
func belongs(u *unit.Unit, hunk *git.Hunk) bool {
	if hunk.Path != u.Path || hunk.Side != u.Side {
		return false
	}
	start, end := hunk.HeadRange()
	return slices.Contains(u.HunkRanges, unit.Range{Start: start, End: end})
}

// fanOut gives every unit the directory §4.6.2's output files sit in, and
// creates those directories under the pull request's lock (§2.3.1).
//
// Directories are all `cr review` writes, and nothing it writes is a file: the
// records are the roles' to write, and §4.6.3 has `cr review` write nothing to
// findings.ndjson. They sit under the state root per §2.2, so a role told where
// to write is never told to write inside the repository under review.
func (r *Round) fanOut(src *Sources) error {
	ids := make([]string, 0, len(r.Units))
	for i := range r.Units {
		r.Units[i].FanOut = src.Layout.FanOutDir(src.Owner, src.Repo, src.PR, r.Round, r.Units[i].ID)
		ids = append(ids, r.Units[i].ID)
	}
	held, err := src.Layout.LockPR(src.Owner, src.Repo, src.PR)
	if err != nil {
		return err
	}
	if err := held.EnsureFanOut(r.Round, ids); err != nil {
		// The lock is released on the way out of every branch, and the
		// write's own failure is what the caller is told about.
		_ = held.Unlock()
		return err
	}
	return held.Unlock()
}

// profileOf loads the profile the round resolved to, and the empty profile of
// §2.4.4 when none matched. An empty profile declares no language, no test
// glob, and no rule, so every lens that needs one reports itself unavailable
// rather than being given a guess.
func profileOf(l state.Layout, id string) (*profile.Profile, error) {
	if id == "" {
		return &profile.Profile{}, nil
	}
	loaded, err := profile.Load(l.Profile(id))
	if err != nil {
		return nil, err
	}
	return &loaded, nil
}
