package finding

// WithdrawalReason is the reason §9.6.2's waiver records under §7.4.8, so a
// waiver a retraction wrote reads as one in `cr waivers list` rather than as a
// triage deletion nobody explained.
const WithdrawalReason = "withdrawn after posting (§9.6.2)"

// WithdrawalOutcome is the §7.3.1 outcome a retraction with disposition d
// records: `withdrawn-wrong` or `withdrawn-not-here`.
//
// The disposition is the reviewer's to give and never cr's to infer. A
// retracted concern may have been false, or true and not worth the comment,
// and nothing cr can observe separates the two (P2) — while the two carry
// opposite weight, one against the class and one for volume only. A value
// naming neither is refused, for the reason Waiver.Scope refuses one.
func WithdrawalOutcome(d Disposition) (Outcome, error) {
	switch d {
	case DispositionWrong:
		return OutcomeWithdrawnWrong, nil
	case DispositionNotHere:
		return OutcomeWithdrawnNotHere, nil
	}
	return "", &UnknownDispositionError{Value: string(d)}
}

// WaiverForWithdrawal is the waiver a retraction with disposition d writes:
// the record's own §7.4.1 key under the scope the disposition derives.
//
// It is WaiverFor with the disposition given rather than read off the record,
// because a posted record carries none — §9.1 sets `disposition` only on the
// way to `discarded`. The key comes through WaiverKeyOf, so a withdrawal and a
// triage deletion over the same code at the same class are the same waiver.
func WaiverForWithdrawal(trees Trees, record *Finding, d Disposition) (Waiver, error) {
	waiver := Waiver{Disposition: d}
	if _, err := waiver.Scope(); err != nil {
		return Waiver{}, err
	}
	key, err := WaiverKeyOf(trees, record)
	if err != nil {
		return Waiver{}, err
	}
	waiver.WaiverKey = key
	return waiver, nil
}
