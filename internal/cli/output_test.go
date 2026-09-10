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
	"strconv"
	"strings"
	"testing"

	"github.com/creack/pty"

	"github.com/deligoez/cr/internal/finding"
	"github.com/deligoez/cr/internal/gh"
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

// §11.1 has `--quiet` suppress informational messages, and a command's own
// result is not one of them.
//
// Both shapes are asserted because they fail differently. A flag reaching into
// the JSON takes a field out of the document an agent parses; one reaching into
// the text leaves a person's terminal empty. §12.1 settles the shape from
// stdout and `--json` alone, and no output flag empties a command's answer.
func TestQuietLeavesACommandsResultAlone(t *testing.T) {
	crHome(t)

	assert.Equal(t, throughAPipe(t, "config"), throughAPipe(t, "config", "--quiet"),
		"§11.1: --quiet took something out of the document cr config printed")

	terminal := throughATerminal(t, "config", "--quiet")
	assert.Contains(t, terminal, "\x1b[36mpost.max_comments\x1b[0m = 20")
}

// quietFlag is §11.1's flag name. pflag offers no way to reach a registered
// flag but by naming it, so the literal is what a walk over the source can look
// for.
const quietFlag = "quiet"

// disclosureContract is the interface a report implements to become one of the
// seven §11.1 exempts from `--quiet`.
const disclosureContract = "HonestyDisclosure"

// namesTheQuietFlag reports whether call passes the flag's name as a literal.
func namesTheQuietFlag(call *ast.CallExpr) bool {
	for _, arg := range call.Args {
		literal, ok := arg.(*ast.BasicLit)
		if !ok || literal.Kind != token.STRING {
			continue
		}
		if name, err := strconv.Unquote(literal.Value); err == nil && name == quietFlag {
			return true
		}
	}
	return false
}

// registersTheFlag reports whether call declares a flag rather than reads one:
// `PersistentFlags().Bool(...)`, which is the whole of what §11.1's table costs
// and the one mention of the name that suppresses nothing.
func registersTheFlag(call *ast.CallExpr) bool {
	declared, ok := call.Fun.(*ast.SelectorExpr)
	if !ok || declared.Sel.Name != "Bool" {
		return false
	}
	on, ok := declared.X.(*ast.CallExpr)
	if !ok {
		return false
	}
	flags, ok := on.Fun.(*ast.SelectorExpr)
	return ok && flags.Sel.Name == "PersistentFlags"
}

// namesTheDisclosureContract reports whether any of shipped under the given
// directory names disclosureContract in code.
//
// A comment does not count, and that is the point. internal/cli/output.go has
// said since output-mode-switching that the exemption belongs to the writer,
// and saying so is not having it.
func namesTheDisclosureContract(t *testing.T, root string, shipped []string, under string) bool {
	t.Helper()
	found := false
	for _, rel := range shipped {
		if !strings.HasPrefix(rel, under+string(filepath.Separator)) {
			continue
		}
		parsed, err := parser.ParseFile(token.NewFileSet(), filepath.Join(root, rel), nil, 0)
		require.NoError(t, err)
		ast.Inspect(parsed, func(node ast.Node) bool {
			if ident, isIdent := node.(*ast.Ident); isIdent && ident.Name == disclosureContract {
				found = true
			}
			return !found
		})
	}
	return found
}

// §11.1 gives `--quiet` a suppression and then seven reports it may never
// suppress, and cr today has the second half and not the first: the flag is
// registered, nothing reads it, and nothing is suppressed.
//
// This is what keeps that from being undone by halves. The failure §11.1 exists
// to prevent is a suppression shipped ahead of its exemption — a `--quiet` that
// silences the lenses of §4.5.4, the probe cap of §5.6.4, the forcing counts of
// §6.3.2, the waiver and duplicate counts of §10.1.6, the sandbox notice of
// §5.1.6, the stale round of §9.3.2, or the comment cap of §1.6.2 — and that
// failure has a shape a walk can see: a file that reads the flag while no
// writer under internal/cli consumes finding.HonestyDisclosure. The first is
// the suppression, the second is the exemption, and the first without the
// second fails here.
//
// It is coarse deliberately. It cannot tell a correct routing from a wrong one;
// what it can do is make the exemption impossible to postpone, which is the
// half that gets left for later. quiet-honesty-exemptions is the task that
// supplies it, and the day it does, this test goes quiet and stays true.
func TestNoSuppressionUnderQuietArrivesBeforeItsExemption(t *testing.T) {
	root := moduleRoot(t)

	shipped := make([]string, 0)
	crSource(t, func(rel string, _ []string) { shipped = append(shipped, rel) })

	registrations := 0
	suppressors := make([]string, 0)
	for _, rel := range shipped {
		parsed, err := parser.ParseFile(token.NewFileSet(), filepath.Join(root, rel), nil, 0)
		require.NoError(t, err)
		ast.Inspect(parsed, func(node ast.Node) bool {
			call, isCall := node.(*ast.CallExpr)
			if !isCall || !namesTheQuietFlag(call) {
				return true
			}
			if registersTheFlag(call) {
				registrations++
				return true
			}
			suppressors = append(suppressors, rel)
			return true
		})
	}
	require.Equal(t, 1, registrations,
		"§11.1 registers --quiet once, and a walk that cannot see it proves nothing")

	// The exemption is looked for where it is declared as well, so a false
	// below is an absence in internal/cli and not a walk that reads no Go.
	require.True(t, namesTheDisclosureContract(t, root, shipped, filepath.Join("internal", "finding")),
		"the walk misses %s where it is declared", disclosureContract)
	exempt := namesTheDisclosureContract(t, root, shipped, filepath.Join("internal", "cli"))

	assert.True(t, len(suppressors) == 0 || exempt,
		"§11.1: %v read --quiet while no writer under internal/cli consumes finding.%s, "+
			"so the seven disclosures go out with the informational messages",
		suppressors, disclosureContract)
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
var outputStructs = []result{
	initResult{}, configResult(nil), &noteResult{}, &retractResult{}, &answerResult{}, &contextResult{},
	&recordResult{}, &claimsRecordResult{}, &briefResult{}, &cellsRecordResult{},
	&rulesCheckResult{},
	&mapRecordResult{}, &sandboxCreateResult{}, &sandboxDestroyResult{}, &testRunResult{},
	&probeRunResult{}, &draftResult{}, &reviewResult{},
}

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

// gatedCommands names each file under internal/cli against whether it mints
// §8.5's confirmation.
//
// The mint is the gate, and the gate is the whole of what could have written:
// §8.5.2 requires `--confirm` for every network write, §8.5.3 forbids anything
// that supplies it implicitly, and a confirmation's only field is unexported,
// so a file naming neither of mints holds nothing but the zero token and
// writes nothing with it. TestNothingOutsideTheGhPackageMintsAConfirmation
// keeps that set to one deliberate widening.
//
// mints is that guard's own list, read here rather than restated, and the set
// is derived from the source on every run rather than written down. So §12.6's
// commands and the write boundary's commands are one set and have no way to
// come apart.
func gatedCommands(t *testing.T) map[string]bool {
	t.Helper()
	sources, err := os.ReadDir(".")
	require.NoError(t, err)

	gated := map[string]bool{}
	for _, source := range sources {
		name := source.Name()
		if !strings.HasSuffix(name, ".go") || strings.HasSuffix(name, "_test.go") {
			continue
		}
		raw, err := os.ReadFile(name)
		require.NoError(t, err)
		gated[name] = slices.ContainsFunc(mints, func(mint string) bool {
			return strings.Contains(string(raw), mint)
		})
	}

	require.Greater(t, len(gated), 3,
		"only %d files were scanned, so this guard proved nothing", len(gated))
	return gated
}

// printsPosted reports whether a payload's own JSON document carries §12.6's
// field. The document is read rather than the type, so a command that spells
// the field for itself is answered for exactly as one that embeds posting.
func printsPosted(t *testing.T, payload result) bool {
	t.Helper()
	encoded, err := json.Marshal(withoutNilSlices(payload))
	require.NoError(t, err)

	document := map[string]json.RawMessage{}
	require.NoError(t, json.Unmarshal(encoded, &document))
	_, carried := document["posted"]
	return carried
}

// §12.6: a command reports `posted` exactly when it could have performed a
// network write.
//
// The negative half is the one that is easy to get wrong. A `"posted": false`
// on `cr status` is not a harmless default — it says the command weighed
// posting and did not post, which is a claim about a decision nothing made. So
// presence is a property of the command, and both sides of the equality are
// read out of the source: the field from the document the payload prints, the
// permission from the file that mints §8.5's confirmation.
//
// cr post is unwritten, so nothing mints one today and the rule bites in the
// negative direction alone: no payload carries the field. The positive
// direction arrives with the gate, and in the same commit, because the file
// that gains the mint is the file this then requires the field of.
func TestOnlyACommandBehindTheGateReportsPosted(t *testing.T) {
	gated := gatedCommands(t)
	declared := emittablePayloads(t)

	for _, payload := range outputStructs {
		name := payloadName(payload)
		t.Run(name, func(t *testing.T) {
			file, found := declared[name]
			require.True(t, found, "no file under internal/cli declares %s's Text method", name)

			assert.Equal(t, gated[file], printsPosted(t, payload),
				"§12.6: %s reports posted if and only if %s could have performed a network write",
				name, file)
		})
	}
}

// §12.6's field is declared once, in the writer, and nowhere else.
//
// The equality above holds between a command's permission and its document,
// and a payload spelling the field itself would satisfy it while giving
// `posted` another meaning, another type, or an omitempty that drops the false
// §8.5.1 requires a dry run to print. Embedding posting is the only way to
// carry it, and this is what makes that so.
func TestThePostedFieldIsDeclaredInOnePlace(t *testing.T) {
	sources, err := os.ReadDir(".")
	require.NoError(t, err)

	scanned := 0
	var found []string
	for _, source := range sources {
		name := source.Name()
		if !strings.HasSuffix(name, ".go") || strings.HasSuffix(name, "_test.go") || name == "output.go" {
			continue
		}
		raw, err := os.ReadFile(name)
		require.NoError(t, err)
		scanned++
		if strings.Contains(string(raw), `json:"posted"`) {
			found = append(found, name)
		}
	}

	require.Greater(t, scanned, 3,
		"only %d files were scanned, so this guard proved nothing", scanned)
	assert.Empty(t, found,
		"§12.6: posting declares the field, so a payload carries it by embedding posting")
}

// The five fields §12.5 decides about, each given a value nothing else in the
// payload spells, so an assertion about the document is an assertion about
// that field and not about a word that happens to appear twice.
const (
	probeInput   = "@@ -1 +1 @@ -if ready { +if !ready {"
	probeTail    = "ok  github.com/example/pkg  0.412s"
	openingBody  = "this looks like it drops the last element"
	replyBody    = "it does; the loop bound is wrong"
	recordReason = "the head-side range excludes the final line"
	citedPath    = "internal/example/loop.go"
)

// bulkyProbe stands in for §5.5's probe record, which has no Go type in this
// repository yet — probe-record-schema is a later task. Its two fields are
// declared under the names §5.5's table gives them, which is the whole of what
// §12.5 keys on: the rule is stated over field names, so a stand-in carrying
// the right names is the same payload to it as the record will be.
type bulkyProbe struct {
	ID         string `json:"id"`
	Kind       string `json:"kind"`
	Input      string `json:"input"`
	Result     string `json:"result"`
	OutputTail string `json:"output_tail"`
}

// bulkyPayload carries all five of §12.5's fields at once.
//
// Three of them are the real types' own. §6.1's record is finding.Finding and
// §3.5.1's ingestion lands in gh.Thread, so `evidence`, `citations` and the two
// bodies are named here by the types that will print them rather than by names
// this test chose for itself, and a rename in either is a failure here.
type bulkyPayload struct {
	Probe    bulkyProbe        `json:"probe"`
	Findings []finding.Finding `json:"findings"`
	Threads  []gh.Thread       `json:"threads"`
}

func (bulkyPayload) Text(*writer) string { return "a payload for §12's JSON rules" }

// bulky builds that payload: one probe record, one finding, and one ingested
// thread with a reply, which is every place §12.5 names.
func bulky() bulkyPayload {
	comment := func(id, body string) gh.Comment {
		return gh.Comment{
			ID:             id,
			Author:         "some-author",
			AuthorTypename: "User",
			Body:           body,
			CreatedAt:      "2026-08-30T09:00:00Z",
			URL:            "https://example.invalid/" + id,
		}
	}
	return bulkyPayload{
		Probe: bulkyProbe{
			ID:         "p1",
			Kind:       "mutation",
			Input:      probeInput,
			Result:     "no-test-failed",
			OutputTail: probeTail,
		},
		Findings: []finding.Finding{{
			ID:        "f1",
			Kind:      finding.KindFinding,
			Severity:  finding.SeverityMedium,
			Summary:   "the loop drops its last element",
			Evidence:  recordReason,
			Citations: []finding.Citation{{Path: citedPath, Line: 42}},
		}},
		Threads: []gh.Thread{{
			ID:         "t1",
			Comment:    comment("c1", openingBody),
			AuthorType: gh.AuthorHuman,
			Replies:    []gh.Comment{comment("c2", replyBody)},
		}},
	}
}

// emitted prints a payload through the real writer, with the flags settled off
// the real root command.
//
// The flags matter more than the writer does. A test that set w.compact itself
// would pass whatever `--compact` is called in root.go, or whether settle reads
// it at all; parsing against newRootCmd()'s own flag set means the registration
// and the reading are both on the path being measured.
func emitted(t *testing.T, payload result, args ...string) string {
	t.Helper()
	var printed bytes.Buffer
	root := newRootCmd()
	root.SetOut(&printed)
	require.NoError(t, root.ParseFlags(args))

	out := &writer{}
	require.NoError(t, out.settle(root))
	require.Equal(t, ModeJSON, out.mode, "a buffer is not a terminal, so §12.1 gives it JSON")
	require.NoError(t, out.emit(payload))
	return printed.String()
}

// §12.5: `--compact` omits `output_tail`, `input`, and ingested thread bodies,
// and it leaves `evidence` and `citations` exactly where they were.
//
// One payload carries all five, because the rule is a division and not two
// lists: a compaction that stripped everything bulky would satisfy a test that
// only looked at what went, and one that stripped nothing would satisfy a test
// that only looked at what stayed. The same payload is printed without the flag
// first, so each absence below is the flag's doing rather than a field the
// payload never had.
func TestCompactDropsTheBulkAndKeepsWhatAPostedBodyRestsOn(t *testing.T) {
	full := emitted(t, bulky())
	compact := emitted(t, bulky(), "--compact")

	for _, gone := range []string{`"output_tail"`, probeTail, `"input"`, probeInput, openingBody, replyBody} {
		assert.Contains(t, full, gone, "the payload has to carry %q for its absence to mean anything", gone)
		assert.NotContains(t, compact, gone, "§12.5: --compact left %q in the document", gone)
	}

	for _, kept := range []string{`"evidence"`, recordReason, `"citations"`, citedPath} {
		assert.Contains(t, compact, kept,
			"§12.5: the agent composes every posted body from %q, so --compact may not drop it", kept)
	}

	// A thread loses its bodies and not itself: §3.5.4 suppresses a finding
	// by naming the thread that already covers it, which needs the id and
	// the anchor that a wholesale removal would take with them.
	assert.Contains(t, compact, `"id": "t1"`)
	assert.Contains(t, compact, `"author": "some-author"`)

	// §12.2 does not except a compacted document.
	assert.True(t, json.Valid([]byte(compact)), "--compact printed %q", compact)
	assert.Contains(t, compact, "\n  \"findings\": [")
}

// §12.5's second sentence is a rule about what the omission table may never
// say, so it is enforced where the table is built rather than beside it.
//
// omittedFields refuses the name outright, which is what makes `--compact`
// incapable of reaching `evidence` and `citations` rather than merely coded not
// to today: a table naming one of them does not fail this test, it fails to
// start — every command in the tree aborts before it runs, and so does every
// other test in the package. This checks that the refusal is real, and that a
// qualified name is no way round it, since narrowing an omission to one key is
// exactly the shape a later edit would reach for to make it look local.
func TestTheOmissionTableRefusesAFoundingField(t *testing.T) {
	for name, field := range map[string]string{
		"evidence":          "evidence",
		"citations":         "citations",
		"record.evidence":   "evidence",
		"finding.citations": "citations",
	} {
		t.Run(name, func(t *testing.T) {
			assert.PanicsWithValue(t,
				"§12.5: --compact may not omit "+field+"; the agent composes every posted body from it",
				func() { omittedFields(name) },
				"§12.5: the omissions accepted %q", name)
		})
	}

	assert.NotPanics(t, func() { omittedFields("output_tail", "comment.body") },
		"§12.5 refuses two fields, not the table they are kept out of")
	require.NotEmpty(t, compactOmits, "the refusal is worth nothing if the table it guards is empty")
}

// jsonName is the key a field prints under, with the options after it dropped.
// It takes the tag rather than the field, which is a hundred bytes of reflect
// metadata this has no use for.
func jsonName(tag reflect.StructTag) string {
	name, _, _ := strings.Cut(tag.Get("json"), ",")
	return name
}

// §12.5's "ingested thread bodies" are §3.5.1's, and gh.Thread is where that
// ingestion lands.
//
// The omission is written as a key and a field name, so it is worth exactly as
// much as those two remaining what gh prints. Both are read out of the type
// here rather than restated: a thread that grew a third place to keep a comment
// — a quoted body, a review summary — would fail here rather than post one
// under `--compact`, and so would a renamed tag.
func TestEveryPlaceAnIngestedThreadKeepsABodyIsOmitted(t *testing.T) {
	comment := reflect.TypeFor[gh.Comment]()
	body, carried := comment.FieldByName("Body")
	require.True(t, carried, "§3.5.1 ingests a thread's body, and gh.Comment is what holds it")
	require.Equal(t, "body", jsonName(body.Tag), "the omission names the key gh.Comment prints under")

	thread := reflect.TypeFor[gh.Thread]()
	holders := make([]string, 0, 2)
	for i := range thread.NumField() {
		field := thread.Field(i)
		held := field.Type
		if held.Kind() == reflect.Slice {
			held = held.Elem()
		}
		if held != comment {
			continue
		}
		under := jsonName(field.Tag)
		holders = append(holders, under)
		assert.True(t, compactOmits[under+"."+jsonName(body.Tag)],
			"§12.5: a thread keeps a body under %q, and --compact does not omit it", under)
	}

	require.ElementsMatch(t, []string{"comment", "replies"}, holders,
		"a thread's comments are the whole of where its bodies are; a walk finding none proves nothing")
}
