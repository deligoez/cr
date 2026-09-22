package cli

import (
	"errors"
	"io/fs"
	"maps"
	"os"
	"strings"

	"github.com/deligoez/cr/internal/finding"
	"github.com/deligoez/cr/internal/migrate"
	"github.com/deligoez/cr/internal/state"
)

// ingestRound is ingestDraft for `cr draft`, with §7.1.7's carried bodies
// seeded into what it preserved.
func ingestRound(
	l state.Layout, owner, repo string, pr int, round *state.Meta, records []*finding.Finding,
	journal *finding.Journal,
) (triaged, error) {
	triage, err := ingestDraft(l, owner, repo, pr, round, records, journal)
	if err != nil {
		return triaged{}, err
	}
	triage.Preserved, err = seedCarried(l, owner, repo, pr, round, triage.Preserved)
	return triage, err
}

// seedCarried is §7.1.7: a record §9.3.4 carried is rendered with the body the
// reviewer had edited it to in the round it came from, which `cr brief` kept on
// its line of migrations.ndjson.
//
// It seeds only a round whose draft.md holds nothing yet. A carry happens on
// §9.3.3's increment and nowhere else, so every carried record is in the round
// before its first draft; from then on the round's own draft.md is the record of
// what the reviewer wants, and §7.1.6 preserves an edit from there. Seeding
// again later would put back a body the reviewer had since rewritten, or reverted
// to cr's rendering on purpose.
//
// preserved is §7.1.6's map from the ingest, which a first draft leaves empty;
// a copy is returned, so the ingest's own answer is not changed under it.
func seedCarried(
	l state.Layout, owner, repo string, pr int, round *state.Meta, preserved map[string]string,
) (map[string]string, error) {
	body, err := os.ReadFile(l.RoundFile(owner, repo, pr, round.Round, state.FileDraft))
	if err != nil && !errors.Is(err, fs.ErrNotExist) {
		return nil, err
	}
	if strings.TrimSpace(string(body)) != "" {
		return preserved, nil
	}
	carried, err := state.ReadRecords[migrate.Record](l, owner, repo, pr, state.FileMigrations)
	if err != nil {
		return nil, err
	}
	seeded := maps.Clone(preserved)
	if seeded == nil {
		seeded = make(map[string]string)
	}
	for i := range carried {
		line := &carried[i]
		if line.Round != round.Round || !line.Carried || line.Body == "" {
			continue
		}
		if _, held := seeded[line.Record]; !held {
			seeded[line.Record] = line.Body
		}
	}
	return seeded, nil
}
