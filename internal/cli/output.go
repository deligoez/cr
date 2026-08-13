package cli

import (
	"encoding/json"
	"fmt"
	"io"
	"os"

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

	w.out = cmd.OutOrStdout()
	if forceJSON || !isTerminal(w.out) {
		w.mode, w.color = ModeJSON, false
		return nil
	}
	w.mode, w.color = ModeText, !noColor
	return nil
}

// emit writes one command's result in the shape settle chose.
func (w *writer) emit(r result) error {
	if w.mode == ModeJSON {
		encoded, err := json.MarshalIndent(r, "", "  ")
		if err != nil {
			return err
		}
		_, err = fmt.Fprintln(w.out, string(encoded))
		return err
	}
	_, err := fmt.Fprintln(w.out, r.Text(w))
	return err
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
