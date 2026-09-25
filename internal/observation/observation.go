// Package observation models what a role saw outside its unit, per
// spec/0.16.0.md §4.6.9.
//
// A role reads one unit, and the unit's change can bear on code the unit does
// not contain. Measured on a real pull request on a private Laravel
// repository: a role noticed that the same `locale:` defect the change fixed in
// one directory still sat in a sibling directory's file outside the diff, and
// nothing it could write had anywhere to go. A record is anchored inside a unit
// (§6.2.1), so the observation was lost.
//
// An observation is not a record. It is never graded, posted or counted: it is
// stored in observations.ndjson and shown to the human by `cr draft` and
// `cr status`, and what to do about it is the human's call. The agent writes
// it, this package validates it, and nothing in cr reads it for what it says.
package observation

import (
	"encoding/json"
	"errors"
	"fmt"
	"slices"
	"strconv"
	"strings"

	"github.com/deligoez/cr/internal/probe"
	"github.com/deligoez/cr/internal/state"
)

// fanOutPrefix and fanOutSuffix are the name §4.6.9 gives the file one role
// writes its observations about one unit to, beside the records, proposals and
// cell §4.6.2 names.
const (
	fanOutPrefix = "observations-"
	fanOutSuffix = ".ndjson"
)

// FanOutFile is the file §4.6.9 has one role write its observations to.
func FanOutFile(role string) string {
	return fanOutPrefix + role + fanOutSuffix
}

// IsFanOutFile reports whether name is the file §4.6.9 has a role write its
// observations to: `observations-<role>.ndjson`, not anything that merely
// opens so.
func IsFanOutFile(name string) bool {
	id, found := strings.CutPrefix(name, fanOutPrefix)
	if !found {
		return false
	}
	id, found = strings.CutSuffix(id, fanOutSuffix)
	return found && id != ""
}

// fields are §4.6.9's `{path, line, text}`, in that order.
var fields = []string{"path", "line", "text"}

// Fields returns §4.6.9's fields in the order the section names them. The
// result is a copy.
func Fields() []string {
	return slices.Clone(fields)
}

// Observation is one line of observations.ndjson: §4.6.9's three fields, and
// the head and round `cr observations record` stored it at.
//
// The pair is state.Stamp, so the agent's line is refused when it carries
// either, as every recorded file refuses it; observations.ndjson is not one of
// §2.3.3's nine, so the command writes the pair itself.
type Observation struct {
	// Path is the repository-relative file the observation is about.
	Path string `json:"path"`
	// Line is the line of Path it is about, and zero for the file as a
	// whole.
	Line int `json:"line,omitempty"`
	// Text is what the role saw, in English.
	Text string `json:"text"`
	state.Stamp
}

// Location is the observation's `path:line`, or its path alone when it names
// no line.
func (o *Observation) Location() string {
	if o.Line == 0 {
		return o.Path
	}
	return o.Path + ":" + strconv.Itoa(o.Line)
}

// RejectedError reports an observation §4.6.9 refuses. The cli layer maps it
// onto exit code 1.
type RejectedError struct {
	// File is the NDJSON file the agent handed the command.
	File string
	// Line is the one-based line the observation sits on.
	Line int
	// Field is the field the refusal blames.
	Field string
	// Problem completes the sentence naming that field.
	Problem string
}

func (e *RejectedError) Error() string {
	return fmt.Sprintf("%s line %d: %s %s", e.File, e.Line, e.Field, e.Problem)
}

// Decode reads the observations an agent hands `cr observations record`,
// refusing a line that carries a key §4.6.9 does not name, that leaves `path`
// or `text` empty, or whose `path:line` does not resolve at the head.
//
// head reads a file at the round's head. A read that failed because the clone
// lacks the head is returned as it came, for the command to answer with the
// fetch that fixes it, and never as the line's fault.
func Decode(file string, body []byte, head probe.HeadFile) ([]*Observation, error) {
	return state.DecodeStamped[Observation](file, body,
		func(line int, supplied map[string]json.RawMessage, o *Observation) error {
			reject := func(field, problem string) error {
				return &RejectedError{File: file, Line: line, Field: field, Problem: problem}
			}
			for name := range supplied {
				if !slices.Contains(fields, name) {
					return reject(name, "is not a field of §4.6.9; an observation carries "+strings.Join(fields, ", "))
				}
			}
			if strings.TrimSpace(o.Path) == "" {
				return reject("path", "is required by §4.6.9 and this line leaves it empty")
			}
			if strings.TrimSpace(o.Text) == "" {
				return reject("text", "is required by §4.6.9 and this line leaves it empty")
			}
			if _, written := supplied["line"]; written && o.Line < 1 {
				return reject("line", fmt.Sprintf("reads %d; a line is numbered from 1, and an observation about "+
					"the whole file leaves line out", o.Line))
			}
			return resolves(head, o, reject)
		})
}

// resolves holds the observation's location to the head, as §6.2.3 holds a
// citation: the path must be a file the head holds, and the line one it has.
func resolves(head probe.HeadFile, o *Observation, reject func(field, problem string) error) error {
	if o.Line == 0 {
		_, exists, err := head(o.Path)
		if err != nil {
			return err
		}
		if !exists {
			return reject("path", fmt.Sprintf("names %q, which the head under review does not hold as a file", o.Path))
		}
		return nil
	}
	err := probe.CheckTarget(head, o.Location())
	invalid := &probe.InvalidTargetError{}
	if errors.As(err, &invalid) {
		return reject("path", fmt.Sprintf("and line read %q, which does not resolve at the round's head: %s",
			o.Location(), invalid.Problem))
	}
	return err
}
