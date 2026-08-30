package finding

// StampContentHash computes and stores §6.1's citation content hash: the
// normalised hash per §1.4 of the single line the citation names.
//
// Round 8's finding undefined-hash-preimage is why the pre-image is named at
// all. §6.1 marks the field computed and §6.2.3 records it so a v0.2 migration
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
// which is the point, because a v0.2 drift check comparing values produced by
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
