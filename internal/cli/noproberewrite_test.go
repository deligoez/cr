package cli

import (
	"go/ast"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// The calls production code is allowed to hand `state.FileProbes` to, and what
// each of them does to the file.
//
// AppendStamped is the only one that writes. It carries the bytes the file
// already holds through untouched and adds to the end, which is §5.5.1's
// immutability in the only form a file can have it: no stored line is
// re-encoded, restamped, or replaced by a write that was meant to extend the
// file. WriteStamped, the other door §2.3.3 opens onto a stamped file, is
// exactly what §5.5.1 forbids here — it stamps every record it is handed, so
// reading probes.ndjson back and passing it through would rewrite every earlier
// probe with the round now being written.
//
// ReadStamped is the second read, and it is a read in exactly the sense
// ReadRecords is: §9.3.5 has it return one round's records and it writes
// nothing, so admitting it widens what may look at the file and not what may
// change it. The assertion below still names one writer.
var probeFileCalls = map[string]string{
	"ReadRecords":   "reads",
	"ReadStamped":   "reads one round",
	"AppendStamped": "appends",
}

// §5.5.1, proved from the shape of the code rather than from a run: probe
// records are immutable once written.
//
// A test can show that one write left the earlier lines alone. It cannot show
// that the write someone adds next year will — and the way §5.5.1 gets broken
// is not a deliberate rewrite but an ordinary-looking `WriteStamped` reaching
// for the file that sits beside seven others in §2.3.3's list. What can show
// the absence of that is the absence of a call site, and this asserts it: every
// place production code names probes.ndjson is a read or the one append.
//
// The guard is over the constant rather than over the file name, so it does not
// see a write that reached the path through a variable. That is the same reach
// TestOnlyTheStatePackageWritesToTheFilesystem already fences from the other
// side — nothing outside internal/state writes to the filesystem at all — and
// what is added here is that inside cr's own state API, the file has one writer.
//
// §5.5.1 is why an earlier round's probe stays readable at all: NextID
// allocates above every id the file has ever held, and §5.5.3 is what removes
// such a record's standing in the current round rather than removing the record.
func TestProbeRecordsAreOnlyEverAppended(t *testing.T) {
	writers := map[string][]string{}
	eachSourceFile(t, func(rel string, file *ast.File) {
		for _, decl := range file.Decls {
			within, isFunc := decl.(*ast.FuncDecl)
			if !isFunc {
				continue
			}
			ast.Inspect(within, func(n ast.Node) bool {
				call, isCall := n.(*ast.CallExpr)
				if !isCall || !namesTheProbeFile(call.Args) {
					return true
				}
				writers[calledName(call)] = append(
					writers[calledName(call)], filepath.ToSlash(rel)+": "+within.Name.Name)
				return true
			})
		}
	})

	require.NotEmpty(t, writers,
		"§5.5.1: nothing names probes.ndjson, so this guard measured nothing")
	for called, sites := range writers {
		assert.Contains(t, probeFileCalls, called,
			"§5.5.1: probes.ndjson is immutable once written, and %s is neither of the two "+
				"calls that may name it; reached from %v", called, sites)
	}
	assert.Equal(t, []string{"internal/cli/probe.go: appendProbe"}, writers["AppendStamped"],
		"§5.5.1: probes.ndjson has one writer, and it is the append under §2.3.1's lock")
}

// namesTheProbeFile reports whether one call's arguments include the constant
// naming probes.ndjson, written either qualified or bare.
func namesTheProbeFile(args []ast.Expr) bool {
	const constant = "FileProbes"
	for _, arg := range args {
		switch named := arg.(type) {
		case *ast.SelectorExpr:
			if named.Sel.Name == constant {
				return true
			}
		case *ast.Ident:
			if named.Name == constant {
				return true
			}
		}
	}
	return false
}

// calledName is the function a call invokes, by the last name in it, so a
// generic instantiation reads as the function it instantiates.
func calledName(call *ast.CallExpr) string {
	fun := call.Fun
	for {
		switch inner := fun.(type) {
		case *ast.IndexExpr:
			fun = inner.X
		case *ast.IndexListExpr:
			fun = inner.X
		case *ast.SelectorExpr:
			return inner.Sel.Name
		case *ast.Ident:
			return inner.Name
		default:
			return ""
		}
	}
}
