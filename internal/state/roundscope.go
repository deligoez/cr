package state

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"slices"
	"strings"
)

// ReadStamped reads one of the nine §2.3.3 files and returns the records of
// one round, in file order. It refuses a file §2.3.3 does not list, as
// AppendStamped does.
//
// It is §9.3.5's first sentence as a function: a command reads only the current
// round's records, and earlier rounds are history. The scoping belongs at the
// read for the reason the stamping belongs at the write — there is one place
// that decides which records a round can see, so no command can forget the
// filter, and none of them can come to disagree about what a round holds.
//
// The round is read off the stored line rather than off the decoded record, so
// a record type needs no accessor for it and the reader here and the writers
// below cannot key a line differently. roundOf gives a line supplying no round
// the round 0, which §9.3.3 numbers no round, so such a line is invisible to
// every round rather than visible to one of them.
//
// It takes no lock, per §2.3.2.
func ReadStamped[T any](l Layout, owner, repo string, pr int, name string, round int) ([]T, error) {
	if err := checkStamped(name); err != nil {
		return nil, err
	}
	body, err := l.ReadPR(owner, repo, pr, name)
	if err != nil {
		return nil, err
	}
	return decodeRound[T](l.PRFile(owner, repo, pr, name), body, round)
}

// decodeRound decodes the lines of one §2.3.3 file that belong to round,
// naming path in whatever it refuses.
//
// The result is never nil: a file holding none of the round's records is no
// records rather than a null slice (§12.3). A line is decoded twice — once for
// its round and once into the record type — which is what lets the filter run
// over every one of the nine shapes without any of them exposing the pair.
//
// A line it cannot decode is refused through unusableLine, as decodeRecords
// refuses one.
func decodeRound[T any](path string, body []byte, round int) ([]T, error) {
	records := make([]T, 0)
	for i, line := range bytes.Split(body, []byte{'\n'}) {
		if len(bytes.TrimSpace(line)) == 0 {
			continue
		}
		fields, err := storedFields(line)
		if err != nil {
			return nil, unusableLine(path, i+1, err)
		}
		if roundOf(fields) != round {
			continue
		}
		var record T
		if err := json.Unmarshal(line, &record); err != nil {
			return nil, unusableLine(path, i+1, err)
		}
		records = append(records, record)
	}
	return records, nil
}

// unusableLine is the refusal of one stored line a read could not decode: cr's
// own file under ~/.cr, which §11.2 codes 3 with UnusableHint. It names the path
// and the one-based line, counting blank lines, in the shape visitLines gives a
// locked walk's refusal, so a reader and a writer name a bad line alike.
func unusableLine(path string, line int, err error) error {
	return fmt.Errorf("%s line %d: %w", path, line, FileFailure("use", filepath.Base(path), UnusableHint, err))
}

// ReplaceStamped replaces the current round's records in one of the nine
// §2.3.3 files, stamping head and round onto every record it writes and leaving
// every earlier round's line byte for byte. It refuses a file §2.3.3 does not
// list, as AppendStamped does.
//
// claims.ndjson is what it exists for. §3.3.1 has `cr claims record` replace
// that file, and §9.3.5 reads the same sentence back with a scope: a command
// documented as replacing or clearing a file does so for the current round
// only and MUST leave earlier rounds intact.
//
// AppendStamped cannot do that job. It keeps every earlier round's records,
// and keeps the current round's records too, so a second `cr claims record` in
// one round would leave both extractions in the file and every reader would
// see each claim twice.
//
// The earlier rounds' bytes are carried through untouched rather than decoded
// and re-encoded, for the reason AppendStamped gives: a record a later version
// wrote stays exactly as that version wrote it, and nothing this version does
// not understand is dropped on the way past.
func ReplaceStamped[T Stamped](k *Lock, name string, at Stamp, records []T) error {
	if err := checkStamped(name); err != nil {
		return err
	}
	for _, record := range records {
		record.setStamp(at)
	}
	added, err := encodeRecords(name, records)
	if err != nil {
		return err
	}
	kept, err := earlierRounds(k, name, at.Round)
	if err != nil {
		return err
	}
	return k.Write(name, append(kept, added...))
}

// ClearStamped drops the current round's records from one of the nine §2.3.3
// files and leaves every earlier round's line byte for byte.
//
// mapping.ndjson is what it exists for: §3.3.1 clears it whenever claims are
// recorded and §9.3.4 clears it again when a moved head opens a round, while
// §9.3.5 scopes both clearings to the current round. It is ReplaceStamped with
// no records, and it is a function of its own because there are none to infer
// the round from — a caller passing an empty slice would have to name the round
// in a Stamp whose head nothing would ever write.
func ClearStamped(k *Lock, name string, round int) error {
	if err := checkStamped(name); err != nil {
		return err
	}
	kept, err := earlierRounds(k, name, round)
	if err != nil {
		return err
	}
	return k.Write(name, kept)
}

// RewriteStamped rewrites the records of one §2.3.3 file that apply changes and
// leaves every other line byte for byte. It refuses a file §2.3.3 does not
// list, as AppendStamped does.
//
// §9.3.4's stale sweep is what it exists for, and it is the one write here that
// crosses a round boundary. Every other writer in this file is scoped by round,
// because §9.3.5 calls earlier rounds history and has a clearing or a
// replacement take out the current round only. The sweep cannot be: the records
// it moves to `stale` are exactly the ones the round now closing produced, so a
// scoped sweep would reach none of them. Round 12's
// closed-exemption-list-blocks-required-behaviour is that argument, and
// round-scoping's acceptance records the resolution — §9.3.4's transition is
// exempt from §9.3.5's scoping, or `stale` is a state nothing can reach.
//
// What it will not do is restamp. ReplaceStamped would move every record it
// rewrote into the round being opened, and the round that produced them would
// read as a round that recorded nothing; §2.3.3's pair says which run wrote the
// record, and this changes a record's state rather than its provenance.
//
// A line is therefore handed to apply as the fields it supplied rather than as
// a decoded record, so a field this version does not understand is carried
// through rather than dropped. The fields are keyed by FoldedFields, the way
// encoding/json binds a key to a field: every read of a stored record decodes
// into its struct, which binds `"State"` exactly as it binds `"state"`, so a
// sweep that looked a key up by its exact spelling would leave a record open
// that every reader calls open. A line giving one key twice under that folding
// is refused as a file cr cannot use, since no one value is the one a read
// binds. A line apply changes is re-encoded under the spellings it supplied,
// which returns its keys in the order encoding/json writes a map: the values
// are the values that were there, and the byte-for-byte promise is kept for
// every line apply leaves alone.
//
// before runs once every line has been applied and before the file is
// published, and a failure it returns publishes nothing. It is where §9.1.1's
// journal is written: the moves are decided line by line inside apply, and a
// move published ahead of its journal line would be one a failed append leaves
// unjournaled for good, since the re-run finds the record already moved.
func RewriteStamped(
	k *Lock, name string, apply func(map[string]json.RawMessage) (bool, error), before func() error,
) error {
	if err := checkStamped(name); err != nil {
		return err
	}
	out, err := visitLines(k, name, func(fields map[string]json.RawMessage, line []byte) ([]byte, error) {
		changed, err := apply(fields)
		if err != nil || !changed {
			return line, err
		}
		// visitLines has decoded the line already, so this reads the
		// spellings of a well-formed object and its error is returned
		// rather than expected.
		var supplied map[string]json.RawMessage
		if err := json.Unmarshal(line, &supplied); err != nil {
			return nil, err
		}
		return json.Marshal(respelled(supplied, fields))
	})
	if err != nil {
		return err
	}
	if err := before(); err != nil {
		return err
	}
	return k.Write(name, out)
}

// respelled returns folded, the fields apply left, under the spellings supplied
// gave them, so a rewrite changes a record's values and never its keys. A field
// the line did not supply keeps foldKey's spelling.
func respelled(supplied, folded map[string]json.RawMessage) map[string]json.RawMessage {
	spelling := make(map[string]string, len(supplied))
	for key := range supplied {
		spelling[foldKey(key)] = key
	}
	out := make(map[string]json.RawMessage, len(folded))
	for key, value := range folded {
		if given, spelt := spelling[key]; spelt {
			key = given
		}
		out[key] = value
	}
	return out
}

// checkStamped refuses a file §2.3.3 does not list, so a round-scoped write
// cannot be aimed at a file whose records carry no round to scope by.
func checkStamped(name string) error {
	if !slices.Contains(stampedFiles, name) {
		return fmt.Errorf("%s: §2.3.3 does not list it, so its records carry no head or round", name)
	}
	return nil
}

// ReplaceStampedKeys replaces only the current round's records that sit at the
// same key as one of the records being written, and leaves every other line
// byte for byte: every earlier round, and every key of this round the file
// being written does not name.
//
// coverage.ndjson is what it exists for, and it is a different sentence from
// the one ReplaceStamped implements. §3.3.1 has `cr claims record` replace the
// claims of the round, whole; §4.5.6 has `cr cells record` replace "the current
// round's cell for each `(unit, role)` the file names" and leave every other
// cell untouched. A round is filled by many roles handing in many files, so a
// whole-round replacement would have each role's file delete the last one's
// cells — the review would end with the coverage of whichever role reported
// last, and §10.2.2 would read that as the round's proven coverage.
//
// keyFields names the fields that make a record's key, by their JSON keys, so
// what a caller declares is the §4.5.6 clause itself rather than a struct this
// package would have to know. Both sides are keyed by the same function over
// the same encoded JSON — the records being written are read back out of the
// bytes about to be appended — so an incoming record and the stored line it
// replaces cannot be keyed differently.
func ReplaceStampedKeys[T Stamped](
	k *Lock, name string, at Stamp, records []T, keyFields []string,
) error {
	if err := checkStamped(name); err != nil {
		return err
	}
	for _, record := range records {
		record.setStamp(at)
	}
	added, err := encodeRecords(name, records)
	if err != nil {
		return err
	}
	replaced, err := keysOf(name, added, keyFields)
	if err != nil {
		return err
	}
	kept, err := keptLines(k, name, func(line map[string]json.RawMessage) bool {
		_, taken := replaced[keyOf(keyFields, line)]
		return roundOf(line) == at.Round && taken
	})
	if err != nil {
		return err
	}
	return k.Write(name, append(kept, added...))
}

// keysOf reads the keys of the records about to be written out of the bytes
// they were encoded to.
//
// Reading them back rather than off the structs is what makes the two sides
// agree: a stored line is keyed by its JSON fields, so the record replacing it
// is keyed by its JSON fields too, and a field whose Go name and JSON key
// differ cannot key one side and miss the other.
func keysOf(name string, encoded []byte, keyFields []string) (map[string]struct{}, error) {
	keys := make(map[string]struct{})
	for i, line := range bytes.Split(encoded, []byte{'\n'}) {
		if len(bytes.TrimSpace(line)) == 0 {
			continue
		}
		var fields map[string]json.RawMessage
		if err := json.Unmarshal(line, &fields); err != nil {
			return nil, fmt.Errorf("cannot key record %d of %s: %w", i+1, name, err)
		}
		keys[keyOf(keyFields, fields)] = struct{}{}
	}
	return keys, nil
}

// keyOf builds one record's key out of the raw values of keyFields, separated
// by a byte no JSON document can hold, so no two different keys can render the
// same string. A field the line does not carry contributes nothing, which keeps
// such a line distinct from every record a command can hand in — §4.5.6's own
// fields are required, so no incoming cell has an empty one.
func keyOf(keyFields []string, fields map[string]json.RawMessage) string {
	var key strings.Builder
	for _, field := range keyFields {
		key.Write(fields[field])
		key.WriteByte(0)
	}
	return key.String()
}

// roundOf is §2.3.3's round as a stored line supplies it, and 0 for a line that
// supplies none or supplies something that is not a number.
//
// §9.3.3 numbers rounds from 1, so 0 is no round at all and such a line is kept
// by every round-scoped write. Keeping it is the conservative half: a line this
// version cannot read is history it has no licence to delete.
//
// fields are keyed by storedFields, so `"Round"` supplies the round `"round"`
// does, as it does to the struct every read decodes the line into.
func roundOf(fields map[string]json.RawMessage) int {
	var round int
	if err := json.Unmarshal(fields["round"], &round); err != nil {
		return 0
	}
	return round
}

// storedFields is the one reading of a stored §2.3.3 line's fields that
// ReadStamped and every writer here share: FoldedFields, keying each field the
// way encoding/json binds it, so the round a line is scoped by is the round
// every decode of the line reads.
//
// A line giving one key twice under that folding has no one value a decode
// binds, so it is refused. §11.2 codes a repeated key 1 in a file a caller
// hands cr, and this is cr's own stored file, so the refusal carries the key
// and not the error type that would code it 1; the caller names the file and
// the line.
func storedFields(line []byte) (map[string]json.RawMessage, error) {
	fields, err := FoldedFields(line)
	if repeated := (*RepeatedKeyError)(nil); errors.As(err, &repeated) {
		return nil, fmt.Errorf("%s is given more than once", repeated.Key)
	}
	return fields, err
}

// earlierRounds returns the lines of one §2.3.3 file that do not belong to
// round, in file order and byte for byte.
//
// Every round but the named one is kept, not merely the ones before it. §9.3.5
// calls the others history and this has no way to tell a round that has passed
// from one a state directory carries for some other reason, so it takes out
// exactly the round it was asked to take out and nothing else.
func earlierRounds(k *Lock, name string, round int) ([]byte, error) {
	return keptLines(k, name, func(line map[string]json.RawMessage) bool {
		return roundOf(line) == round
	})
}

// keptLines returns the lines of one §2.3.3 file that drop leaves in place, in
// file order and byte for byte.
//
// A line is handed to drop as the fields it supplied rather than as a decoded
// record. This file is generic over nine different record shapes, and decoding
// each line into its own struct would mean a line the current version cannot
// fully decode — a field a later version added — deciding the fate of a round
// it has nothing to do with. The bytes that survive are the bytes that were
// there, so a record a later version wrote stays exactly as that version wrote
// it.
//
// The walk itself is visitLines below, which RewriteStamped shares.
func keptLines(k *Lock, name string, drop func(map[string]json.RawMessage) bool) ([]byte, error) {
	return visitLines(k, name, func(fields map[string]json.RawMessage, line []byte) ([]byte, error) {
		if drop(fields) {
			return nil, nil
		}
		return line, nil
	})
}

// visitLines walks the records of one §2.3.3 file and returns the file its
// visitor leaves behind: each line is offered as the fields it supplied and as
// its own bytes, and the visitor answers with the bytes to keep, or nil to drop
// it.
//
// It is the one place that reads such a file apart into lines, so the two
// writers built on it — keptLines above, which drops, and RewriteStamped, which
// rewrites — cannot come to different conclusions about a blank line, an absent
// file, or a line that does not decode.
//
// The read is unlocked-safe because the caller holds the §2.3.1 lock: no other
// writer can be between this read and the Write that follows it. A file that is
// not there yet holds as many records as an empty one, which is the reading
// storeRecords already gives an absent store.
//
// A refusal names the path and the line, counting blank lines, because the line
// is what the user has to open and nothing but this walk knows which it was — a
// visitor's refusal included. A line that is not JSON is cr's own state it
// cannot use, which §11.2 codes 3 with UnusableHint.
func visitLines(
	k *Lock, name string, visit func(map[string]json.RawMessage, []byte) ([]byte, error),
) ([]byte, error) {
	path := filepath.Join(k.dir, name)
	held, err := os.ReadFile(path)
	if err != nil && !errors.Is(err, fs.ErrNotExist) {
		return nil, FileFailure("read", path, readHint(name), err)
	}
	var kept bytes.Buffer
	for i, line := range bytes.Split(held, []byte{'\n'}) {
		if len(bytes.TrimSpace(line)) == 0 {
			continue
		}
		fields, err := storedFields(line)
		if err != nil {
			return nil, fmt.Errorf("%s line %d: %w", path, i+1, FileFailure("use", name, UnusableHint, err))
		}
		out, err := visit(fields, line)
		if err != nil {
			return nil, fmt.Errorf("%s line %d: %w", path, i+1, err)
		}
		if out == nil {
			continue
		}
		kept.Write(out)
		kept.WriteByte('\n')
	}
	return kept.Bytes(), nil
}
