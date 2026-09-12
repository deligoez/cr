package state

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
)

// The files of the §2.3 table that sit directly in a pull request's state
// directory. Every reader and writer names one of these rather than spelling a
// file name of its own, so the table has one home.
//
// The rounds/<n>/ artefacts of the same table are per-round and are created on
// demand, not here.
const (
	FileMeta        = "meta.json"
	FileClaims      = "claims.ndjson"
	FileUnits       = "units.ndjson"
	FileMapping     = "mapping.ndjson"
	FilePostedIndex = "posted-index.ndjson"
	FileIntentGaps  = "intent-gaps.ndjson"
	FileRuns        = "runs.ndjson"
	FileThreads     = "threads.ndjson"
	FileFindings    = "findings.ndjson"
	FileProbes      = "probes.ndjson"
	FileCoverage    = "coverage.ndjson"
	FileTransitions = "transitions.ndjson"
	FileWaivers     = "waivers.ndjson"
	// FileSandboxBaseline is §5.1.6's post-setup baseline, and it is a
	// row of this table rather than a file in the sandbox because of
	// where the sandbox is. Round 8's unhomed-state finding is the
	// argument: the baseline is durable state one `cr` invocation writes
	// and a later one reads, and the only other place to keep it would be
	// the worktree it describes — which is a worktree of the repository
	// under review, where §2.2 permits cr no write but §5.1.1's
	// registration. So it lives beside the rest of one pull request's
	// state, under the state root every other row is under.
	//
	// It is not in prFiles, for the reason the rounds/<n>/ artefacts are
	// not: it is created on demand, by `cr sandbox create`. The two
	// absences mean different things — an NDJSON file that is there and
	// empty holds no records, while a baseline that is not there is a
	// pull request no sandbox has been prepared for, which is what
	// §5.1.6's check has to be able to tell.
	FileSandboxBaseline = "sandbox-baseline.json"
)

// prFiles is the §2.3 table in table order.
var prFiles = []string{
	FileMeta, FileClaims, FileUnits, FileMapping, FilePostedIndex,
	FileIntentGaps, FileRuns, FileThreads, FileFindings, FileProbes,
	FileCoverage, FileTransitions, FileWaivers,
}

// prFileWriter names the command that fills each file of the §2.3 table, so a
// read that found one missing can say what would have put records in it.
//
// createFiles writes every row empty when the state directory is created, so a
// file that is absent is a directory older than the row or one edited by hand,
// and the honest next step is the command that writes it. transitions.ndjson
// has no entry because nothing in this tree writes it yet: a hint naming the
// wrong command costs more than one naming none.
var prFileWriter = map[string]string{
	FileMeta:        "cr brief",
	FileUnits:       "cr brief",
	FileThreads:     "cr brief",
	FileClaims:      "cr claims record",
	FileMapping:     "cr map record",
	FileIntentGaps:  "cr map record",
	FileCoverage:    "cr cells record",
	FileFindings:    "cr record",
	FileProbes:      "cr probe run",
	FileRuns:        "cr test",
	FilePostedIndex: "cr post --confirm",
	FileWaivers:     "cr draft",
}

// readHint is §12.4's next actionable step for a file of §2.2's tree a command
// required and could not read.
//
// name may carry a rounds/<n>/ prefix, so the table is consulted by base name;
// a per-round artefact matches none of its rows and takes the general answer,
// which is true of every file under the state directory.
func readHint(name string) string {
	if writes, found := prFileWriter[filepath.Base(name)]; found {
		return "§2.2 keeps it under the pull request's state directory, and `" +
			writes + "` is the command that writes it"
	}
	return "§2.2 keeps it under the pull request's state directory; " +
		"`cr status` reports how far the round has got"
}

// PRFiles returns the §2.3 file names in table order. The result is a copy, so
// a caller can neither widen the set nor reorder it.
func PRFiles() []string {
	return append(make([]string, 0, len(prFiles)), prFiles...)
}

// createFiles creates every missing file of the §2.3 table and leaves an
// existing one exactly as it is, so creating a state directory a second time
// never discards a recorded round.
//
// It is a method on the held lock because creating those files is a write to
// per-PR state, and §2.3.1 admits no exception to the lock.
func (k *Lock) createFiles(m *Meta) error {
	meta, err := encodeMeta(m)
	if err != nil {
		return err
	}
	for _, name := range prFiles {
		// Every file but meta.json is NDJSON, whose empty state is an
		// empty file: no records yet, which is what a new directory has.
		var body []byte
		if name == FileMeta {
			body = meta
		}
		if err := k.createMissing(name, body); err != nil {
			return err
		}
	}
	return nil
}

// createMissing publishes one file of the locked pull request's state unless it
// is already there.
func (k *Lock) createMissing(name string, body []byte) error {
	path := filepath.Join(k.dir, name)
	switch _, err := os.Stat(path); {
	case err == nil:
		return nil
	case !errors.Is(err, fs.ErrNotExist):
		return fmt.Errorf("cannot inspect %s: %w", path, err)
	}
	return k.Write(name, body)
}
