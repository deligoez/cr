package finding

import (
	"go/ast"
	"go/parser"
	"go/token"
	"maps"
	"os"
	"slices"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// door is one call that turns bytes cr did not write into records.
type door func(t *testing.T, record map[string]any) ([]*Finding, error)

// doors is every way a record comes into being, keyed by the exported function
// that opens it, with one call per answer that function takes: §6.5.1 has
// `cr record` read the same bytes as the agent's own or as `cr merge`'s output,
// and `cr merge` reads a role's own file. A door that let an unanchored item
// through would put an item with no code location in findings.ndjson, and
// everything downstream of that file — the draft, the payload — reads records
// and asks no further questions about where they came from.
var doors = map[string][]door{
	"Decode": {
		func(t *testing.T, record map[string]any) ([]*Finding, error) {
			return Decode(FanOutFile("test"), onLineThree(t, record), roundUnits, SourceAgent)
		},
		func(t *testing.T, record map[string]any) ([]*Finding, error) {
			return Decode("merged.ndjson", onLineThree(t, record), roundUnits, SourceMerge)
		},
	},
	"DecodePerRole": {
		func(t *testing.T, record map[string]any) ([]*Finding, error) {
			return DecodePerRole(FanOutFile("test"), onLineThree(t, record), roundUnits)
		},
	},
}

// unanchored is an item with no code location, in every shape it can take once
// it is written as a record: the key left out, an anchor object holding
// nothing, one carrying the rest of §9.2's fields but no path, and one naming
// the empty path. A nil is the key left out, which no anchor object can say.
var unanchored = map[string]map[string]any{
	"no anchor at all":          nil,
	"an anchor holding nothing": {},
	"an anchor naming no path":  {"side": "RIGHT", "start_line": 12, "line": 14},
	"an anchor naming the empty path": {
		"path": "", "side": "RIGHT", "start_line": 12, "line": 14,
	},
}

// §1.6.1 leaves v0.1 no unanchored comment channel. The question this asks is
// therefore not whether the decoder refuses an item with no code location —
// §6.1.2 has it refuse, and TestAnItemWithNoCodeLocationNeverBecomesARecord
// pins that — but whether it refuses one on every way in. A posted comment is
// rendered from a record, so the set of functions that hand back records is the
// set of paths an unanchored item could take to a payload, and one of them
// forgiving is the whole fence.
//
// That set is read out of the package's own source rather than listed here, so
// a second way to obtain a record — added years from now, by someone who never
// read §1.6 — fails this test rather than quietly opening the channel §1.6.1
// closed. The fence is on the way in: a record on disk arrived through one of
// these, and no reader re-checks it.
func TestNoUnanchoredItemBecomesARecordByAnyDoor(t *testing.T) {
	require.ElementsMatch(t, slices.Collect(maps.Keys(doors)), recordDoors(t),
		"a new way to obtain a record is a new way to a payload: drive it here and prove it refuses")

	for name, calls := range doors {
		t.Run(name, func(t *testing.T) {
			for _, call := range calls {
				records, err := call(t, aRecord())
				require.NoError(t, err, "the same fixture passes once it is anchored")
				require.Len(t, records, 2)

				for shape, anchor := range unanchored {
					item := aRecord()
					if anchor == nil {
						delete(item, "anchor")
					} else {
						item["anchor"] = anchor
					}
					_, err := call(t, item)
					var rejected *RejectedRecordError
					require.ErrorAs(t, err, &rejected, shape)
					assert.Equal(t, "anchor", rejected.Field, shape)
				}
			}
		})
	}
}

// recordDoors returns the name of every exported function of this package whose
// results carry a Finding, which is everything outside the package can obtain a
// record from. A method is named by its receiver type as well.
func recordDoors(t *testing.T) []string {
	t.Helper()
	sources, err := os.ReadDir(".")
	require.NoError(t, err)

	names := make([]string, 0, len(doors))
	for _, source := range sources {
		name := source.Name()
		if !strings.HasSuffix(name, ".go") || strings.HasSuffix(name, "_test.go") {
			continue
		}
		parsed, err := parser.ParseFile(token.NewFileSet(), name, nil, parser.SkipObjectResolution)
		require.NoError(t, err)
		for _, decl := range parsed.Decls {
			fn, ok := decl.(*ast.FuncDecl)
			if !ok || !fn.Name.IsExported() || !handsBackARecord(fn.Type.Results) {
				continue
			}
			names = append(names, doorName(fn))
		}
	}
	return names
}

// handsBackARecord reports whether a result list names the record type at all,
// so a *Finding, a []*Finding, and a shape nobody has written yet all count.
func handsBackARecord(results *ast.FieldList) bool {
	if results == nil {
		return false
	}
	found := false
	ast.Inspect(results, func(node ast.Node) bool {
		if named, ok := node.(*ast.Ident); ok && named.Name == "Finding" {
			found = true
		}
		return !found
	})
	return found
}

// doorName is the function's name, prefixed by the receiver type when it is a
// method, so a door added as a method is named distinctly from a function of
// the same name. Only the type is walked; the receiver's own name is not part
// of how anything calls it.
func doorName(fn *ast.FuncDecl) string {
	if fn.Recv == nil || len(fn.Recv.List) == 0 {
		return fn.Name.Name
	}
	receiver := ""
	ast.Inspect(fn.Recv.List[0].Type, func(node ast.Node) bool {
		if named, ok := node.(*ast.Ident); ok && receiver == "" {
			receiver = named.Name
		}
		return receiver == ""
	})
	return receiver + "." + fn.Name.Name
}
