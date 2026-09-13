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
	"unicode"
)

// Stamp is the head and round pair §2.3.3 requires on every record of the eight
// files it names.
//
// A record type carries the pair by embedding this struct, which is also the
// only way to satisfy Stamped: setStamp is unexported, so a type declared
// outside this package cannot claim to be stamped while holding fields of its
// own.
type Stamp struct {
	// Head is the commit the record was produced against.
	Head string `json:"head"`
	// Round is the review round that produced it.
	Round int `json:"round"`
}

// setStamp is the writer's hook. It replaces whatever the record arrived
// carrying, because the pair has one author and it is never the caller.
func (s *Stamp) setStamp(at Stamp) { *s = at }

// Stamped is implemented by every record type that embeds Stamp.
type Stamped interface {
	setStamp(Stamp)
}

// stampedFiles are the eight files §2.3.3 names, in the order it names them.
// The list is what routes a write: it is consulted here rather than at any call
// site, so no command can decide for itself whether its file carries the pair.
var stampedFiles = []string{
	FileClaims, FileUnits, FileMapping, FileFindings,
	FileProbes, FileRuns, FileIntentGaps, FileCoverage,
}

// StampedFiles returns the eight §2.3.3 file names in the order §2.3.3 names
// them. The result is a copy, so a caller can neither widen the set nor
// reorder it.
//
// It is exported for the guard that holds §9.3.5's scoping over the tree:
// which files carry a round is what decides which reads have to be scoped by
// one, and a guard spelling that set out for itself could go on measuring
// yesterday's table.
func StampedFiles() []string {
	return append(make([]string, 0, len(stampedFiles)), stampedFiles...)
}

// WriteRecords writes an NDJSON file of the locked pull request's state that
// §2.3.3 does not list, and refuses one it does.
func WriteRecords[T any](k *Lock, name string, records []T) error {
	if slices.Contains(stampedFiles, name) {
		return fmt.Errorf(
			"%s: §2.3.3 requires head and round on every record, so it is written with WriteStamped",
			name,
		)
	}
	return writeRecords(k, name, records)
}

// WriteStamped writes one of the eight §2.3.3 files, stamping head and round
// onto every record on the way out, and refuses a file §2.3.3 does not list.
//
// The stamping belongs to the writer rather than to its callers. There is one
// place that sets the pair, so the commands that take records from an agent
// cannot forget it, cannot disagree about it, and cannot be talked into
// carrying the agent's own values through: whatever a record already holds is
// overwritten here.
func WriteStamped[T Stamped](k *Lock, name string, at Stamp, records []T) error {
	if err := checkStamped(name); err != nil {
		return err
	}
	for _, record := range records {
		record.setStamp(at)
	}
	return writeRecords(k, name, records)
}

// AppendStamped adds records to the end of one of the eight §2.3.3 files,
// stamping head and round onto every record it adds and leaving what the file
// already holds byte for byte. It refuses a file §2.3.3 does not list, as
// WriteStamped does.
//
// findings.ndjson is what it exists for. §2.3 has that file hold all findings
// and questions in all states, so a round adds its records to it rather than
// replacing it, and §9.3.5 leaves earlier rounds intact.
//
// WriteStamped cannot do that job, and the reason is the property that makes it
// worth having: it stamps every record it is handed. Reading the file's
// existing records back and passing them through it would restamp each one with
// the round now being written, and the history §9.3.5 protects would be rewritten
// by the write that was meant to extend it. Carrying the previous bytes through
// untouched is the only form of the append that cannot do that — and it also
// keeps a record a later version wrote exactly as that version wrote it, rather
// than re-encoding it through this one's struct.
//
// The read is unlocked-safe because the caller holds the §2.3.1 lock: no other
// writer can be between this read and the Write that follows it.
func AppendStamped[T Stamped](k *Lock, name string, at Stamp, records []T) error {
	if err := checkStamped(name); err != nil {
		return err
	}
	for _, record := range records {
		record.setStamp(at)
	}
	return appendLines(k, name, records)
}

// AppendRecords adds records to the end of an NDJSON file of the locked pull
// request's state that §2.3.3 does not list, leaving what the file already
// holds byte for byte, and refuses one it does, as WriteRecords does.
//
// transitions.ndjson is what it exists for: §9.1.1's journal is history that
// only grows, so a write that re-encoded the lines already there could rewrite
// an earlier transition, and one that replaced the file would erase it.
func AppendRecords[T any](k *Lock, name string, records []T) error {
	if slices.Contains(stampedFiles, name) {
		return fmt.Errorf(
			"%s: §2.3.3 requires head and round on every record, so it is appended with AppendStamped",
			name,
		)
	}
	return appendLines(k, name, records)
}

// appendLines is the body both appends share: the records encoded as NDJSON
// after the bytes the file already holds, published through the held lock.
func appendLines[T any](k *Lock, name string, records []T) error {
	added, err := encodeRecords(name, records)
	if err != nil {
		return err
	}
	path := filepath.Join(k.dir, name)
	// A file that is not there yet holds as many records as an empty one,
	// which is the reading storeRecords already gives an absent store.
	held, err := os.ReadFile(path)
	if err != nil && !errors.Is(err, fs.ErrNotExist) {
		return FileFailure("read", path, readHint(name), err)
	}
	return k.Write(name, append(held, added...))
}

// ReservedFieldError reports a record that supplied a field cr writes itself.
// It carries the line so the user can open it and the field so they know what
// to drop, which is what §6.1.4 requires of the rejection. The cli layer maps
// it onto exit code 1.
type ReservedFieldError struct {
	// File is the NDJSON file the agent handed the command.
	File string
	// Line is the one-based line the record sits on, counting blank lines.
	Line int
	// Field is the field the record may not carry.
	Field string
}

func (e *ReservedFieldError) Error() string {
	return fmt.Sprintf(
		"%s line %d: %s is written by cr and must not be supplied", e.File, e.Line, e.Field,
	)
}

// stampFields are the two fields of §2.3.3, in the order it names them, so a
// record supplying both is always reported by the same one.
var stampFields = []string{"head", "round"}

// DecodeStamped decodes the NDJSON an agent hands a recording command into the
// record type of one of the eight §2.3.3 files, and refuses a line that
// supplied head or round.
//
// WriteStamped owns the pair, so a record arriving with either was written by
// the agent, and §6.1.4 rejects that with exit code 1 naming the line and the
// field. What is tested is presence on the wire rather than a non-zero Go
// field: `"round": 0` is a value the agent chose exactly as much as
// `"round": 7` is, and a decoded struct reports the two alike.
//
// Decoding is one function rather than one per command because the commands of
// §3.3.1, §4.1.6, §4.5.6 and §6.1.3 all reach WriteStamped through it. Each
// inherits the rejection instead of restating it, so none of them can give the
// agent a different answer about who owns head and round.
//
// check is what one command adds to that shared rejection. §4.1.6, §4.5.6 and
// §6.1.3 each refuse a line for reasons of their own, and each has to name the
// line it refused, so the check runs inside this loop rather than after it:
// there is one place that counts lines, and a second pass could not agree with
// it about a blank one. It is given the one-based line number, the line's
// fields as FoldedFields keys them — so a check can tell a field the agent
// omitted from one it wrote empty, under whatever letter case the decode binds
// — and the decoded record. A command with nothing to add passes nil.
func DecodeStamped[E any, T interface {
	*E
	Stamped
}](file string, body []byte, check func(int, map[string]json.RawMessage, T) error) ([]T, error) {
	records := make([]T, 0)
	for _, line := range numberedRecords(body) {
		supplied, err := FoldedFields(line.text)
		if err != nil {
			return nil, fmt.Errorf("%s line %d: %w", file, line.at, err)
		}
		for _, field := range stampFields {
			if _, written := supplied[field]; written {
				return nil, &ReservedFieldError{File: file, Line: line.at, Field: field}
			}
		}
		record := T(new(E))
		if err := json.Unmarshal(line.text, record); err != nil {
			return nil, fmt.Errorf("%s line %d: %w", file, line.at, err)
		}
		if check != nil {
			if err := check(line.at, supplied, record); err != nil {
				return nil, err
			}
		}
		records = append(records, record)
	}
	return records, nil
}

// FoldedFields decodes one JSON object into its fields, keyed the way
// encoding/json binds an object key to a struct field.
//
// That binding is case-insensitive: the decode matches a key to a field's name
// under bytes.EqualFold, which folds Unicode letters as well as ASCII ones, so
// `"Grade"` sets the field `"grade"` sets. A fence that looked a key up by its
// exact spelling would refuse the one and let the other through carrying the
// value it refuses. So every key is held under foldKey's spelling, which for
// every field name cr's records use is the name itself. Where two spellings of
// one field appear the later is kept, because the decode keeps the later value.
//
// A line holding null has no fields and gives a nil map, as json.Unmarshal
// reads it; a line that is not an object gives json.Unmarshal's own error.
func FoldedFields(object []byte) (map[string]json.RawMessage, error) {
	var exact map[string]json.RawMessage
	if err := json.Unmarshal(object, &exact); err != nil || exact == nil {
		return nil, err
	}
	// The object decoded above, so the walk below reads a well-formed
	// object and its errors are returned rather than expected.
	folded := make(map[string]json.RawMessage, len(exact))
	walk := json.NewDecoder(bytes.NewReader(object))
	if _, err := walk.Token(); err != nil {
		return nil, err
	}
	for walk.More() {
		token, err := walk.Token()
		if err != nil {
			return nil, err
		}
		key, isKey := token.(string)
		if !isKey {
			return nil, fmt.Errorf("object key %v is not a string", token)
		}
		var value json.RawMessage
		if err := walk.Decode(&value); err != nil {
			return nil, err
		}
		folded[foldKey(key)] = value
	}
	return folded, nil
}

// foldKey spells a key so that two keys spell alike exactly when
// bytes.EqualFold calls them equal, which is the equality encoding/json binds
// a key to a field by.
//
// Each rune becomes the least rune of its case-folding orbit, as
// encoding/json's own folding does — U+017F long s becomes S, U+212A Kelvin
// sign becomes K — and an ASCII capital is then written in lower case, so a
// lowercase ASCII field name spells itself.
func foldKey(key string) string {
	var folded strings.Builder
	for _, r := range key {
		least := r
		for next := unicode.SimpleFold(r); next != r; next = unicode.SimpleFold(next) {
			least = min(least, next)
		}
		if 'A' <= least && least <= 'Z' {
			least += 'a' - 'A'
		}
		folded.WriteRune(least)
	}
	return folded.String()
}

// numberedLine is one line of an NDJSON body that carries a record: the
// one-based line it sits on, and the line itself.
type numberedLine struct {
	// at is the number every rejection names, counting the blank lines
	// that were skipped to reach it.
	at int
	// text is the line's bytes, as they arrived.
	text []byte
}

// numberedRecords is the one place that decides which lines of an NDJSON body
// carry a record, and what line number each one sits on.
//
// It is one function rather than a loop written at each reader for the reason
// DecodeStamped gives about its check: a second pass could not agree with the
// first about a blank line, and every rejection names a line the user has to
// open. DecodeStamped decodes from this, and RecordLines reports the numbers to
// a caller that must refuse a record after the decode.
func numberedRecords(body []byte) []numberedLine {
	lines := make([]numberedLine, 0)
	for i, line := range bytes.Split(body, []byte{'\n'}) {
		if len(bytes.TrimSpace(line)) > 0 {
			lines = append(lines, numberedLine{at: i + 1, text: line})
		}
	}
	return lines
}

// RecordLines is the one-based line each record of an NDJSON body arrived on,
// in the order DecodeStamped returns those records.
//
// §6.2.3 is why it is exported. That rejection is raised after the decode, over
// a slice of records that carries no line numbers of its own, and it has to
// name the line the user must open exactly as §6.1.3's rejections do — so the
// caller asks the counter DecodeStamped used rather than counting a second
// time.
func RecordLines(body []byte) []int {
	lines := numberedRecords(body)
	at := make([]int, 0, len(lines))
	for _, line := range lines {
		at = append(at, line.at)
	}
	return at
}

// writeRecords encodes records as NDJSON — one JSON document per line — and
// publishes the file through the held lock.
func writeRecords[T any](k *Lock, name string, records []T) error {
	body, err := encodeRecords(name, records)
	if err != nil {
		return err
	}
	return k.Write(name, body)
}

// encodeRecords renders records as NDJSON — one JSON document per line. It is
// shared with the context store of §3.6, which is NDJSON too but is not per-PR
// state, so it is written outside the §2.3.1 lock this file's other writers
// hold.
func encodeRecords[T any](name string, records []T) ([]byte, error) {
	var body bytes.Buffer
	for i, record := range records {
		line, err := json.Marshal(record)
		if err != nil {
			return nil, fmt.Errorf("cannot encode record %d of %s: %w", i+1, name, err)
		}
		body.Write(line)
		body.WriteByte('\n')
	}
	return body.Bytes(), nil
}

// ReadRecords decodes one NDJSON file of a pull request's state into its record
// type, so no command parses those lines itself. It takes no lock, per §2.3.2.
//
// The result is never nil: an empty file is no records rather than a null slice
// (§12.3). A line that does not decode is reported with its number, counting
// blank lines, so the error names the line the user has to open.
func ReadRecords[T any](l Layout, owner, repo string, pr int, name string) ([]T, error) {
	body, err := l.ReadPR(owner, repo, pr, name)
	if err != nil {
		return nil, err
	}
	return decodeRecords[T](l.PRFile(owner, repo, pr, name), body)
}

// storeRecords decodes one NDJSON store that lives outside a pull request's
// state directory, treating an absent file as no records.
//
// §3.6's context store and §7.4.4's repository-wide waiver file are the two.
// Each is created by the first record appended to it, so a file that is not
// there yet holds exactly as many records as an empty one does — and both
// readers share this so the two stores cannot come to disagree about what an
// absent file means. A read that failed for any other reason is reported with
// the path, and an undecodable line with its number, for the reason
// decodeRecords gives.
func storeRecords[T any](path string) ([]T, error) {
	body, err := os.ReadFile(path)
	if errors.Is(err, fs.ErrNotExist) {
		return make([]T, 0), nil
	}
	if err != nil {
		return nil, FileFailure("read", path, UnusableHint, err)
	}
	return decodeRecords[T](path, body)
}

// decodeRecords decodes an NDJSON body into its record type, naming path in
// whatever it refuses. It is shared with the context store of §3.6 for the
// reason encodeRecords is.
func decodeRecords[T any](path string, body []byte) ([]T, error) {
	records := make([]T, 0)
	for i, line := range bytes.Split(body, []byte{'\n'}) {
		if len(bytes.TrimSpace(line)) == 0 {
			continue
		}
		var record T
		if err := json.Unmarshal(line, &record); err != nil {
			return nil, fmt.Errorf("%s line %d: %w", path, i+1, err)
		}
		records = append(records, record)
	}
	return records, nil
}
