package migrate

// Record is one line of migrations.ndjson: §9.4.7's report of one record that
// §9.3.3's increment migrated, and whether §9.4.5 carried it into the round the
// increment opened.
//
// It is written by `cr brief`, which is the only command that migrates, and
// read by `cr recheck` (§9.5.7) and `cr draft` (§7.1.7). The file is not one of
// §2.3.3's stamped files, so the round and head are the record's own fields,
// as a verdict's are.
type Record struct {
	Outcome
	// Carried reports whether §9.4.5 carried the record. A placed anchor
	// can still stay behind, when it is LEFT or lies in no unit of the new
	// round, so this is not Placed under another name.
	Carried bool `json:"carried"`
	// FromRound is the round the record was in before the increment.
	FromRound int `json:"from_round"`
	// Round and Head are the round the increment opened and its head.
	Round int    `json:"round"`
	Head  string `json:"head"`
	// Probe is the probe a carried record held, which §9.4.8 clears from
	// the record: the experiment ran against code the push changed, and the
	// id is kept here so the reviewer can see what to run again.
	Probe string `json:"probe,omitempty"`
	// Body is the agent region the closing round's draft held for a carried
	// record when it differed from that round's rendered.json, which §7.1.7
	// has `cr draft` render in the new round.
	Body string `json:"body,omitempty"`
}
