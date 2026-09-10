package cli

import (
	"reflect"
	"strings"
	"testing"

	"github.com/spf13/cobra"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/deligoez/cr/internal/coverage"
	"github.com/deligoez/cr/internal/finding"
	"github.com/deligoez/cr/internal/intent"
	"github.com/deligoez/cr/internal/mapping"
)

// bodyWord is what a flag, a positional, or a field would be called if it
// carried a comment body into cr.
const bodyWord = "body"

// everyUseInTheTree collects the Use line of every command in the tree, at any
// depth, which is where cobra keeps a command's positional arguments.
func everyUseInTheTree(cmd *cobra.Command) []string {
	uses := []string{cmd.Use}
	for _, child := range cmd.Commands() {
		uses = append(uses, everyUseInTheTree(child)...)
	}
	return uses
}

// jsonNames collects every JSON key a value of type t decodes from, following
// nested structs, pointers, slices, and embedded structs — the keys an agent
// could write into a file handed to cr.
func jsonNames(t reflect.Type) []string {
	for t.Kind() == reflect.Pointer || t.Kind() == reflect.Slice {
		t = t.Elem()
	}
	if t.Kind() != reflect.Struct {
		return nil
	}
	names := make([]string, 0, t.NumField())
	for i := range t.NumField() {
		field := t.Field(i)
		if !field.IsExported() {
			continue
		}
		name, _, _ := strings.Cut(field.Tag.Get("json"), ",")
		if name == "-" {
			continue
		}
		if name != "" {
			names = append(names, name)
		}
		names = append(names, jsonNames(field.Type)...)
	}
	return names
}

// §8.1.2: editing `draft.md` is the only input path for reader-facing prose,
// and cr accepts a body through no other channel.
//
// A channel is anything a caller can hand cr, so each is walked rather than
// sampled. The command line is two of them — a flag and a positional — and the
// whole tree is walked for both. The files are the other: every record type a
// command decodes from an agent's file is walked for a key that would hold a
// body. `cr answer` and `cr note` take a `<text>`, and that is a fact for
// §3.6's context store rather than a comment: it is stored with its source and
// never becomes the prose of a block.
//
// The last half is the one that proves "does not accept" rather than "does not
// declare". A key Go's decoder does not know is dropped on the way in, so a
// record written with a body of its own is stored, drafted, and rendered — and
// the block it reaches holds the summary and the evidence and not one word of
// that body.
func TestNoCommandAcceptsABodyArgumentOrABodyField(t *testing.T) {
	root := newRootCmd()

	flags := everyFlagInTheTree(root)
	require.NotEmpty(t, flags, "a walk that found no flag proves nothing")
	for _, name := range flags {
		assert.NotContainsf(t, strings.ToLower(name), bodyWord,
			"§8.1.2: --%s would be a second channel for a body", name)
	}

	uses := everyUseInTheTree(root)
	require.Greater(t, len(uses), 1, "a walk that found only the root proves nothing")
	for _, use := range uses {
		assert.NotContainsf(t, strings.ToLower(use), bodyWord,
			"§8.1.2: %q would take a body as an argument", use)
	}

	for _, decoded := range []reflect.Type{
		reflect.TypeFor[finding.Finding](),
		reflect.TypeFor[intent.Claim](),
		reflect.TypeFor[mapping.Pair](),
		reflect.TypeFor[coverage.Cell](),
	} {
		names := jsonNames(decoded)
		require.NotEmptyf(t, names, "%s decodes from no key at all", decoded)
		for _, name := range names {
			assert.NotContainsf(t, name, bodyWord,
				"§8.1.2: %s decodes a %q key, which would be a body arriving in a file", decoded, name)
		}
	}

	const smuggled = "An agent-written body that no command may carry into a block."
	record := aGradedRecord("f1")
	record[bodyWord] = smuggled
	block := draftedBlockFor(t, gradedHome(t), record)
	assert.NotContains(t, block, smuggled,
		"§8.1.2: a body field in a record file is not a channel into the draft")
	assert.Contains(t, block, record["summary"].(string)+"\n\n"+record["evidence"].(string),
		"§8.1.2: the block's initial body is the summary and the evidence")
}
