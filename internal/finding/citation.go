package finding

import "fmt"

// StampContentHash computes and stores §6.1's citation content hash: the
// normalised hash per §1.4 of the single line the citation names.
//
// Round 8's finding undefined-hash-preimage is why the pre-image is named at
// all. §6.1 marks the field computed and §6.2.3 records it so a v0.3 migration
// can detect drift, but neither said what is hashed — the one hash in the
// document with no stated pre-image — so two implementations could both satisfy
// every word and produce values that cannot be compared, which is exactly the
// comparability the drift check rests on. §6.2.3 settles it by reference: the
// hash is recorded per §9.2.3, so the rule is §9.2's, and a citation names one
// line, so it is that rule over a range of one.
//
// So the value is AnchorContentHash's, over a slice of one line, rather than a
// second computation that agrees with it on the day it is written. A citation's
// hash and a one-line anchor's over the same text are one value by
// construction, and this cannot drift from §9.2 without §9.2 moving with it —
// which is the point, because a v0.3 drift check comparing values produced by
// two rules would be reading the rules and calling it code.
//
// The content is the cited line's own text, which the caller reads out of the
// current head: §6.1 resolves every citation there, and §6.2.3 rejects an entry
// whose path does not exist or whose line is out of range before a hash is
// worth taking, so what arrives here is a line that resolved.
//
// Stamping is a method, and the only exported way the field is filled, because
// §6.2.3 puts both halves on `cr`: it computes the hash and stores it. The
// other half — that the agent supplies neither — is enforced on the way in,
// where §6.1's table marks `content_hash` computed and checker.computedCitations
// rejects a record arriving with one, so what is left here is that cr's own
// value has a single origin.
//
// §6.2.3's "that hash cannot fail then" is about validation: the hash does no
// validating work in v0.1 and can reject no record. §1.4 step 1 is a separate
// refusal, and it is returned rather than swallowed for the reason
// text.NormalisedHash gives — a line that does not decode has no hash at all,
// so the field is left as it was rather than set to the hash of nothing.
func (c *Citation) StampContentHash(content string) error {
	hash, err := AnchorContentHash([]string{content})
	if err != nil {
		return err
	}
	c.ContentHash = hash
	return nil
}

// HeadFile reads a path as the head under review holds it: the file's lines,
// and whether the head holds it as a file at all.
//
// It is git.FileAtRevision with the repository and the head already bound. It
// is taken as a parameter rather than called for, so that the one place cr
// decides what tree "the current head" names stays in internal/git with every
// other read of the repository under review, and so that §6.2.3's two refusals
// can be exercised without a repository standing behind them.
type HeadFile func(path string) (lines []string, exists bool, err error)

// ResolveCitations resolves every entry of one record's citations against the
// current head, per §6.2.3: an entry whose path the head does not hold, or
// whose line the file does not have, is rejected, and an entry that resolves is
// stamped with the content hash of the line it names.
//
// The entries are stamped in place, which is what §6.2.3's "computes and
// stores" comes to here — the record that goes on to be written carries cr's
// own value, and the agent supplied none, because §6.1.4 rejects a record
// arriving with one.
//
// A rejection is a RejectedRecordError, the shape §6.1.3 gives every record
// fault and the one internal/cli already maps onto exit code 1. That is the
// code §11.2 asks for and the right one on both counts: the file was read and
// parsed without trouble, and what is wrong is the record's own content. The
// entry is named by its index and its field, as checker.computedCitations names
// one, so a record carrying several citations still points at the one at fault.
//
// Every entry is resolved, not merely the first that answers. §6.2's cited row
// asks for at least one entry that resolved, but §6.2.6 renders every one of
// them into the draft block for the human to open, so an entry nobody looked at
// is an entry the reader is sent to and cr never was.
//
// Nothing here reads the stored hash back. §6.2.3 has cr compute and store it
// and says in as many words that it does no validating work in v0.1: comparing
// a stored hash against the line it was taken from is the drift check §1.3.6
// leaves to v0.2, and in v0.1 there is nothing to compare it against anyway,
// because the value being stored is the one cr has just computed.
func ResolveCitations(read HeadFile, file string, line int, citations []Citation) error {
	for at := range citations {
		citation := &citations[at]
		reject := func(field, problem string) error {
			return &RejectedRecordError{
				File:    file,
				Line:    line,
				Field:   fmt.Sprintf("citations[%d].%s", at, field),
				Problem: problem,
			}
		}

		lines, exists, err := read(citation.Path)
		if err != nil {
			return err
		}
		if !exists {
			return reject("path", fmt.Sprintf(
				"names %q, which the head under review does not hold as a file",
				citation.Path,
			))
		}
		if citation.Line < 1 || citation.Line > len(lines) {
			return reject("line", fmt.Sprintf(
				"names line %d of %q, which holds %d lines at the head under review",
				citation.Line, citation.Path, len(lines),
			))
		}
		if err := citation.StampContentHash(lines[citation.Line-1]); err != nil {
			return err
		}
	}
	return nil
}
