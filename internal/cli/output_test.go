package cli

import (
	"bytes"
	"encoding/json"
	"go/ast"
	"go/parser"
	"go/token"
	"io"
	"maps"
	"os"
	"path/filepath"
	"reflect"
	"slices"
	"strings"
	"testing"

	"github.com/creack/pty"

	"github.com/deligoez/cr/internal/state"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// crHome points the state tree of spec/0.1.0.md §2.2 at a temporary directory,
// so a command resolves the built-in defaults and reads nothing of the author's
// own. It returns the root, which is what `cr init` reports.
func crHome(t *testing.T) string {
	t.Helper()
	root := filepath.Join(t.TempDir(), ".cr")
	require.NoError(t, os.MkdirAll(root, 0o700))
	t.Setenv(state.HomeEnv, root)
	return root
}

// execute runs one command with its output on file.
//
// The file is the whole point. §12.1 keys the output shape on what stdout is,
// so a test that hands the command a buffer and a flag saying what to pretend
// about it proves that the branch runs and nothing about the detection. Every
// caller here passes a real open file, and the answer comes from the kernel.
func execute(t *testing.T, file *os.File, args ...string) {
	t.Helper()
	cmd := newRootCmd()
	cmd.SetOut(file)
	cmd.SetErr(file)
	cmd.SetArgs(args)
	require.NoError(t, cmd.Execute())
}

// throughAPipe runs a command with stdout on a real pipe and returns what came
// out of the other end. This is how an agent runs cr.
func throughAPipe(t *testing.T, args ...string) string {
	t.Helper()
	reader, writer, err := os.Pipe()
	require.NoError(t, err)
	t.Cleanup(func() { reader.Close() })

	// Drained concurrently, so a command whose output outgrows the pipe
	// buffer cannot deadlock the test it is being read by.
	drained := make(chan string, 1)
	go func() {
		read, _ := io.ReadAll(reader)
		drained <- string(read)
	}()

	execute(t, writer, args...)
	require.NoError(t, writer.Close())
	return <-drained
}

// terminalSentinel is written to the pseudo-terminal after the command has
// finished, so the reader knows it has everything without waiting for the file
// to close. It is not a value any command produces.
const terminalSentinel = "cr-output-drained"

// throughATerminal runs a command with stdout on the slave side of a real
// pseudo-terminal, which is the one way to prove the detection rather than the
// branch behind it. A boolean the test sets itself would exercise the same
// line of code and establish nothing about what happens in front of a person.
//
// The sentinel is what makes the read deterministic. Closing the slave and
// reading to the end is the obvious shape and it is a race: the master reports
// EIO the moment the last slave goes, and output still sitting in the terminal
// buffer is lost with it, so the test passes or reports nothing at all
// depending on which side got there first. Reading concurrently keeps a command
// larger than the buffer from deadlocking, and stopping at the sentinel keeps
// it from depending on a close at all.
func throughATerminal(t *testing.T, args ...string) string {
	t.Helper()
	master, slave, err := pty.Open()
	require.NoError(t, err)
	t.Cleanup(func() { master.Close() })
	t.Cleanup(func() { slave.Close() })

	drained := make(chan string, 1)
	go func() {
		var read []byte
		chunk := make([]byte, 4096)
		for {
			n, err := master.Read(chunk)
			read = append(read, chunk[:n]...)
			if err != nil || bytes.Contains(read, []byte(terminalSentinel)) {
				break
			}
		}
		drained <- string(read)
	}()

	execute(t, slave, args...)
	_, err = slave.WriteString(terminalSentinel + "\n")
	require.NoError(t, err)

	// A terminal's line discipline turns \n into \r\n on the way out.
	out := strings.ReplaceAll(<-drained, "\r\n", "\n")
	out, _, ok := strings.Cut(out, terminalSentinel)
	require.True(t, ok, "the terminal never handed back the sentinel; it gave %q", out)
	return out
}

// §12.1: output is JSON when stdout is not a terminal. No flag is needed to
// reach it, which is the half of the rule that matters most — cr's caller is an
// agent reading a pipe, and the shape it parses is the one it gets by default.
func TestAPipedCommandEmitsJSON(t *testing.T) {
	crHome(t)

	out := throughAPipe(t, "config")

	var printed map[string]any
	require.NoError(t, json.Unmarshal([]byte(out), &printed), "a pipe was given %q", out)
	assert.Equal(t, float64(20), printed["post.max_comments"])
	assert.NotContains(t, out, "\x1b[", "an escape sequence in a JSON document is a defect in the document")
}

// §12.1's other half: a terminal is given human-readable coloured text, and it
// is the only thing that is. The colour is asserted rather than assumed,
// because a rendering that reads the flag and then never uses the answer is
// indistinguishable from one that got the flag wrong.
func TestATerminalCommandEmitsColouredText(t *testing.T) {
	crHome(t)

	out := throughATerminal(t, "config")

	assert.False(t, json.Valid([]byte(out)), "a terminal was given JSON: %q", out)
	assert.Contains(t, out, "\x1b[36mpost.max_comments\x1b[0m = 20")
}

// §12.1 gives `--json` one job, and it is the only override there is: a
// terminal that would have been given text is given JSON instead. Nothing
// forces the other direction, so this is the whole of the flag.
func TestTheJSONFlagForcesJSONOnATerminal(t *testing.T) {
	crHome(t)

	out := throughATerminal(t, "config", "--json")

	var printed map[string]any
	require.NoError(t, json.Unmarshal([]byte(out), &printed), "a terminal under --json was given %q", out)
	assert.Equal(t, float64(20), printed["post.max_comments"])
	assert.NotContains(t, out, "\x1b[", "colour belongs to the text rendering and to nothing else")
}

// §11.1 gives `--no-color` the colour and nothing beyond it. There is no flag
// that forces text, so the shape is unchanged in both directions: a pipe under
// `--no-color` is still JSON, and a terminal under it is still text.
//
// The two halves are one test because the claim is about the boundary between
// them. A run that answered either half alone would be consistent with
// `--no-color` having quietly become a second way to ask for text.
func TestNoColorStripsColourAndNothingElse(t *testing.T) {
	crHome(t)

	piped := throughAPipe(t, "config", "--no-color")
	assert.True(t, json.Valid([]byte(piped)), "a pipe under --no-color was given %q", piped)

	terminal := throughATerminal(t, "config", "--no-color")
	assert.False(t, json.Valid([]byte(terminal)), "a terminal under --no-color was given JSON: %q", terminal)
	assert.Contains(t, terminal, "post.max_comments = 20")
	assert.NotContains(t, terminal, "\x1b[", "--no-color left an escape sequence behind")
}

// §12.2: the JSON is pretty-printed with two-space indentation.
//
// Re-indenting what cr printed has to be a no-op, which is a stronger claim
// than reading one line off the top: it holds at every depth the document has,
// and it fails for a tab, for four spaces, and for a document printed compact
// alike. The line is then read as well, because an equality between two
// derived strings is hard to see a width in.
func TestJSONIsPrettyPrintedWithTwoSpaceIndentation(t *testing.T) {
	crHome(t)

	out := strings.TrimSuffix(throughAPipe(t, "config"), "\n")

	var compact, indented bytes.Buffer
	require.NoError(t, json.Compact(&compact, []byte(out)), "a pipe was given %q", out)
	require.NoError(t, json.Indent(&indented, compact.Bytes(), "", "  "))
	assert.Equal(t, indented.String(), out)
	assert.Contains(t, out, "\n  \"post.max_comments\": 20")
}

// outputDeciders are the imports that would let a file under internal/cli
// answer §12.1 for itself: the terminal check, the encoder, and the colour.
// Each is a way of deciding what output looks like, and the writer is where all
// three are already decided.
var outputDeciders = map[string]string{
	"github.com/mattn/go-isatty": "asks for itself whether stdout is a terminal",
	"encoding/json":              "encodes its own output",
	"github.com/fatih/color":     "reaches for colour of its own",
}

// §12.1's decision is made once, in the shared writer, and not per command.
//
// The rule is not that today's commands happen to route through it — they do,
// and a reader can check that by eye — but that a command added later cannot
// quietly reach a second answer. Imports are what that costs: a command cannot
// consult a terminal, encode a document, or emit a colour without naming the
// package that does it, and none of the three can be spelled around.
//
// Only internal/cli is fenced. A package below it parses JSON from files and
// must go on doing so; what it may not do is decide what a command prints,
// which it has no way to reach from there anyway.
func TestOnlyTheSharedWriterDecidesTheOutputShape(t *testing.T) {
	commands := filepath.Join("internal", "cli") + string(filepath.Separator)
	theWriter := filepath.Join("internal", "cli", "output.go")

	var found []string
	crSource(t, func(rel string, imports []string) {
		if !strings.HasPrefix(rel, commands) || rel == theWriter {
			return
		}
		for _, name := range imports {
			if why, ok := outputDeciders[name]; ok {
				found = append(found, rel+" "+why)
			}
		}
	})

	assert.Empty(t, found,
		"§12.1: the output shape is settled once in "+theWriter+", so no command may settle it again")
}

// Both shapes belong to every command, not to the one whose own test happens to
// read them. `cr config` proves the pair above; `cr init` is the other command
// in the tree, and its terminal rendering has a job of its own — §2.2 puts the
// state tree wherever $CR_HOME says, so the directory it names is the answer to
// a question the user could not have answered themselves.
func TestATerminalInitNamesTheStateDirectory(t *testing.T) {
	root := crHome(t)

	out := throughATerminal(t, "init")

	assert.Contains(t, out, "state directory ready at ")
	assert.Contains(t, out, "\x1b[36m"+root+"\x1b[0m")
}

// nestedInner is an inner record carrying a slice of its own.
type nestedInner struct {
	Items []string `json:"items"`
}

// hiddenTags is embedded unexported, which encoding/json promotes the exported
// fields of into the document and reflection refuses to hand over whole.
type hiddenTags struct {
	Flags []string `json:"flags"`
}

// nestedPayload is every place a null can hide in one value: a slice, a slice
// of slices, a slice of structs, a pointer, a map's value, an any, and a field
// promoted out of an embedding.
//
// It is a test's own type because no payload v0.1 prints carries a slice at
// all. The walk over the real ones is therefore true of every one of them
// today and says nothing about the reach of the rule it is walking; this is
// the shape that rule has to hold for the day a payload does.
type nestedPayload struct {
	hiddenTags
	Tags   []string               `json:"tags"`
	Groups [][]string             `json:"groups"`
	Rows   []nestedInner          `json:"rows"`
	Nested *nestedInner           `json:"nested"`
	ByName map[string]nestedInner `json:"by_name"`
	Deep   []map[string][]string  `json:"deep"`
	Loose  any                    `json:"loose"`
}

// §12.3 holds wherever a nil slice can sit and not only at a payload's own top
// layer, which is all a marshal of the outermost value would answer for.
func TestANullHidesNowhereInAPayload(t *testing.T) {
	payload := nestedPayload{
		hiddenTags: hiddenTags{Flags: nil},
		Tags:       nil,
		Groups:     [][]string{nil},
		Rows:       []nestedInner{{Items: nil}},
		Nested:     &nestedInner{Items: nil},
		ByName:     map[string]nestedInner{"one": {Items: nil}},
		Deep:       []map[string][]string{{"two": nil}},
		Loose:      []string(nil),
	}

	encoded, err := json.Marshal(withoutNilSlices(payload))
	require.NoError(t, err)

	assert.NotContains(t, string(encoded), "null", "§12.3: the payload printed %s", encoded)
}

// The one thing §12.3 is not applied to is a type that serialises itself, and
// json.RawMessage is the case that shows why it cannot be: an empty raw
// message is not an empty array but a document holding nothing, and filling it
// would not print a null — it would fail to encode at all and print nothing.
// So the null a self-marshalling type asks for is the null it gets.
func TestATypeThatSerialisesItselfKeepsItsOwnNull(t *testing.T) {
	payload := struct {
		Raw json.RawMessage `json:"raw"`
	}{}

	encoded, err := json.Marshal(withoutNilSlices(payload))
	require.NoError(t, err)

	assert.JSONEq(t, `{"raw":null}`, string(encoded))
}

// outputStructs is one value of every payload a command can hand to emit,
// which is the set §12.3 has to hold for. It is proven complete below rather
// than trusted: a payload nobody listed here is a payload nobody checked, and
// a list somebody has to remember to extend is the failure §12.3 keeps having.
var outputStructs = []result{initResult{}, configResult(nil)}

// payloadName is a payload's own type name, whether the value listed is the
// type or a pointer to it, so it can be matched against the receiver its Text
// method names in the source.
func payloadName(payload result) string {
	structure := reflect.TypeOf(payload)
	if structure.Kind() == reflect.Pointer {
		structure = structure.Elem()
	}
	return structure.Name()
}

// §12.3: no slice reaches a printed document as null.
//
// The payloads are walked zero-valued, which is the state that makes the rule
// bite: a slice a constructor would have made empty is nil until the
// constructor runs, and nothing runs one here. Every other position is
// materialised instead — a pointer allocated, a map given an entry, an any
// holding a nil list, which is how `cr config`'s list settings arrive — so a
// null can hide nowhere, and no null the document does print has an innocent
// explanation. That is what lets the document's own text be the assertion.
//
// A payload holding a type that marshals itself is beyond this. Such a type
// decides what its own nulls mean and withoutNilSlices leaves it alone, so if
// one ever reaches an output struct this is where it stops and asks.
func TestNoOutputStructPrintsANullArray(t *testing.T) {
	listed := make([]string, 0, len(outputStructs))
	for _, payload := range outputStructs {
		listed = append(listed, payloadName(payload))
	}
	require.ElementsMatch(t, slices.Collect(maps.Keys(emittablePayloads(t))), listed,
		"a payload cr can print is a place a null array can appear: walk it here too")

	for _, payload := range outputStructs {
		t.Run(payloadName(payload), func(t *testing.T) {
			structure := reflect.TypeOf(payload)
			for open := 0; open <= sliceLayers(structure, map[reflect.Type]bool{}); open++ {
				probe := zeroPayload(structure, open, map[reflect.Type]bool{})

				var printed bytes.Buffer
				out := &writer{out: &printed, mode: ModeJSON}
				require.NoError(t, out.emit(probe.Interface().(result)))

				assert.NotContains(t, printed.String(), "null",
					"§12.3: with %d slice layer(s) opened, the payload printed %s", open, printed.String())
			}
		})
	}
}

// emittablePayloads reads out of internal/cli's own source the name of every
// type a command can emit — one whose Text method takes the writer, which is
// what the result interface asks for — against the file that declares it.
//
// Nothing outside this package can implement it — writer is unexported — so
// the package's source is the whole list, and a payload written years from now
// by someone who never read §12.3 is on it whether or not they remember this
// test exists.
func emittablePayloads(t *testing.T) map[string]string {
	t.Helper()
	sources, err := os.ReadDir(".")
	require.NoError(t, err)

	names := make(map[string]string, len(outputStructs))
	for _, source := range sources {
		name := source.Name()
		if !strings.HasSuffix(name, ".go") || strings.HasSuffix(name, "_test.go") {
			continue
		}
		parsed, err := parser.ParseFile(token.NewFileSet(), name, nil, parser.SkipObjectResolution)
		require.NoError(t, err)
		for _, decl := range parsed.Decls {
			if method, ok := decl.(*ast.FuncDecl); ok && rendersForTheWriter(method) {
				names[receiverName(method)] = name
			}
		}
	}
	return names
}

// rendersForTheWriter reports whether a declaration is the result interface's
// method: a method named Text taking the writer and nothing else.
func rendersForTheWriter(method *ast.FuncDecl) bool {
	if method.Recv == nil || method.Name.Name != "Text" || len(method.Type.Params.List) != 1 {
		return false
	}
	pointer, ok := method.Type.Params.List[0].Type.(*ast.StarExpr)
	if !ok {
		return false
	}
	named, ok := pointer.X.(*ast.Ident)
	return ok && named.Name == "writer"
}

// receiverName is the type a method hangs off, with any pointer dropped. Only
// the type is walked; what the receiver calls itself is no part of the name.
func receiverName(method *ast.FuncDecl) string {
	name := ""
	ast.Inspect(method.Recv.List[0].Type, func(node ast.Node) bool {
		if named, ok := node.(*ast.Ident); ok && name == "" {
			name = named.Name
		}
		return name == ""
	})
	return name
}

// sliceLayers is how deep slices nest inside a type: a []string is one layer,
// a [][]string or a []struct{ Tags []string } is two. It bounds the probes
// below, since a nil under two slices is unreachable until the first one has
// an element to look inside.
func sliceLayers(structure reflect.Type, seen map[reflect.Type]bool) int {
	if seen[structure] {
		return 0
	}
	seen[structure] = true
	defer delete(seen, structure)

	switch structure.Kind() {
	case reflect.Slice:
		return 1 + sliceLayers(structure.Elem(), seen)
	case reflect.Pointer, reflect.Array, reflect.Map:
		return sliceLayers(structure.Elem(), seen)
	case reflect.Struct:
		deepest := 0
		for i := range structure.NumField() {
			if field := structure.Field(i); field.IsExported() {
				deepest = max(deepest, sliceLayers(field.Type, seen))
			}
		}
		return deepest
	default:
		return 0
	}
}

// zeroPayload builds the payload a command would hand over having filled in
// nothing: every string empty, every number zero. Everything a JSON document
// can reach is materialised, so no position goes unprinted for want of a value
// — except slices, which are left nil, that being the state §12.3 forbids the
// document.
//
// open is how many layers of slices are given one element each before the rest
// are left nil. A nil slice inside a slice of structs is invisible until the
// outer slice has an element, so the probe is run once per layer.
func zeroPayload(structure reflect.Type, open int, seen map[reflect.Type]bool) reflect.Value {
	if seen[structure] {
		// A type reached from inside itself. The probe stops rather
		// than recurring forever, and the nil pointer it leaves is a
		// null this test would then report.
		return reflect.Zero(structure)
	}
	seen[structure] = true
	defer delete(seen, structure)

	switch structure.Kind() {
	case reflect.Interface:
		// A nil list inside an any is the live hazard here, since
		// §2.7's list settings reach `cr config`'s payload exactly
		// that way. A named interface is left alone: no type is known
		// to satisfy it from here.
		if structure.NumMethod() > 0 {
			return reflect.Zero(structure)
		}
		return reflect.ValueOf([]string(nil))
	case reflect.Pointer:
		allocated := reflect.New(structure.Elem())
		allocated.Elem().Set(zeroPayload(structure.Elem(), open, seen))
		return allocated
	case reflect.Struct:
		built := reflect.New(structure).Elem()
		for i := range structure.NumField() {
			if field := structure.Field(i); field.IsExported() {
				built.Field(i).Set(zeroPayload(field.Type, open, seen))
			}
		}
		return built
	case reflect.Map:
		// The key is left zero: a JSON key is a string, and no slice
		// can become one.
		built := reflect.MakeMap(structure)
		built.SetMapIndex(reflect.Zero(structure.Key()), zeroPayload(structure.Elem(), open, seen))
		return built
	case reflect.Slice:
		if open <= 0 {
			return reflect.Zero(structure)
		}
		built := reflect.MakeSlice(structure, 1, 1)
		built.Index(0).Set(zeroPayload(structure.Elem(), open-1, seen))
		return built
	case reflect.Array:
		built := reflect.New(structure).Elem()
		for i := range structure.Len() {
			built.Index(i).Set(zeroPayload(structure.Elem(), open, seen))
		}
		return built
	default:
		return reflect.Zero(structure)
	}
}

// postingPayload stands in for the result `cr post` will hand over: a payload
// carrying §12.6's boolean and rendering it. The gate itself belongs to
// dry-run-posting and confirm-flag-required, so what is exercised here is the
// seam, which is both halves of what §12.6 asks to be distinguishable — the
// field in the document and the line in the terminal.
type postingPayload struct {
	posting
}

func (p postingPayload) Text(w *writer) string { return p.line(w) }

// §12.6 and §8.5.1: a dry run and a write that happened are told apart in JSON
// and in a terminal alike.
//
// The two shapes are asserted separately because they fail separately. A
// payload can carry the field faithfully and still render a line that reads
// the same whichever way the run went, and §12.6 names the terminal as well as
// the document.
func TestADryRunAndAConfirmedPostAreDistinguishable(t *testing.T) {
	rendered := func(mode Mode, sent bool) string {
		var printed bytes.Buffer
		out := &writer{out: &printed, mode: mode}
		require.NoError(t, out.emit(postingPayload{posting{Posted: sent}}))
		return printed.String()
	}

	assert.Contains(t, rendered(ModeJSON, false), `"posted": false`)
	assert.Contains(t, rendered(ModeJSON, true), `"posted": true`)

	dry, sent := rendered(ModeText, false), rendered(ModeText, true)
	assert.Contains(t, dry, "not posted: nothing was sent to GitHub")
	assert.Contains(t, sent, "posted: the review was sent to GitHub")
	assert.NotContains(t, sent, "not posted")
}
