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

// writeRecords encodes records as NDJSON — one JSON document per line — and
// publishes the file through the held lock.
func writeRecords[T any](k *Lock, name string, records []T) error {
	var body bytes.Buffer
	for i, record := range records {
		line, err := json.Marshal(record)
		if err != nil {
			return fmt.Errorf("cannot encode record %d of %s: %w", i+1, name, err)
		}
		body.Write(line)
		body.WriteByte('\n')
	}
	return k.Write(name, body.Bytes())
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
	records := make([]T, 0)
	for i, line := range bytes.Split(body, []byte{'\n'}) {
		if len(bytes.TrimSpace(line)) == 0 {
			continue
		}
		var record T
		if err := json.Unmarshal(line, &record); err != nil {
			return nil, fmt.Errorf("%s line %d: %w", l.PRFile(owner, repo, pr, name), i+1, err)
		}
		records = append(records, record)
	}
	return records, nil
}
