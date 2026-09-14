package finding

import (
	"fmt"
	"regexp"
)

// classPattern is §6.1's defect class, exactly as the table writes it:
// [a-z0-9-]+, anchored end to end so nothing but a class matches it. Go's $
// ends the text rather than a line, so a value carrying a newline is not a
// class either.
var classPattern = regexp.MustCompile("^" + ClassForm + "$")

// ClassForm is §6.1's class form as the table writes it, unanchored: the
// pattern classPattern anchors, and the form a refusal and a prompt (§4.6.2)
// name.
const ClassForm = "[a-z0-9-]+"

// InvalidClassError reports a record whose class is not the form §6.1 fixes.
// It carries the file and the line so the user can open the record, and the
// value so they can see what was rejected. The cli layer maps it onto exit code
// 1: the file was read and parsed, and what is wrong is the data inside it.
type InvalidClassError struct {
	// File is the NDJSON file the agent handed the command.
	File string
	// Line is the one-based line the record sits on, counting blank lines.
	Line int
	// Class is the offending value, exactly as it was written.
	Class string
}

func (e *InvalidClassError) Error() string {
	return fmt.Sprintf(
		"%s line %d: class %q is not kebab-case; §6.1 fixes the form at %s",
		e.File, e.Line, e.Class, ClassForm,
	)
}

// ValidateClass holds one record's class to §6.1's form.
//
// The class is not decoration. §6.4.1 deduplicates on it, §7.3 counts triage
// outcomes by it, and §7.4 waives by it, so two spellings of one class are two
// classes: the duplicate goes unsuppressed, the waiver misses, and the
// statistics that would demote a noisy class quietly stop counting it. Every
// other form is rejected so that one defect class stays one string.
func ValidateClass(file string, line int, class string) error {
	if classPattern.MatchString(class) {
		return nil
	}
	return &InvalidClassError{File: file, Line: line, Class: class}
}
