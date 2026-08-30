package finding

import (
	"fmt"

	"github.com/deligoez/cr/internal/git"
)

// WaiverKey is §7.4.1's waiver key: what a waiver waives, and — because §9.3.6
// keys the posted index "exactly as a §7.4.1 waiver is" — what one entry of
// `posted-index.ndjson` holds as well.
//
// §7.4.1 writes three of the four fields: `(anchor.path, class, normalised hash
// of the anchored lines)`. `side` is the fourth, from round 8's finding
// side-omitted-from-identity-keys. §9.2 makes side part of an anchor and §6.1.2
// resolves the two sides against different trees, so without it a waiver written
// over a line removed from the merge base also silences a finding about the line
// that replaced it in the head — two records about two texts in two trees, which
// look like one only because the file and the class agree.
//
// The hash is the anchor's own `content_hash` rather than a second computation
// of it. §9.2 defines that field as the normalised hash per §1.4 of the lines
// from `start_line` to `line` inclusive, which is the value §7.4.1 asks for word
// for word, and AnchorContentHash is the single place it is produced — so the
// key and the anchor cannot end up as two answers about the same lines.
//
// Everything else a record carries is left out, the summary loudest: §7.4.1
// excludes it in as many words, because a summary is agent-composed prose that
// differs between rounds and a key carrying it would let a waived finding
// resurface under a rewording. The same reasoning covers the role, the grade,
// the severity and the id, which say who produced the record rather than what it
// is about — §7.4.1 keys by class "so a waived finding cannot return under a
// different producer".
//
// It is not §6.4.1's dedup key, which carries `anchor.line` where this carries
// the content hash. Dedup groups records within one round against one head,
// where a line number names code; a waiver outlives the round and has to follow
// the code as the file above it moves, which is what §7.4.2's "the same
// unchanged code" means.
//
// The type is comparable, and equality across all four fields is the whole of
// what matching means. There is deliberately no looser comparison: round 8's
// approved decision dropped §7.4.6's path-prefix widening from v0.1, so a waiver
// never reaches past the anchored lines it was written over.
//
// It lives in this package, beside the anchor and the class it is made of,
// rather than beside the waiver store §7.4.4 will write: §9.3.6's posted index
// is not a waiver, and it must not have to import one in order to be keyed like
// one.
type WaiverKey struct {
	// Path is the anchored file, from `anchor.path`.
	Path string `json:"path"`
	// Side is the tree the anchored lines are counted in, from
	// `anchor.side`: RIGHT in the head, LEFT in the merge base (§9.2.1).
	Side git.Side `json:"side"`
	// Class is the record's defect class, kebab-case per §6.1 and held to
	// that form by ValidateClass — two spellings of one class would be two
	// keys, and the waiver would miss.
	Class string `json:"class"`
	// ContentHash is the anchor's content hash: §9.2's normalised hash of
	// the anchored lines taken as one text.
	ContentHash string `json:"content_hash"`
}

// WaiverKeyOf reads one record's waiver key.
//
// It is the only construction of that key, so the waiver §7.2 writes at triage,
// the posted-index entry §9.3.6 appends at posting, and the merge-time lookup
// §6.4.4 drops a finding against are all reading the same four fields off the
// same record. Two of the three would otherwise spell the key out where they
// need it, and a fifth field added to one of those spellings would silently stop
// the other from ever matching.
//
// The record is taken by pointer because it is a wide struct and nothing here
// writes to it.
func WaiverKeyOf(record *Finding) WaiverKey {
	return WaiverKey{
		Path:        record.Anchor.Path,
		Side:        record.Anchor.Side,
		Class:       record.Class,
		ContentHash: record.Anchor.ContentHash,
	}
}

// WaiverScope is which of §7.4.4's two files a waiver lives in, and so how far
// the silence it buys reaches: the whole repository, or this pull request alone.
//
// It follows the disposition and nothing else. §7.4.1 binds the two — a waiver
// written for `wrong` is scoped to the repository, one written for `not-here` to
// the pull request — and §7.4.3 gives the reason. "This is wrong" is a fact
// about the class and generalises across the repository; "not worth saying
// here" is a fact about this pull request and MUST NOT silence the finding
// anywhere else. A `not-here` that reached repository scope would suppress a
// true finding in every future pull request of that repository, and nothing the
// reviewer could read would say why.
//
// So a scope is not a field a caller fills in. This is a struct around an
// unexported name for the reason State gives in state.go: a defined string type
// is open, and `var s WaiverScope = "repository"` would compile anywhere in the
// tree — which is exactly the write §7.4.3 forbids. Here the two values below
// can be named but not made, and Scope is the only thing that returns one.
//
// There is deliberately no parser and no JSON form. A scope is never stored
// beside the disposition it is derived from, because two fields that can
// disagree are one field and one bug — and the one that would be believed is the
// stored scope, which no longer knows what it was written for.
type WaiverScope struct{ name string }

// The two scopes of §7.4.1, each named for the file of §7.4.4 it selects.
var (
	// ScopeRepository is `~/.cr/waivers/<owner>/<repo>.ndjson`: what a
	// `wrong` disposition writes, and it holds across the repository.
	ScopeRepository = WaiverScope{"repository"}
	// ScopePullRequest is that pull request's `waivers.ndjson` per §2.3:
	// what a `not-here` disposition writes, and it holds nowhere else.
	ScopePullRequest = WaiverScope{"pull-request"}
)

// String returns the scope's name, which is what §7.4.7 prints beside a waiver's
// disposition.
func (s WaiverScope) String() string {
	return s.name
}

// Waiver is one silence a reviewer chose: the §7.4.1 key naming what it covers,
// and the §7.2 disposition saying why.
//
// The disposition is recorded per §7.4.5, so §7.3 can tell a false positive from
// a deliberate silence. §7.3.4 puts `discarded-wrong` in the demotion numerator
// and excludes `not-here` in as many words, so a waiver that recorded only that
// something was silenced would make a class which is always right and merely
// never worth saying look like a class which is wrong.
//
// It carries no scope of its own: Scope derives one, and §7.4.4's two files
// follow from that. The provenance §7.4.8 asks for — the round, the pull
// request, the head, and the reason when one was given — belongs to the waiver
// store that writes those files, not to the identity of the waiver itself.
type Waiver struct {
	// WaiverKey is what the waiver covers, per §7.4.1.
	WaiverKey
	// Disposition is why the record was discarded, per §7.2, recorded
	// because §7.4.5 requires it.
	Disposition Disposition `json:"disposition"`
}

// WaiverFor writes the waiver a discarded record calls for.
//
// Both halves are read off the one record — the key through WaiverKeyOf, the
// disposition §7.2 set when the block was deleted or marked `wrong` — so a
// waiver cannot end up covering one record and explaining another, and so the
// disposition §7.4.5 stores is the same value §7.3 counts the record under.
//
// A record whose disposition is neither of §7.2's two is refused rather than
// waived. §9.1's `discarded` row has the waiver written and the disposition set
// together, so a record missing one is a discard that did not happen; and a
// scope guessed for it would have to guess repository, the wider of the two, on
// the record cr understands least.
func WaiverFor(record *Finding) (Waiver, error) {
	waiver := Waiver{WaiverKey: WaiverKeyOf(record), Disposition: record.Disposition}
	if _, err := waiver.Scope(); err != nil {
		return Waiver{}, err
	}
	return waiver, nil
}

// Scope reports how far the waiver reaches, per §7.4.1.
//
// This is the whole binding between §7.2's dispositions and §7.4.4's two files,
// and it is the only one there is. No caller hands a scope in, so no caller can
// put a `not-here` in the repository file — the mistake §7.4.3 is written to
// prevent, and the one nobody would ever see, since its cost is a finding that
// stops being raised.
//
// The receiver is a pointer because a waiver is a wide struct and nothing here
// writes to it, as WaiverKeyOf takes its record.
func (w *Waiver) Scope() (WaiverScope, error) {
	switch w.Disposition {
	case DispositionWrong:
		return ScopeRepository, nil
	case DispositionNotHere:
		return ScopePullRequest, nil
	}
	return WaiverScope{}, &UnknownDispositionError{Value: string(w.Disposition)}
}

// UnknownDispositionError reports a value that names no disposition of §7.2.
//
// §7.2's table has exactly two, and §6.1.4 has cr write `disposition` rather
// than the agent, so reaching this means either that a record was discarded
// without one or that a stored line was written by something other than cr.
type UnknownDispositionError struct {
	// Value is the offending value, exactly as it was written.
	Value string
}

func (e *UnknownDispositionError) Error() string {
	return fmt.Sprintf(
		"%q is not a triage disposition; §7.2 has exactly %q and %q, and a waiver's scope follows it",
		e.Value, DispositionWrong, DispositionNotHere,
	)
}
