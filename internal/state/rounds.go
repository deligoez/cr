package state

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strconv"
)

// The per-round artefacts of the §2.3 table. They sit under rounds/<n>/ rather
// than beside the thirteen flat files because those hold the current round's
// working state while a round directory is history: §9.3.5 has a command read
// only the current round's records, and a command documented as replacing or
// clearing a file do so for the current round only, leaving earlier rounds
// intact.
//
// Nothing here decides what the artefacts hold. draft.md is §7.1's rendering,
// rendered.json §7.1.5's per-record agent regions, posted.json §8.3.3's exact
// payload, and summary.json §10.3's counts. This file supplies the directory,
// the paths, and typed access to them.
const (
	FileDraft    = "draft.md"
	FileRendered = "rendered.json"
	FilePosted   = "posted.json"
	FileSummary  = "summary.json"
)

// FileIntake is `rounds/<n>/intake.json`: the key of every record `cr merge`'s
// and `cr record`'s drops took out, which the round summary's intake counts are
// made of. It sits beside the §2.3 table rather than in it, because §6.4.4
// lets only the count of a dropped finding reach summary.json, and it is not
// created with a round: a round no v0.2.1 intake has written has none.
const FileIntake = "intake.json"

// FileContract is `rounds/<n>/contract.md`: the record schema of §6.1 that
// `cr review` writes and every prompt of round n names (§4.6.2). Like
// FileIntake it is not created with a round, since a round no `cr review` has
// emitted for has none.
const FileContract = "contract.md"

// roundsDirName holds every round of one pull request.
const roundsDirName = "rounds"

// emptyDocument is the initial content of a round's JSON artefacts: a document
// with nothing recorded in it yet. The empty file an NDJSON artefact starts
// from would not parse as JSON.
const emptyDocument = "{}\n"

// roundFiles is the rounds/<n>/ part of the §2.3 table, in table order.
var roundFiles = []string{FileDraft, FileRendered, FilePosted, FileSummary}

// RoundFiles returns the per-round artefact names in table order. The result is
// a copy, so a caller can neither widen the set nor reorder it.
func RoundFiles() []string {
	return append(make([]string, 0, len(roundFiles)), roundFiles...)
}

// roundDir is one round's directory relative to the pull request's state
// directory. Every round path is composed from here, so the writer holding the
// lock and the reader resolving a layout path cannot drift apart.
func roundDir(round int) string {
	return filepath.Join(roundsDirName, strconv.Itoa(round))
}

// roundPath is one artefact of one round, relative to the same directory.
func roundPath(round int, name string) string {
	return filepath.Join(roundDir(round), name)
}

// RoundDir is one round's artefact directory (§2.3).
func (l Layout) RoundDir(owner, repo string, pr, round int) string {
	return filepath.Join(l.PRDir(owner, repo, pr), roundDir(round))
}

// RoundFile is one per-round artefact of the §2.3 table.
func (l Layout) RoundFile(owner, repo string, pr, round int, name string) string {
	return filepath.Join(l.PRDir(owner, repo, pr), roundPath(round, name))
}

// ReadRound reads one round's artefact. It takes no lock, per §2.3.2, and it
// reads whichever round it is asked for: earlier rounds are history, and
// history stays readable.
func (l Layout) ReadRound(owner, repo string, pr, round int, name string) ([]byte, error) {
	if err := checkRound(round); err != nil {
		return nil, err
	}
	return l.ReadPR(owner, repo, pr, roundPath(round, name))
}

// checkRound refuses a round index no directory can belong to.
//
// §9.3.3 has cr brief open round 1, so meta.json's initial 0 means no round has
// been opened rather than a round of its own. Creating rounds/0/ would give a
// pull request a history it does not have, and every artefact inside it would
// belong to a round nothing ever recorded.
func checkRound(round int) error {
	if round < 1 {
		return fmt.Errorf(
			"round %d has no directory: rounds are numbered from 1, and meta.json's 0 means no round has been opened; run cr brief to open one",
			round,
		)
	}
	return nil
}

// checkRoundFile refuses a name the §2.3 table does not give a round, so the
// table, FileIntake and FileContract stay the only things that decide what a
// round directory holds.
func checkRoundFile(name string) error {
	if !slices.Contains(roundFiles, name) && name != FileIntake && name != FileContract {
		return fmt.Errorf("%s: §2.3 gives a round no such artefact", name)
	}
	return nil
}

// EnsureRound creates one round's directory and its four artefacts on demand: a
// round directory appears when that round is first written, not when the state
// directory is created.
//
// It touches exactly the round it is given. No path it composes names another
// round, and an artefact already there is left as it is, so re-entering a round
// discards nothing either.
//
// It is a method on the held lock because a round directory is per-PR state,
// and §2.3.1 admits no unlocked write to that.
func (k *Lock) EnsureRound(round int) error {
	if err := checkRound(round); err != nil {
		return err
	}
	if err := makeDirs([]string{filepath.Join(k.dir, roundDir(round))}); err != nil {
		return err
	}
	for _, name := range roundFiles {
		var body []byte
		if name != FileDraft {
			body = []byte(emptyDocument)
		}
		if err := k.createMissing(roundPath(round, name), body); err != nil {
			return err
		}
	}
	return nil
}

// WriteRound publishes one artefact of one round, creating that round's
// directory on demand.
func (k *Lock) WriteRound(round int, name string, data []byte) error {
	if err := checkRoundFile(name); err != nil {
		return err
	}
	if err := k.EnsureRound(round); err != nil {
		return err
	}
	return k.Write(roundPath(round, name), data)
}

// UpdateRoundSection writes one named field of a round's JSON artefact and
// leaves every other field of that document exactly as it found it.
//
// It is §10.3's sentence as a function. `cr merge`, `cr draft` and `cr post`
// each accumulate their own counts into one summary.json, so a writer owns some
// of its fields and none of the rest — and the failure that invites is silent
// and total: a command that decoded the document into a struct of its own
// fields would re-encode it without every field that struct does not name, and
// the history §10.3 exists to make reconstructable from state alone would lose
// whatever the writer before it put there.
//
// The document is therefore carried as raw fields, which is the reading
// keptLines already gives an NDJSON line: a field this version does not
// understand is passed through rather than dropped.
func UpdateRoundSection[T any](k *Lock, round int, name, section string, value T) error {
	encoded, err := json.Marshal(value)
	if err != nil {
		return fmt.Errorf("cannot encode %s of %s: %w", section, name, err)
	}
	return UpdateRoundJSON(k, round, name, func(doc *map[string]json.RawMessage) {
		if *doc == nil {
			*doc = make(map[string]json.RawMessage, 1)
		}
		(*doc)[section] = encoded
	})
}

// UpdateRoundJSON reads one round's JSON artefact, hands the decoded document
// to apply, and republishes it.
//
// summary.json is what this exists for. §10.3 has cr merge, cr draft, and cr
// post each accumulate their own counts into one document, so a writer sets its
// own fields and leaves every other field exactly as it found it. Without this
// each of the three would read and rewrite the whole document at its own call
// site, and all three would have to keep agreeing about fields none of them
// owns.
func UpdateRoundJSON[T any](k *Lock, round int, name string, apply func(*T)) error {
	if err := checkRoundFile(name); err != nil {
		return err
	}
	// EnsureRound publishes the empty document when the round is new, so the
	// read below always has something to decode.
	if err := k.EnsureRound(round); err != nil {
		return err
	}
	// EnsureRound does not create FileIntake, so its first writer does.
	if err := k.createMissing(roundPath(round, name), []byte(emptyDocument)); err != nil {
		return err
	}
	path := filepath.Join(k.dir, roundPath(round, name))
	body, err := os.ReadFile(path)
	if err != nil {
		return FileFailure("read", path, readHint(name), err)
	}
	var doc T
	if err := json.Unmarshal(body, &doc); err != nil {
		return FileFailure("read", path, UnusableHint, err)
	}
	apply(&doc)
	out, err := json.MarshalIndent(doc, "", "  ")
	if err != nil {
		return fmt.Errorf("cannot encode %s: %w", path, err)
	}
	return k.WriteRound(round, name, append(out, '\n'))
}
