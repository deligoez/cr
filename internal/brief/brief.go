// Package brief assembles and persists the orientation payload of
// spec/0.1.0.md §3.7.
//
// §3.7 is read-only with respect to judgement: nothing here forms a finding, a
// coverage cell, a probe, or a thread, and nothing here reaches the network to
// write. What it does is fetch, compute, and record facts about the pull
// request — the units of §3.4, the threads of §3.5, the key of §3.2, the claims
// already recorded, and which axes of §4.5 can look at all.
//
// The persistence is an obligation rather than the permission §3.7 words it as,
// and state.NotBriefedError says why: §4.1.6 and §4.5.6 reject a mapping or a
// cell naming an unknown unit id, §9.3.1 and §9.3.3 read meta.json's recorded
// head, and §3.5.3 attaches the ingested threads. Every one of them treats these
// files as authoritative, so if `cr brief` does not write them nothing can.
//
// §9.3 is the other half, and it is here because §9.3.2 makes `cr brief` the
// only way forward from a moved head. §9.3.3's increment is roundOf's, and
// §9.3.4's invalidation is invalidate.go's: on that increment, and only on it,
// the records the closing round left open move to `stale`, mapping.ndjson is
// cleared for the round being opened, and the claims are carried into it
// unchanged. The three happen in one critical section with the writes below,
// because half of them would leave a round whose index says one thing and whose
// records say another.
package brief

import (
	"errors"
	"io/fs"
	"slices"

	"github.com/deligoez/cr/internal/activation"
	"github.com/deligoez/cr/internal/config"
	"github.com/deligoez/cr/internal/finding"
	"github.com/deligoez/cr/internal/gh"
	"github.com/deligoez/cr/internal/git"
	"github.com/deligoez/cr/internal/intent"
	"github.com/deligoez/cr/internal/note"
	"github.com/deligoez/cr/internal/profile"
	"github.com/deligoez/cr/internal/role"
	"github.com/deligoez/cr/internal/state"
	"github.com/deligoez/cr/internal/symbol"
	"github.com/deligoez/cr/internal/unit"
)

// The layers §2.4.1 admits as having selected a profile, which §3.7.1 has the
// brief print alongside the profile itself.
//
// §2.4.1 states two: selection is automatic via `match.files`, and overridable
// by `profile` in the per-repository config. The third is §2.4.4's state, where
// nothing matched and every axis needing a profile is switched off rather than
// guessed at.
const (
	// LayerConfigured is the `profile` setting of §2.7, which §2.4.1 makes
	// the override.
	LayerConfigured = "configuration"
	// LayerMarkerFiles is §2.4.1's automatic selection.
	LayerMarkerFiles = "match.files"
	// LayerNone is §2.4.4: no profile matched this repository.
	LayerNone = "none"
)

// ProfileReport is §3.7.1's "resolved profile together with the layer that
// selected it per §2.4".
//
// The layer is the mechanism §2.4.1 names and not the file the value came from.
// internal/config resolves the five layers of §2.7 into one value and keeps no
// record of which supplied it, and inventing one here would mean a second
// resolution that can disagree with the one the run actually used.
type ProfileReport struct {
	// ID is the resolved profile's id, empty when none matched.
	ID string `json:"id"`
	// Selected reports whether a profile was resolved at all.
	Selected bool `json:"selected"`
	// SelectionLayer is one of the three constants above. It is spelled
	// with a qualifier because §2.5.5's own `Layer` is fenced inside
	// internal/role, and the two layers are different things: that one is
	// where a role file was found, this one is how a profile was chosen.
	SelectionLayer string `json:"layer"`
	// Missing is §2.4.4's report, present only when nothing matched. It is
	// a pointer because its absence and a report naming nothing are
	// different facts.
	Missing *profile.MissingProfile `json:"missing,omitempty"`
}

// IssueReport is §3.7.2: the key, the source §3.2 resolved it from, and either
// the issue text or the reason there is no key.
type IssueReport struct {
	// Key is §3.2's resolution, empty when no source yielded one.
	Key string `json:"key"`
	// Origin names which of §3.2's four sources yielded it.
	Origin intent.KeyOrigin `json:"origin"`
	// Text is the issue text read for Key, empty when there is no key.
	Text string `json:"text"`
	// Reason is why no key was found, and what would make one. It is
	// §4.5.3's own reason rather than a second wording of it, so the
	// sentence the reader sees here and the sentence the axis report
	// carries are one string.
	Reason string `json:"reason"`
}

// Brief is §3.7's payload: the six items, in the order §3.7 numbers them.
type Brief struct {
	// §3.7.1: PR identity, head, merge base, and the resolved profile.
	Owner     string        `json:"owner"`
	Repo      string        `json:"repo"`
	PR        int           `json:"pr"`
	Round     int           `json:"round"`
	Head      string        `json:"head"`
	MergeBase string        `json:"merge_base"`
	Profile   ProfileReport `json:"profile"`
	// §3.7.2: the issue key, its source, and the text or the reason.
	Issue IssueReport `json:"issue"`
	// §3.7.3: the claims already recorded for this round, and §3.3.3's
	// drift comparison against the issue text as it now reads.
	Claims []intent.Claim `json:"claims"`
	Drift  intent.Drift   `json:"drift"`
	// §3.7.4: the units of §3.4, with their paths, hunk ranges, and hashes.
	Units []unit.Unit `json:"units"`
	// §3.7.5: the ingested threads of §3.5 and the notes of §3.6.
	Threads []gh.Thread `json:"threads"`
	Notes   []note.Note `json:"notes"`
	// §3.7.6: the active, disabled, and unavailable axes with their
	// reasons.
	Axes activation.Activation `json:"axes"`
	// ActiveRoles is §4.5.1's role half over the corpus of §2.5.5, and
	// the value meta.json's `active_roles` carries. It sits beside Axes
	// because §4.5.1 defines the two together and one run must answer both
	// from the same axis decision and the same resolved profile.
	ActiveRoles []string `json:"active_roles"`
	// round is §9.3.3's decision for this run: the round above, and the
	// round it was opened from. §9.3.4 reads both — whether an increment
	// happened at all, and which round's records the increment invalidates.
	//
	// It is unexported because it is a fact about the run rather than an
	// item of §3.7's payload: the six items are the fields above, and a
	// seventh on the wire would be normative surface the spec never asked
	// for. It travels on the value all the same, because `persist` is
	// handed that value and nothing else, and the alternative — deciding a
	// second time from meta.json — is the second read of the round
	// §9.3.1's door does not admit.
	round roundState
}

// Disclosures collects §3.7.6's report as the disclosure contract §11.1 exempts
// from `--quiet`, so the writer that holds the exemption is handed one slice and
// cannot forget a category.
//
// §2.4.4's report comes last and separately because it is not an axis report.
// activation.Activate is only asked when a profile was resolved — its own
// documentation says deriving a second answer for the repository no profile
// matched would give one run two reports that can disagree — so when nothing
// matched, profile.MissingProfile is the report, and the intent axis's own
// unavailability is added to Axes beside it.
func (b *Brief) Disclosures() []finding.HonestyDisclosure {
	out := b.Axes.Disclosures()
	if b.Profile.Missing != nil {
		out = append(out, *b.Profile.Missing)
	}
	return out
}

// Sources are the inputs one orientation run reads.
//
// Everything that touches the outside world is a field rather than a call made
// inside Run: the GitHub client is the one internal/gh already makes
// injectable, the repository is a directory, and the configuration has been
// resolved by the caller. That is what lets §3.7 be exercised against a real
// repository with no token and no network.
type Sources struct {
	// Layout resolves §2.2's paths and holds the per-PR state.
	Layout state.Layout
	// GH reads the pull request and its threads.
	GH gh.Client
	// Config is §2.7's resolved configuration.
	Config config.Config
	// Owner, Repo, and PR name the pull request under review.
	Owner string
	Repo  string
	PR    int
	// RepoDir is the repository under review. Every read of it goes
	// through internal/git, and nothing here writes into it.
	RepoDir string
	// IssueFlag is `--issue`, §3.2's first source.
	IssueFlag string
	// Intent is §3.1's source for the issue text: the configured tracker
	// command, or the file §3.1.4 replaces it with.
	Intent intent.Source
}

// Run assembles §3.7's payload and records the derived inputs the rest of cr
// reads as authoritative.
//
// Everything is read before anything is written. The fetches can each fail —
// GitHub can refuse, a revision can be absent from the checkout, a profile can
// be malformed — and a run that had already published half the state directory
// would leave a round oriented on inputs it never finished computing.
func Run(src *Sources) (*Brief, error) {
	assembled, err := assemble(src)
	if err != nil {
		return nil, err
	}
	if err := persist(src, assembled); err != nil {
		return nil, err
	}
	return assembled, nil
}

// assemble carries out §3.7's six items and writes nothing.
//
//nolint:funlen // measured 2026-08-31 at 68 lines; refactor to clear, never raise the limit
func assemble(src *Sources) (*Brief, error) {
	pr, err := src.GH.PullRequest(src.Owner, src.Repo, src.PR)
	if err != nil {
		return nil, err
	}
	resolved, err := intent.Resolve(intent.KeySources{
		Flag:   src.IssueFlag,
		Branch: pr.HeadRefName,
		Title:  pr.Title,
		Body:   pr.Body,
	}, src.Config.String("intent.key_pattern"), src.Intent)
	if err != nil {
		return nil, err
	}
	selection, err := profile.Select(
		src.Layout.ProfilesDir(), src.RepoDir, src.Config.String("profile"))
	if err != nil {
		return nil, err
	}
	// §2.4.2: a tie is named and the command stops, rather than one of the
	// tied profiles being picked.
	if err := selection.Err(); err != nil {
		return nil, err
	}
	units, mergeBase, err := unitsOf(src, &selection.Profile, pr.Base, pr.Head)
	if err != nil {
		return nil, err
	}
	threads, err := src.GH.Threads(src.Owner, src.Repo, src.PR)
	if err != nil {
		return nil, err
	}
	round, err := roundOf(src, pr.Head)
	if err != nil {
		return nil, err
	}
	// The claims read are the ones going in, which on an increment belong
	// to the round being closed: §9.3.4 carries them forward unchanged, so
	// they are this round's claims and `persist` is what re-records them.
	claims, err := claimsOfRound(src, round.carries)
	if err != nil {
		return nil, err
	}
	// §3.3.3's comparison runs on every round, over the claims already
	// recorded and the issue text as it now reads. It reports and never
	// re-extracts, which is a property of intent.DetectDrift's signature
	// rather than a rule remembered here.
	drift, err := intent.DetectDrift(slices.Values(claims), resolved.Text)
	if err != nil {
		return nil, err
	}
	notes, err := notesOf(src, resolved.Key.Value)
	if err != nil {
		return nil, err
	}
	// §2.5.5's corpus, resolved from the two on-disk layers of §2.2 and
	// the built-ins. It is read here rather than by whatever consumes the
	// active set, so §4.5.1's two inputs — the axis decision above and the
	// resolved profile — meet the roles in one place and one round cannot
	// answer the question twice.
	corpus, err := role.Resolve(
		src.Layout.RepoRolesDir(src.Owner, src.Repo), src.Layout.RolesDir())
	if err != nil {
		return nil, err
	}
	axes := axesOf(&selection, resolved)
	return &Brief{
		Owner:       src.Owner,
		Repo:        src.Repo,
		PR:          src.PR,
		Round:       round.index,
		round:       round,
		Head:        pr.Head,
		MergeBase:   mergeBase,
		Profile:     profileReport(&selection, src.Config.String("profile")),
		Issue:       issueReport(resolved),
		Claims:      claims,
		Drift:       drift,
		Units:       units,
		Threads:     threads,
		Notes:       notes,
		Axes:        axes,
		ActiveRoles: axes.ActiveRoles(corpus, selection.Profile.ID),
	}, nil
}

// unitsOf carries out §3.4 for one head: the diff against the merge base, the
// hunks, §3.4.4's clustering, §3.4.5's split, and §3.4.6's records. It returns
// the merge base alongside, because §3.7.1 prints the commit the diff was
// actually taken against.
//
// The symbol index is §4.3.1's head index, the one `cr review` attaches
// candidates from, so §3.4.4's symbol branch reads the same declarations the
// reinvention lens does.
func unitsOf(src *Sources, p *profile.Profile, base, head string) ([]unit.Unit, string, error) {
	diff, err := git.DiffAgainstMergeBase(src.RepoDir, base, head)
	if err != nil {
		return nil, "", err
	}
	hunks, err := git.ParseHunks(diff.Patch)
	if err != nil {
		return nil, "", err
	}
	index, err := headSymbols(src.RepoDir, head, p)
	if err != nil {
		return nil, "", err
	}
	clusters := unit.Clusters(hunks, p, index, src.Config.Int("cluster.gap_lines"))
	units, err := unit.Units(unit.Split(clusters, src.Config.Int("cluster.max_lines")))
	if err != nil {
		return nil, "", err
	}
	return units, diff.MergeBase, nil
}

// headSymbols is the head index as unit.Clusters takes it: nil when the profile
// declares no language cr can index, which unit.Detectable reads as no index
// and §3.4.3 falls through to adjacency on without an error. The nil is
// returned as an interface value and never as a nil *symbol.Index inside one,
// which a nil check would not see.
func headSymbols(dir, head string, p *profile.Profile) (unit.SymbolIndex, error) {
	index, built, err := symbol.Head(dir, head, p)
	if err != nil || !built {
		return nil, err
	}
	return index, nil
}

// roundOf is §9.3.3's decision: the round this brief orients — the one already
// open, §9.3.3's first, or the next one when the head has moved — together with
// the round whose records §9.3.4 carries into it.
//
// A pull request cr holds no state for is round 1, and so is one whose
// meta.json carries the 0 state.Layout.EnsurePR writes — §9.3.3 numbers rounds
// from 1, so that 0 means no round has been opened rather than a round of its
// own.
//
// Past that, §9.3.3 makes the comparison against the recorded head the whole of
// the decision: the index moves if and only if the current head differs, so a
// same-head brief returns the index it read and the round it re-orients is the
// one already open. That is what makes the rest of the command idempotent
// without anything else in it having to ask whether it has run before — every
// file `persist` writes is a function of the head and the round, so an
// unchanged pair rewrites the same bytes.
//
// The head is passed rather than read here because §3.7.1 has already fetched
// it. Reading it a second time would let one run orient on one head and
// compare against another.
//
// A meta.json that is there and cannot be read is neither of those and is not
// swallowed. Reading it as "no round" would have the write below publish a
// fresh round 1 over a state directory whose history cr just failed to
// understand.
func roundOf(src *Sources, head string) (roundState, error) {
	recorded, err := src.Layout.ReadMeta(src.Owner, src.Repo, src.PR)
	switch {
	case errors.Is(err, fs.ErrNotExist):
		return roundState{index: 1, carries: 1}, nil
	case err != nil:
		return roundState{}, err
	case recorded.Round < 1:
		return roundState{index: 1, carries: 1}, nil
	case recorded.Head != head:
		return roundState{index: recorded.Round + 1, carries: recorded.Round}, nil
	}
	return roundState{index: recorded.Round, carries: recorded.Round}, nil
}

// roundState is §9.3.3's decision about one brief, which §9.3.4 then reads
// twice over.
//
// The two fields are one comparison, so they are returned together rather than
// re-derived. §9.3.4 needs to know both that a round was opened — an increment
// is what its sweep, its clearing and its carry-forward hang on — and which
// round's claims are the ones to carry, and a second answer to either question
// would be a second reading of `meta.json` the §9.3.1 door does not admit.
type roundState struct {
	// index is the round this brief orients, and the round every record
	// it writes is stamped with.
	index int
	// carries is the round whose records are the current ones going in:
	// the recorded round when this brief opened a new one, and index
	// itself otherwise. §9.3.4 carries that round's claims forward.
	carries int
}

// opened reports whether this brief incremented the round index per §9.3.3,
// which is the condition every clause of §9.3.4 is written under.
func (r roundState) opened() bool {
	return r.index != r.carries
}

// claimsOfRound reads the claims recorded for one round, per §9.3.5: a command
// reads only the current round's records, and earlier rounds are history.
//
// A pull request with no state directory holds no claims, which is the same
// answer an empty file gives, so an absent file is not a failure: on a first
// brief there is nothing to read and nothing to report.
func claimsOfRound(src *Sources, round int) ([]intent.Claim, error) {
	claims, err := state.ReadStamped[intent.Claim](
		src.Layout, src.Owner, src.Repo, src.PR, state.FileClaims, round)
	if errors.Is(err, fs.ErrNotExist) {
		return []intent.Claim{}, nil
	}
	if err != nil {
		return nil, err
	}
	return claims, nil
}

// notesOf loads §3.6's store for the resolved key, and no store at all when
// §3.2 resolved none.
//
// The empty key is not a lookup with an empty answer: §2.2 stores the context
// store at context/<ISSUE-KEY>.ndjson, so asking for one under no key would name
// a file whose path is the directory plus an extension.
func notesOf(src *Sources, key string) ([]note.Note, error) {
	if key == "" {
		return []note.Note{}, nil
	}
	return note.Load(src.Layout, key)
}

// profileReport answers §3.7.1's second half.
func profileReport(selection *profile.Selection, configured string) ProfileReport {
	missing, none := selection.Missing()
	if none {
		return ProfileReport{SelectionLayer: LayerNone, Missing: &missing}
	}
	layer := LayerMarkerFiles
	if configured != "" {
		layer = LayerConfigured
	}
	return ProfileReport{ID: selection.Profile.ID, Selected: true, SelectionLayer: layer}
}

// issueReport answers §3.7.2, taking the no-key reason from §4.5.3's own entry
// so the brief and the axis report cannot word it differently.
func issueReport(resolved intent.Intent) IssueReport {
	report := IssueReport{
		Key:    resolved.Key.Value,
		Origin: resolved.Key.Origin,
		Text:   resolved.Text,
	}
	if unavailable, marked := resolved.Unavailability(); marked {
		report.Reason = unavailable.Reason
	}
	return report
}

// axesOf answers §3.7.6.
//
// activation.Activate is asked only when a profile was resolved, because that
// is what its own contract requires: §2.4.4's repository is profile.Selection's
// to report, and a second answer derived here could disagree with it about
// which axes looked. What is still true with no profile is §4.5.3, so the
// intent axis's unavailability is carried whichever branch runs.
func axesOf(selection *profile.Selection, resolved intent.Intent) activation.Activation {
	if selection.Selected {
		return activation.Activate(&selection.Profile, resolved)
	}
	axes := activation.Activation{
		Active:      []string{},
		Disabled:    []activation.Disabled{},
		Unavailable: []intent.Unavailable{},
	}
	if unavailable, marked := resolved.Unavailability(); marked {
		axes.Unavailable = append(axes.Unavailable, unavailable)
	}
	return axes
}
