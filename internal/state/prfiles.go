package state

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"slices"
	"strconv"
	"strings"
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
	FileProposals   = "proposals.ndjson"
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
	// FileIssueText is the issue text the round last read, cleaned, with the
	// round and head it was read at: `cr brief` and `cr claims record` write
	// it, and `cr status` reads the paragraphs no claim span covers out of it
	// without running the tracker command. It sits beside the §2.3 table
	// rather than in it for the reason FileSandboxBaseline does, and is not
	// created with the directory: a round no v0.2.3 command has read the
	// issue for has none.
	FileIssueText = "issue.json"
	// FileEmissions is one line per prompt `cr review` emitted: the round and
	// head, the pass, the role and unit, the moment, and the ids of the notes
	// the prompt carried. `cr note`, `cr record`, `cr status` and `cr draft`
	// read it to say which prompts a note postdates. It sits beside the §2.3
	// table rather than in it for the reason FileIssueText does, and is not
	// created with the directory: a round no v0.2.3 `cr review` has emitted
	// for has none.
	FileEmissions = "emissions.ndjson"
)

// prFiles is the part of the §2.3 table createFiles publishes with the
// directory, in table order.
var prFiles = []string{
	FileMeta, FileClaims, FileUnits, FileMapping, FilePostedIndex,
	FileIntentGaps, FileRuns, FileThreads, FileFindings, FileProbes,
	FileProposals, FileCoverage, FileTransitions, FileWaivers,
}

// prNamed is every name §2.3's table gives a file sitting directly in a pull
// request's state directory: prFiles, then the three rows created on demand,
// in table order.
//
// The rows of the same table under rounds/<n>/ are not here; roundFiles,
// FileIntake and FileContract are theirs, and checkPRFile sends a round path to
// checkRoundFile.
var prNamed = append(slices.Clone(prFiles), FileEmissions, FileIssueText, FileSandboxBaseline)

// PRNamed returns those names in table order. The result is a copy, so a caller
// can neither widen the set nor reorder it.
func PRNamed() []string {
	return slices.Clone(prNamed)
}

// checkPRFile refuses a name §2.3's table does not give a pull request's state
// directory.
//
// Write is the one door to that directory — §2.3.1 puts every write behind the
// lock, and the lock is what owns Write — so this is where "cr writes nothing
// §2.3 omits" stops being a property a test observed over one populated tree
// and becomes one no call site can break. It is checkRoundFile's counterpart,
// and for the same reason: the table, and nothing else, decides what a pull
// request's state directory holds.
//
// A round's artefact arrives as the rounds/<n>/<name> path roundPath composes,
// and is judged by the round's own half of the table. Nothing else nests, so
// any other path with a separator in it is refused here.
func checkPRFile(name string) error {
	if slices.Contains(prNamed, name) {
		return nil
	}
	if parts := strings.Split(filepath.ToSlash(name), "/"); len(parts) == 3 && parts[0] == roundsDirName {
		round, err := strconv.Atoi(parts[1])
		if err != nil {
			return fmt.Errorf("%s: §2.3 numbers a round's directory, and %q is not a number", name, parts[1])
		}
		if err := checkRound(round); err != nil {
			return err
		}
		return checkRoundFile(parts[2])
	}
	return fmt.Errorf("%s: §2.3 gives a pull request's state directory no such file", name)
}

// prFileWriter names the command that fills each file of the §2.3 table, so a
// read that found one missing can say what would have put records in it.
//
// createFiles writes every row empty when the state directory is created, so a
// file that is absent is a directory older than the row or one edited by hand,
// and the honest next step is the command that writes it. Where several
// commands append to a file the entry names the one that writes it first:
// transitions.ndjson is §9.1.1's journal, which finding.Journal appends to from
// `cr record`, `cr draft`, `cr post` and `cr brief`, and every move the other
// three journal is a move of a record `cr record` stored.
var prFileWriter = map[string]string{
	FileTransitions: "cr record",
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
