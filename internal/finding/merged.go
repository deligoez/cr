package finding

import (
	"bytes"
	"encoding/json"
	"fmt"
	"slices"
)

// duplicateOfField is §6.4.3's mark, and the one computed field §6.5.1 lets
// `cr merge` put in its output.
const duplicateOfField = "duplicate_of"

// mergedFields are §6.1's rows a merged line keeps, in the table's own order.
//
// They are derived from `fields` rather than listed, which is what makes
// §6.5.1's "no computed field except `duplicate_of`" a property of the table
// instead of a second list kept beside it: a row the spec marks computed leaves
// the output by being marked, and a row added later has to be given a
// Requirement before it can appear at all.
//
// Stamped leaves with Computed, and that pair is the reason this exists.
// state.Stamp carries head and round without `omitempty`, deliberately, because
// §2.3.3 requires both on a stored record — so marshalling a decoded Finding
// emits `"head":""` and `"round":0`, and state.DecodeStamped rejects those keys
// by presence. `cr merge`'s output is not a stored record; it is the file
// §6.5.1 hands to `cr record`, which stamps the pair itself. A merge that wrote
// them would produce a file the very next command refuses.
//
// §6.1.4's other three — `disposition`, `thread_id` and `duplicate_of` — are
// the Optional rows `alsoReserved` names, and the first two leave here with the
// computed ones. They cannot be set on anything this function sees, since every
// record reaching a merge came through a decoder that refuses all three; taking
// them out anyway costs nothing and keeps the rule readable off one list.
var mergedFields = mergedFieldNames()

// mergedFieldNames builds that list once, at package initialisation.
func mergedFieldNames() []string {
	kept := make([]string, 0, len(fields))
	for _, field := range fields {
		if field.Name == duplicateOfField {
			kept = append(kept, field.Name)
			continue
		}
		if field.Requirement == Computed || field.Requirement == Stamped ||
			slices.Contains(alsoReserved, field.Name) {
			continue
		}
		kept = append(kept, field.Name)
	}
	return kept
}

// MergedRecords renders one round's merged records as the NDJSON §6.5.1 has
// `cr merge` write to its `-o` path: one JSON document per line, each holding
// §6.1's rows that survived mergedFields and nothing else.
//
// It encodes each record and then drops keys, rather than copying the surviving
// fields into a narrower struct. A second struct would be a second
// transcription of §6.1's table — one that could fall behind the first silently,
// since a field it failed to copy would simply be absent from the output and
// every test asserting on what is *present* would still pass.
//
// The order is §6.1's, which is the order the struct declares its fields in and
// the order a reader of the spec expects, so a merged file can be read beside
// the table. Nothing depends on it: JSON object order is not semantic, and
// `cr record` decodes by key.
func MergedRecords(records []*Finding) ([]byte, error) {
	var body bytes.Buffer
	for i, record := range records {
		line, err := mergedLine(record)
		if err != nil {
			return nil, fmt.Errorf("cannot encode merged record %d: %w", i+1, err)
		}
		body.Write(line)
		body.WriteByte('\n')
	}
	return body.Bytes(), nil
}

// mergedLine is one record as `cr merge` writes it.
//
// A field the record does not hold is left out rather than written empty, which
// is the shape the struct's own `omitempty` tags already give a line and the
// shape §6.1.3 reads: `written` counts `null` and `""` as supplying nothing, so
// a merged line carrying `"claim":""` would be a key that says the agent chose
// a value while meaning the opposite.
func mergedLine(record *Finding) ([]byte, error) {
	whole, err := json.Marshal(record)
	if err != nil {
		return nil, err
	}
	var held map[string]json.RawMessage
	if err := json.Unmarshal(whole, &held); err != nil {
		return nil, err
	}
	cited, err := mergedCitations(held[citationsField])
	if err != nil {
		return nil, err
	}
	held[citationsField] = cited

	var line bytes.Buffer
	line.WriteByte('{')
	for _, name := range mergedFields {
		value := held[name]
		if len(value) == 0 {
			continue
		}
		if line.Len() > 1 {
			line.WriteByte(',')
		}
		fmt.Fprintf(&line, "%q:", name)
		line.Write(value)
	}
	line.WriteByte('}')
	return line.Bytes(), nil
}

// mergedCitations is one record's citations array with §6.2.3's `content_hash`
// and §6.2.5's `origin` taken back out of every entry.
//
// `cr merge` resolves and stamps both to compute the grade §6.4.2 ranks a
// duplicate group by, and §6.1.4 rejects a record arriving with either — so a
// merged line that carried them forward would be refused by `cr record` on the
// entry cr itself had written. The whole array goes when the record has none,
// so the key is absent rather than `[]`.
func mergedCitations(citations json.RawMessage) (json.RawMessage, error) {
	if len(citations) == 0 {
		return nil, nil
	}
	var entries []map[string]json.RawMessage
	if err := json.Unmarshal(citations, &entries); err != nil {
		return nil, err
	}
	for _, entry := range entries {
		for _, field := range citationFields {
			if field.Requirement == Computed {
				delete(entry, field.Name)
			}
		}
	}
	return json.Marshal(entries)
}
