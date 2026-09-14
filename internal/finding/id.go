package finding

import (
	"strconv"
	"strings"
)

// idPrefix is the letter §6.1 gives a record id.
const idPrefix = "f"

// ValidID reports whether id is spelled the way §6.1 spells a record id.
//
// It is the same reading NextID allocates against, exported for a command that
// takes a record id from a person rather than from the file cr wrote it to.
// §3.6.2's `cr answer` is the first of those: it names a record inside a note
// without resolving one, so the spelling is the whole of what it can check, and
// it is checked here rather than spelled a second time there.
func ValidID(id string) bool {
	_, ok := parseID(id)
	return ok
}

// IDSuffix reads the n of an f<n> record id, reporting whether id is spelled
// the way §6.1 spells one.
//
// §8.3.3 orders the payload's comments by "record id by numeric suffix", which
// is a reading of the id and not of the string: f2 sorts before f10, where a
// string comparison puts f10 first. It is the same parse NextID allocates
// against and ValidID checks, exported rather than respelled, because a second
// reading of the id would be a second answer to what an id means.
func IDSuffix(id string) (int, bool) {
	return parseID(id)
}

// NextID allocates the id for a new record.
//
// existing MUST be every record findings.ndjson holds, not the current round's.
// §6.1 makes a record id stable for the life of the pull request, so an id an
// earlier round assigned is spent even when that record went stale, was
// discarded, or was posted: reusing it would put two different records under
// one id in a file that keeps all of them (§2.3), and point one id at two
// different comments in the pull request's history.
//
// §3.4.6's unit ids follow the opposite rule — they are round-scoped and MUST
// NOT be carried across rounds — and the two allocations share nothing, so the
// round-scoped rule cannot reach a record id by way of a common helper.
func NextID(existing []Finding) string {
	highest := 0
	// Indexed rather than ranged by value: a record is a wide struct, and
	// only its id is read here.
	for i := range existing {
		// `>=` here would behave identically — it would assign the
		// value already held — so no test can tell the two apart.
		if n, ok := parseID(existing[i].ID); ok && n > highest {
			highest = n
		}
	}
	return IDOf(highest + 1)
}

// IDOf spells n as §6.1's f<n> record id, the reading IDSuffix takes back.
//
// `cr review` hands every prompt a block of ids by number (§4.6.2), and the
// spelling it tells a role is this one rather than a prefix written out there.
func IDOf(n int) string {
	return idPrefix + strconv.Itoa(n)
}

// parseID reads the n of an f<n> id.
//
// It accepts only the canonical spelling: f7 is an id, f+7, f07 and f-7 are
// not, and neither is f0, because ids are numbered from one. An id cr did not
// write is no evidence about what is taken, so it contributes nothing to the
// next allocation rather than being read as some number near it.
func parseID(id string) (int, bool) {
	rest, found := strings.CutPrefix(id, idPrefix)
	if !found {
		return 0, false
	}
	n, err := strconv.Atoi(rest)
	if err != nil || rest != strconv.Itoa(n) || n < 1 {
		return 0, false
	}
	return n, true
}

// IDForm is how parseID's spelling is told to a person: the refusal of an id
// that is not one, and the prompt telling a role how to write one (§4.6.2), read
// the same words.
const IDForm = idPrefix + "<n>, numbered from one"
