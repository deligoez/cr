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
)

// ReplaceStamped replaces the current round's records in one of the eight
// §2.3.3 files, stamping head and round onto every record it writes and leaving
// every earlier round's line byte for byte. It refuses a file §2.3.3 does not
// list, as WriteStamped does.
//
// claims.ndjson is what it exists for. §3.3.1 has `cr claims record` replace
// that file, and §9.3.5 reads the same sentence back with a scope: a command
// documented as replacing or clearing a file does so for the current round
// only and MUST leave earlier rounds intact.
//
// Neither of the two writers already here can do that job, and each fails in
// its own direction. WriteStamped publishes only what it is handed, so a
// replacement written through it would take every earlier round's records out
// of the file — the history §9.3.5 protects, deleted by the write that was
// meant to replace one round of it. AppendStamped keeps them, and keeps the
// current round's records too, so a second `cr claims record` in one round
// would leave both extractions in the file and every reader would see each
// claim twice.
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

// ClearStamped drops the current round's records from one of the eight §2.3.3
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

// checkStamped refuses a file §2.3.3 does not list, so a round-scoped write
// cannot be aimed at a file whose records carry no round to scope by.
func checkStamped(name string) error {
	if !slices.Contains(stampedFiles, name) {
		return fmt.Errorf("%s: §2.3.3 does not list it, so its records carry no head or round", name)
	}
	return nil
}

// roundOf is the one field a round-scoped write reads out of a stored line:
// §2.3.3's round, which is the whole of what makes a line belong to a round.
//
// It is deliberately not the record's own type. This file is generic over
// eight different record shapes, and decoding each line into its own struct
// would mean a line the current version cannot fully decode — a field a later
// version added — deciding the fate of a round it has nothing to do with.
type roundOf struct {
	Round int `json:"round"`
}

// earlierRounds returns the lines of one §2.3.3 file that do not belong to
// round, in file order and byte for byte.
//
// Every round but the named one is kept, not merely the ones before it. §9.3.5
// calls the others history and this function has no way to tell a round that
// has passed from one a state directory carries for some other reason, so it
// takes out exactly the round it was asked to take out and nothing else.
//
// The read is unlocked-safe because the caller holds the §2.3.1 lock: no other
// writer can be between this read and the Write that follows it. A file that is
// not there yet holds as many records as an empty one, which is the reading
// storeRecords already gives an absent store.
func earlierRounds(k *Lock, name string, round int) ([]byte, error) {
	path := filepath.Join(k.dir, name)
	held, err := os.ReadFile(path)
	if err != nil && !errors.Is(err, fs.ErrNotExist) {
		return nil, fmt.Errorf("cannot read %s: %w", path, err)
	}
	var kept bytes.Buffer
	for i, line := range bytes.Split(held, []byte{'\n'}) {
		if len(bytes.TrimSpace(line)) == 0 {
			continue
		}
		var at roundOf
		if err := json.Unmarshal(line, &at); err != nil {
			return nil, fmt.Errorf("%s line %d: %w", path, i+1, err)
		}
		if at.Round == round {
			continue
		}
		kept.Write(line)
		kept.WriteByte('\n')
	}
	return kept.Bytes(), nil
}
