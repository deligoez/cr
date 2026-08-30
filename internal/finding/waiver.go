package finding

import "github.com/deligoez/cr/internal/git"

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
