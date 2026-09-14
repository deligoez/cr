package cli

import (
	"github.com/deligoez/cr/internal/finding"
	"github.com/deligoez/cr/internal/state"
)

// keyTrees are the trees §7.4.1's key reads a record's anchored lines from, at
// the head the record was produced against: anchorTrees, held in a variable for
// the reason positionHunks is one. A stored record keeps the hash of its
// anchored lines and not the lines, so a waiver written at triage and a
// posted-index entry written at posting read them again; most of `cr draft`'s
// and `cr post`'s fixtures stand a round on a head that is no commit, and one of
// them can stand in a tree without a checkout behind it. The tests about the key
// itself leave it as it is and read a real one.
var keyTrees = anchorTrees

// stampAnchors resolves every accepted record's anchor against the round's
// trees and records §9.2's content hash and context window on it, refusing the
// whole file when one does not resolve.
//
// §9.2.3 has both recorded even though v0.1 never migrates, and they are cr's to
// write for the reason finding.StampAnchor gives: §7.4.1 keys a waiver on the
// window and the lines, and a window the agent typed or left empty keys it on
// nothing the tree holds. Measured 2026-09-11 on a real pull request, every
// anchor `cr record` stored carried `content_hash: ""`, so a waiver written from
// a discard shrank to path and class and kept suppressing after the code under
// it changed; measured again at 8cdf48b, a typed `0123456789abcdef` reached
// findings.ndjson unchanged.
//
// The trees are the round's own — the head meta.json recorded, and the merge
// base against it — so an anchor is bound to the head it was produced against,
// per §9.2.2, and nothing here reads a later one.
func stampAnchors(
	owner, repo string, pr int, round *state.Meta, file string, body []byte, records []*finding.Finding,
) error {
	trees := anchorTrees(owner, repo, pr, round.Head)
	at := state.RecordLines(body)
	for i, record := range records {
		if err := finding.StampAnchor(trees, file, at[i], &record.Anchor); err != nil {
			return err
		}
	}
	return nil
}
