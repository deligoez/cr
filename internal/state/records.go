package state

import (
	"bytes"
	"encoding/json"
	"fmt"
	"slices"
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
	if !slices.Contains(stampedFiles, name) {
		return fmt.Errorf("%s: §2.3.3 does not list it, so its records carry no head or round", name)
	}
	for _, record := range records {
		record.setStamp(at)
	}
	return writeRecords(k, name, records)
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
// fields by JSON key — so a check can tell a field the agent omitted from one
// it wrote empty — and the decoded record. A command with nothing to add passes
// nil.
func DecodeStamped[E any, T interface {
	*E
	Stamped
}](file string, body []byte, check func(int, map[string]json.RawMessage, T) error) ([]T, error) {
	records := make([]T, 0)
	for i, line := range bytes.Split(body, []byte{'\n'}) {
		if len(bytes.TrimSpace(line)) == 0 {
			continue
		}
		var supplied map[string]json.RawMessage
		if err := json.Unmarshal(line, &supplied); err != nil {
			return nil, fmt.Errorf("%s line %d: %w", file, i+1, err)
		}
		for _, field := range stampFields {
			if _, written := supplied[field]; written {
				return nil, &ReservedFieldError{File: file, Line: i + 1, Field: field}
			}
		}
		record := T(new(E))
		if err := json.Unmarshal(line, record); err != nil {
			return nil, fmt.Errorf("%s line %d: %w", file, i+1, err)
		}
		if check != nil {
			if err := check(i+1, supplied, record); err != nil {
				return nil, err
			}
		}
		records = append(records, record)
	}
	return records, nil
}

// writeRecords encodes records as NDJSON — one JSON document per line — and
// publishes the file through the held lock.
// writeRecords encodes records as NDJSON and publishes the file through the
// held lock.
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
