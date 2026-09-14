package cli

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"reflect"
	"slices"
	"strconv"
	"strings"

	"github.com/fatih/color"
	"github.com/mattn/go-isatty"
	"github.com/spf13/cobra"
)

// Mode is the shape one run's output takes. spec/0.1.0.md §12.1 admits exactly
// two, and settle chooses between them once.
type Mode int

const (
	// ModeText is the human-readable rendering, which §12.1 gives to a
	// terminal and to nothing else.
	ModeText Mode = iota
	// ModeJSON is the machine-readable rendering. It is the default rather
	// than the exception: an agent runs cr through a pipe, so the shape it
	// parses is the one it gets without having to ask for it.
	ModeJSON
)

// result is one command's output held in both shapes at once: the value §12's
// JSON rules serialise, and the prose a terminal reads instead of it.
//
// A command names its result and hands it over. It does not choose the shape,
// encode it, or write it, which is what keeps §12.1's decision in one place
// while every command still gets to say what its own output means.
type result interface {
	// Text renders the result for a terminal. The writer comes with it so
	// a rendering can ask for colour rather than decide about it.
	Text(w *writer) string
}

// posting carries §12.6's boolean — whether this run performed the network
// write of §8 — and a command reports it by embedding this in its result.
//
// The field is opt-in rather than a default on every payload, because false is
// not a neutral value. On `cr status` a `"posted": false` would read as a
// command that weighed posting and declined, which is a claim about a decision
// nothing made. Presence is therefore a property of the command: §8.5.2
// requires `--confirm` for every network write and §8.5.3 allows nothing that
// supplies it implicitly, so the commands that could have written are exactly
// the commands that mint a confirmation, and
// TestOnlyACommandBehindTheGateReportsPosted reads both of those sets out of
// the source rather than keeping a list of either.
//
// Declaring the field in one place is the other half. A command spelling
// `posted` for itself could give it another name, another type, or an
// omitempty that hides the very dry run §8.5.1 exists to make visible.
type posting struct {
	// Posted is whether the write happened. §8.5.4 bounds what it may
	// mean: it records that cr sent the review, never that a human read
	// the draft.
	Posted bool `json:"posted"`
	// ConfirmGiven is whether `--confirm` was given, which §8.5.4 names as
	// the only fact about the gate cr can establish. It is reported beside
	// Posted rather than folded into it, because §8.5.4 has any output
	// describing the gate say that confirmation was given — and the
	// document is one of those outputs, not only the terminal line. The
	// name spells the flag rather than a past participle, which §6.2.4's
	// fence reserves for a claim cr never makes about a record.
	ConfirmGiven bool `json:"confirm_given"`
}

// line is §12.6's other half, which asks for a dry run to be distinguishable
// in a terminal and not only in the JSON document.
//
// It says whether `--confirm` was given and what became of the review, and
// stops. §8.5.4 makes the flag the only fact about the gate cr can establish
// and has every output describing the gate say so, so the sentence names the
// confirmation and the write — and neither a reading nor the draft, which
// would be offering the gate as evidence of human involvement.
func (p posting) line(w *writer) string {
	confirmation := "--confirm was not given"
	if p.ConfirmGiven {
		confirmation = "--confirm was given"
	}
	if p.Posted {
		return w.accent("posted") + ": " + confirmation + ", and the review was sent to GitHub"
	}
	return w.accent("not posted") + ": " + confirmation + ", and nothing was sent to GitHub"
}

// writer is the single place §12's output contract is applied, and it lives
// here for the reason exit.go does: §11.1 registers the output flags on the
// root command and §11.2 fixes the exit codes, so both contracts are settled
// where the command tree is and nowhere below it. A package under cli formats
// nothing and prints nothing; it returns values and errors, and this decides
// what becomes of them.
//
// One writer serves the whole tree. §12.1's decision is made on it once, by
// settle, before any RunE runs, so no command can reach a second answer even
// by accident: there is no per-command terminal check that could disagree with
// this one, and TestOnlyTheSharedWriterDecidesTheOutputShape keeps it that way.
//
// This is also where §11.1's `--quiet` exemption belongs.
// finding.HonestyDisclosure already names the shape of a report that must be
// printed whatever the flags say; the method that consumes it is this type's,
// so the exemption is a property of the one thing that writes rather than a
// check every disclosure's call site has to remember.
type writer struct {
	// out is stdout, or whatever a test pointed the command at. The mode
	// decision is made about this writer and not about os.Stdout, so what
	// the run is actually writing to is what the run is measured on.
	out io.Writer
	// mode is §12.1's answer.
	mode Mode
	// color is whether the text rendering may use colour. It is never true
	// in ModeJSON: an escape sequence inside a JSON document is a defect in
	// the document, not a decoration.
	color bool
	// compact is §11.1's `--compact`, which §12.5 spends on the JSON
	// document alone. It is read in either mode and acted on in one: the
	// flag asks for minimal JSON, and a terminal rendering already prints
	// a summary rather than a payload, so there is nothing there for it to
	// take away. It also does not change the shape — §12.1 settles that
	// from stdout and `--json`, so `--compact` through a pipe is JSON and
	// through a terminal is still text.
	compact bool
	// quiet is §11.1's `--quiet`. It is read in exactly one place,
	// informational, and disclose does not read it at all: the exemption
	// is that the method printing a disclosure has no flag it could
	// consult, rather than that every call site remembers not to.
	// TestEveryDisclosureAResultPrintsGoesThroughTheWriter holds both halves.
	quiet bool
}

// settle makes §12.1's decision from stdout and `--json`, and from nothing
// else.
//
// Text is the narrow case: output is text only when stdout is a terminal and
// `--json` was not given. There is no flag that forces text the other way —
// `--no-color` strips colour and leaves the shape alone — so a command writing
// to a pipe under `--no-color` still emits JSON.
func (w *writer) settle(cmd *cobra.Command) error {
	forceJSON, err := cmd.Flags().GetBool("json")
	if err != nil {
		return err
	}
	noColor, err := cmd.Flags().GetBool("no-color")
	if err != nil {
		return err
	}
	compact, err := cmd.Flags().GetBool("compact")
	if err != nil {
		return err
	}
	quiet, err := cmd.Flags().GetBool("quiet")
	if err != nil {
		return err
	}

	w.out = cmd.OutOrStdout()
	w.compact, w.quiet = compact, quiet
	if forceJSON || !isTerminal(w.out) {
		w.mode, w.color = ModeJSON, false
		return nil
	}
	w.mode, w.color = ModeText, !noColor
	return nil
}

// emit writes one command's result in the shape settle chose, pretty-printed
// with §12.2's two-space indentation and carrying §12.3's empty arrays.
func (w *writer) emit(r result) error {
	if w.mode == ModeJSON {
		payload, err := w.document(r)
		if err != nil {
			return err
		}
		encoded, err := json.MarshalIndent(payload, "", "  ")
		if err != nil {
			return err
		}
		_, err = fmt.Fprintln(w.out, string(encoded))
		return err
	}
	_, err := fmt.Fprintln(w.out, r.Text(w))
	return err
}

// document is the value emit encodes: §12.3's filled slices always, and
// §12.5's omissions on top of them under `--compact`.
//
// The two compose rather than one replacing the other, and in this order.
// §12.3 is a rule about what a present field becomes, §12.5 about which fields
// are present at all, so the value is filled first and the document it encodes
// to is pruned after. §12.2 is unaffected either way: a compacted document is
// still pretty-printed, because §11.1 asks `--compact` for minimal JSON and not
// for JSON a person cannot read.
func (w *writer) document(r result) (any, error) {
	filled := withoutNilSlices(r)
	if !w.compact {
		return filled, nil
	}
	return withoutBulk(filled)
}

// foundingFields are the two fields §12.5 forbids `--compact` to omit.
//
// They are not spared for being small. §8.1.2 has the agent compose every
// posted body out of `evidence` and `citations`, so a payload handed over
// without them is a payload the composition has nothing to rest on — and the
// agent would have no way to know it was reading a compacted document rather
// than a record that never had them. The bulk below is re-derivable from the
// state tree; a body composed out of nothing is not recoverable at all.
var foundingFields = []string{"evidence", "citations"}

// compactOmits are the fields `--compact` drops, named by their JSON key
// rather than by the Go type that prints them.
//
// The wire name is what §12.5 states, and it is the only thing the three have
// in common: `output_tail` belongs to §5.5's probe record and §5.2.4's run
// record alike, `input` to the probe record, and a body to an ingested thread
// from internal/gh. Keying on a type would answer for whichever of them
// happened to be written first and stay silent about the rest.
//
// `output_tail` and `input` are named bare, since no other object in the
// contract carries either. A thread body is named under the key it sits behind,
// because `body` on its own is not one field everywhere: §8.1 gives a drafted
// record a body too, and that one is the whole of what `cr draft` is for.
var compactOmits = omittedFields(
	"output_tail",
	"input",
	"comment.body",
	"replies.body",
)

// omittedFields builds compactOmits' set, and refuses a founding field while
// it does.
//
// The refusal is the mechanism §12.5's second sentence needs. Getting the
// table right today leaves `evidence` one plausible edit away — it is prose, it
// is often the longest string in a record, and it is exactly what someone
// trimming a payload would reach for next — and a rule written only in a
// comment is a rule that edit reads past. This one aborts the package before a
// command can run, so `--compact` is incapable of naming the two fields rather
// than merely coded not to today.
func omittedFields(names ...string) map[string]bool {
	omitted := make(map[string]bool, len(names))
	for _, name := range names {
		// The field is the last segment, so qualifying a name by the
		// key it sits behind is no way around the refusal.
		field := name[strings.LastIndex(name, ".")+1:]
		if slices.Contains(foundingFields, field) {
			panic("§12.5: --compact may not omit " + field +
				"; the agent composes every posted body from it")
		}
		omitted[name] = true
	}
	return omitted
}

// withoutBulk returns payload with §12.5's fields gone.
//
// The pruning is done on the encoded document and not on the Go value, which
// is what lets it answer for a payload nobody has written yet: reflection
// would have to be told which struct fields to skip, and §5.5's probe record
// has no Go type in this repository at all. A document has the field names,
// and the field names are what §12.5 fixes.
//
// It costs the field order: a document rebuilt from maps comes out with its
// keys sorted, where an uncompacted one follows the struct. §12 fixes the
// shape, the indentation, the nulls and the omissions and says nothing about
// the order, and no reader of JSON may depend on it — so this is the whole of
// the difference `--compact` makes beyond the fields it removes.
func withoutBulk(payload any) (any, error) {
	document, err := json.Marshal(payload)
	if err != nil {
		return nil, err
	}

	decoder := json.NewDecoder(bytes.NewReader(document))
	// Numbers are kept exactly as they were written. Decoding them into
	// float64 and encoding them back would round anything past 2^53,
	// which is a corruption of the payload and not a compaction of it.
	decoder.UseNumber()

	var parsed any
	if err := decoder.Decode(&parsed); err != nil {
		return nil, err
	}
	return pruned(parsed, ""), nil
}

// pruned drops every omitted field from node, where under is the key node
// itself arrived behind.
//
// An array hands its own key down to its elements, so a reply's body is judged
// under `replies` rather than under a position that would differ for every
// reply in the thread.
func pruned(node any, under string) any {
	switch value := node.(type) {
	case map[string]any:
		kept := make(map[string]any, len(value))
		for name, field := range value {
			if compactOmits[name] || compactOmits[under+"."+name] {
				continue
			}
			kept[name] = pruned(field, name)
		}
		return kept
	case []any:
		for i, element := range value {
			value[i] = pruned(element, under)
		}
		return value
	default:
		return node
	}
}

// jsonMarshaler is the interface a type uses to take its own serialisation
// over, and the one thing withoutNilSlices leaves alone.
var jsonMarshaler = reflect.TypeFor[json.Marshaler]()

// withoutNilSlices returns payload with every nil slice it can reach replaced
// by an empty one, which is where §12.3 is kept.
//
// The rule could be left to the construction site instead — make([]T, 0)
// rather than var x []T, which is the convention here and stays the convention
// — but a constructor binds only the payloads somebody has already written. It
// says nothing about the next one, and nothing about a field set back to nil
// after it was built. Every JSON document cr prints passes through this one
// function, so the guarantee holds for a payload nobody constructed at all.
//
// The copy is deep, so nothing the caller still holds is rewritten, and it
// takes a payload for a tree: cr has no output struct that points back at
// itself.
func withoutNilSlices(payload any) any {
	value := reflect.ValueOf(payload)
	if !value.IsValid() {
		return payload
	}
	return filledSlices(value).Interface()
}

// filledSlices is withoutNilSlices' recursion, which follows a nil slice
// wherever one can hide: behind a pointer, inside an any, in a map's value, in
// an element of another slice, or in an embedded struct. A payload marshalled
// at its outermost layer alone would answer for none of those.
func filledSlices(value reflect.Value) reflect.Value {
	// A type that marshals itself decides what its own nulls mean, and
	// json.RawMessage is why that has to be honoured: an empty raw message
	// is not an empty array, it is a document holding nothing, and it
	// fails to encode.
	if value.Type().Implements(jsonMarshaler) {
		return value
	}
	switch value.Kind() {
	case reflect.Interface:
		// The concrete value is assignable everywhere the interface
		// was, so it is handed back in the interface's place.
		if value.IsNil() {
			return value
		}
		return filledSlices(value.Elem())
	case reflect.Pointer:
		if value.IsNil() {
			return value
		}
		copied := reflect.New(value.Type().Elem())
		copied.Elem().Set(filledSlices(value.Elem()))
		return copied
	case reflect.Struct:
		// The whole struct is copied first, which carries the
		// unexported fields across — reflection may read those and
		// never write them, and encoding/json does not look at them
		// either. The rest is then rewritten in place.
		copied := reflect.New(value.Type()).Elem()
		copied.Set(value)
		filledFields(copied)
		return copied
	case reflect.Map:
		if value.IsNil() {
			return value
		}
		copied := reflect.MakeMapWithSize(value.Type(), value.Len())
		for entry := value.MapRange(); entry.Next(); {
			copied.SetMapIndex(entry.Key(), filledSlices(entry.Value()))
		}
		return copied
	case reflect.Slice:
		// A nil slice and an empty one differ here and nowhere else:
		// MakeSlice never returns nil, so the length carries over and
		// the null does not.
		copied := reflect.MakeSlice(value.Type(), value.Len(), value.Len())
		for i := range value.Len() {
			copied.Index(i).Set(filledSlices(value.Index(i)))
		}
		return copied
	case reflect.Array:
		copied := reflect.New(value.Type()).Elem()
		for i := range value.Len() {
			copied.Index(i).Set(filledSlices(value.Index(i)))
		}
		return copied
	default:
		return value
	}
}

// filledFields rewrites a struct's own fields where they sit, which an
// embedded unexported type is what forces: encoding/json promotes the exported
// fields inside one into the document, and reflection will not hand the
// embedding itself over as a value — though it does allow the fields within it
// to be set. An embedding through a pointer stays out of reach, since filling
// it would mean writing through the caller's own pointer, and the guard test
// refuses one rather than let this quietly miss it.
func filledFields(target reflect.Value) {
	for i := range target.NumField() {
		field := target.Type().Field(i)
		if field.IsExported() {
			target.Field(i).Set(filledSlices(target.Field(i)))
		} else if field.Anonymous && field.Type.Kind() == reflect.Struct {
			filledFields(target.Field(i))
		}
	}
}

// informational is where an informational message goes: out, or nowhere under
// `--quiet`.
//
// §11.1 gives the flag informational messages and nothing else. The one kind cr
// writes is a test run's live output — `cr test` and `cr probe run` stream the
// suite to standard error while it runs, so a long run is distinguishable from a
// hang — and it is informational in the plain sense: §5.2.4 stores the tail
// either way, and the result reports the counts either way. What is discarded is
// the echo to the person watching, and only that; callers tee the stored tail
// and the counter off the same stream beside this writer, not through it.
func (w *writer) informational(out io.Writer) io.Writer {
	if w.quiet {
		return io.Discard
	}
	return out
}

// disclose renders §11.1's honesty disclosures for a terminal: each sentence
// between before and after, in order.
//
// It is the exemption. It takes no flag and reads none, so a disclosure routed
// through it is printed under `--quiet` because nothing here could stop it,
// not because a call site checked. Every result's Text that prints a
// disclosure comes through this method, and
// TestEveryDisclosureAResultPrintsGoesThroughTheWriter reads the source to keep
// it that way: a Text ranging over its own Honesty, or printing a Disclosure()
// directly, is a call site that could be taught to consult the flag.
func (w *writer) disclose(before, after string, sentences ...string) string {
	var out strings.Builder
	for _, sentence := range sentences {
		out.WriteString(before + sentence + after)
	}
	return out.String()
}

// accent renders s in the colour a terminal rendering highlights with, and
// returns it untouched when colour is off.
//
// The colour is enabled on the instance rather than through the package's
// global switch, which fatih/color sets from os.Stdout at init. A run whose
// output goes somewhere other than os.Stdout — every test here, and a command
// whose stdout was redirected — would otherwise be coloured by a decision made
// about a file it never writes to.
func (w *writer) accent(s string) string {
	if !w.color {
		return s
	}
	accent := color.New(color.FgCyan)
	accent.EnableColor()
	return accent.Sprint(s)
}

// failure is §12.4's error document: what refused, and the next actionable
// step. Both are strings, so there is no slice here for §12.3 to watch.
type failure struct {
	// Error is the refusal as the error states it.
	Error string `json:"error"`
	// Hint is §12.4's next actionable step, from hintFor.
	Hint string `json:"hint"`
}

// reportFailure writes one failed run's error to stderr in the shape §12.1
// gives the run: a JSON document carrying `error` and `hint` when stdout is not
// a terminal or `--json` was given, and two prose lines otherwise.
//
// The shape is decided here, from stdout and the arguments, rather than read
// off the writer settle filled in, because settle runs in PersistentPreRunE and
// a usage error — an unknown flag, a missing argument — refuses before it. A
// failure reported in text through a pipe would be the one output of the run an
// agent could not parse, and it is the one output that says what to do next.
//
// It goes to stderr in both shapes. A failed run has no result, and stdout is
// where a command's result goes; the exit code is what tells the caller which
// of the two to read.
func reportFailure(stdout, stderr io.Writer, args []string, err error) error {
	reported := failure{Error: err.Error(), Hint: hintFor(err)}
	if !asksForJSON(args) && isTerminal(stdout) {
		_, werr := fmt.Fprintf(stderr, "error: %s\nhint: %s\n", reported.Error, reported.Hint)
		return werr
	}
	encoded, merr := json.MarshalIndent(reported, "", "  ")
	if merr != nil {
		return merr
	}
	_, werr := fmt.Fprintln(stderr, string(encoded))
	return werr
}

// asksForJSON reports whether the arguments set §11.1's `--json`, as the
// command they name reads it: the tree resolves the command and that command's
// own flag set parses the rest, so a string flag that takes a literal `--json`
// as its value has not asked for anything. Only arguments that parse fails on
// fall back to scanning.
//
// The fresh tree is first prepared as cobra's ExecuteC prepared the one the run
// parsed, in its order: the help and completion commands on the root, then the
// help and version flags on the command found. Without them a line the run
// parsed, such as one carrying `--help=false`, fails to parse here and the scan
// answers instead. ExecuteC's hidden `__complete` command is not added: cobra
// adds it only when the arguments name it, and it parses no flags.
func asksForJSON(args []string) bool {
	root := newRootCmd()
	root.InitDefaultHelpCmd()
	root.InitDefaultCompletionCmd(args...)
	cmd, flags, err := root.Find(args)
	if err != nil {
		return scansForJSON(args)
	}
	cmd.InitDefaultHelpFlag()
	cmd.InitDefaultVersionFlag()
	if cmd.ParseFlags(flags) != nil {
		return scansForJSON(args)
	}
	// Every command inherits the root's `--json`, so the lookup has nothing
	// to refuse.
	asked, _ := cmd.Flags().GetBool("json")
	return asked
}

// scansForJSON reads `--json` off arguments no command's flag set could parse,
// the way pflag reads a boolean flag: bare, or with any value strconv.ParseBool
// accepts, the last occurrence deciding, and never after the `--` that ends
// flag parsing. A value ParseBool refuses is where pflag stops with a usage
// error, so the answer is the one the occurrences before it left.
func scansForJSON(args []string) bool {
	asked := false
	for _, arg := range args {
		if arg == "--" {
			return asked
		}
		if arg == "--json" {
			asked = true
			continue
		}
		value, ok := strings.CutPrefix(arg, "--json=")
		if !ok {
			continue
		}
		parsed, err := strconv.ParseBool(value)
		if err != nil {
			return asked
		}
		asked = parsed
	}
	return asked
}

// isTerminal reports whether out is a terminal.
//
// The question is asked of the io.Writer the command will actually write to,
// so there is no flag or field to inject: anything that is not an open file is
// not a terminal, and a file is one only if the kernel says so. A character
// device is not enough on its own — /dev/null and the master side of a
// pseudo-terminal are both character devices and neither is a terminal — so
// the terminal ioctl answers rather than the file mode.
func isTerminal(out io.Writer) bool {
	file, ok := out.(*os.File)
	return ok && isatty.IsTerminal(file.Fd())
}
