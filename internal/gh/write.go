package gh

import (
	"fmt"
	"slices"
	"strings"
	"unicode"
)

// The network-write boundary of §2.1.2.
//
// §2.1.2 permits cr no network write except the GitHub calls of §8, and those
// only behind the confirmation gate of §8.5. That is enforced here rather than
// stated. gh is the only binary cr reaches GitHub through, every invocation of
// it is built in this package, and the two doors out of this package are
// deliberately asymmetric.
//
// Run is the read door and it is closed by default: an invocation is refused
// unless the boundary recognises it as a read. A write therefore reaches
// nothing by being new, by being spelled differently, or by arriving in a
// release of gh nobody here has seen.
//
// Write is the other door, and it is a method on Confirmation rather than a
// function taking one, so there is no call shape that omits the token.
// Confirmation's only field is unexported and Confirm is its only constructor,
// so no package outside this one can build a valid token at all: a
// Confirmation composed elsewhere is the zero value and writes nothing. The
// precedent is state.Lock, which makes a per-PR write unreachable without
// holding it, and state.Stamp, whose unexported setter makes Stamped
// unimplementable outside its package.

// WriteRefusedError reports an invocation the boundary would not run.
//
// It is deliberately not a CommandError. gh never started, so there is no exit
// status and no stderr to surface, and §3.1.3's shape describes an external
// command that ran and refused. Reporting this as one would name a failure
// that did not happen and send the reader to gh's output for a decision cr
// made.
type WriteRefusedError struct {
	// Args are the arguments the caller offered, the binary name aside, so
	// the refusal names the invocation that was actually attempted.
	Args []string
	// Reason names what the boundary objected to, so the caller learns
	// which part of the invocation it read as a write.
	Reason string
}

func (e *WriteRefusedError) Error() string {
	return fmt.Sprintf(
		"gh %s: %s: §2.1.2 allows a network write only through the confirmation gate of §8.5",
		strings.Join(e.Args, " "), e.Reason,
	)
}

// Confirmation is the token §8.5's gate mints once --confirm has been given.
//
// It carries no payload and is not evidence of anything beyond the flag:
// §8.5.4 forbids cr to claim a human read the draft, and the only fact
// available here is that --confirm was passed.
type Confirmation struct {
	granted bool
}

// Confirm mints the token from the value of --confirm.
//
// It takes the gate's decision rather than making it. §8.5.3 forbids any
// setting, environment variable, profile field, or alias that supplies
// --confirm implicitly, so the flag is the only input this may ever have, and
// the single caller is the gate in cr post.
func Confirm(given bool) Confirmation { return Confirmation{granted: given} }

// Write runs one network write and returns gh's standard output.
func (c Confirmation) Write(args ...string) (string, error) {
	if !c.granted {
		return "", &WriteRefusedError{
			Args:   slices.Clone(args),
			Reason: "no confirmation was presented",
		}
	}
	// §8's write is an API call and nothing else. §8.3.1 requires every
	// comment of a round to arrive as one review, and §8.4.1 requires that
	// review to be created atomically; `gh pr comment` posts a separate
	// issue comment, and `gh pr review` carries no line positions. Neither
	// can express the call §8 describes, so neither is a write cr has.
	if len(args) == 0 || args[0] != "api" {
		return "", &WriteRefusedError{
			Args:   slices.Clone(args),
			Reason: "§8's only write is an API call",
		}
	}
	return invoke(args...)
}

// valueArgs are the `gh api` flags whose value is the next argument when it is
// not attached with `=` or written against a shorthand.
//
// The list is what lets the scan tell an endpoint from a flag's value. A flag
// it does not know has its value read as an endpoint instead, which is a
// mistake in the safe direction: an endpoint the boundary does not recognise
// as graphql is judged by the stricter rule below, not the looser one.
var valueArgs = map[string]bool{
	"--cache":     true,
	"--field":     true,
	"-F":          true,
	"--header":    true,
	"-H":          true,
	"--hostname":  true,
	"--input":     true,
	"--jq":        true,
	"-q":          true,
	"--method":    true,
	"-X":          true,
	"--preview":   true,
	"-p":          true,
	"--raw-field": true,
	"-f":          true,
	"--template":  true,
	"-t":          true,
}

// apiCall is one `gh api` invocation as the boundary reads it.
type apiCall struct {
	// endpoint is the first positional argument: "graphql" for a GraphQL
	// call, a REST path otherwise.
	endpoint string
	// method is the verb --method or -X named, upper-cased, and empty when
	// the invocation named none.
	method string
	// fields are the values the field flags carried, by key.
	fields map[string]string
	// body records that at least one field flag was given, which is what
	// makes gh default the method to POST.
	body bool
	// input records --input, which sends a request body from a file or
	// from standard input.
	input bool
}

// flagOf splits one argument into a flag name and any value attached to it. It
// answers an empty name for a positional argument.
func flagOf(arg string) (name, value string, attached bool) {
	if strings.HasPrefix(arg, "--") {
		name, value, attached = strings.Cut(arg, "=")
		return name, value, attached
	}
	if strings.HasPrefix(arg, "-") && len(arg) > 1 {
		return arg[:2], arg[2:], len(arg) > 2
	}
	return "", "", false
}

// readAPI reads an argument vector into the parts the boundary judges. The
// vector excludes the `api` subcommand itself.
func readAPI(args []string) apiCall {
	call := apiCall{fields: map[string]string{}}
	for i := 0; i < len(args); i++ {
		name, value, attached := flagOf(args[i])
		if name == "" {
			if call.endpoint == "" {
				call.endpoint = args[i]
			}
			continue
		}
		if !valueArgs[name] {
			continue
		}
		if !attached {
			i++
			if i == len(args) {
				break
			}
			value = args[i]
		}
		switch name {
		case "--method", "-X":
			call.method = strings.ToUpper(value)
		case "--input":
			call.input = true
		case "--field", "-F", "--raw-field", "-f":
			call.body = true
			key, text, _ := strings.Cut(value, "=")
			call.fields[key] = text
		}
	}
	return call
}

// readOnly reports whether one gh invocation only reads, and names what the
// boundary objected to when it does not.
//
// The rule is an allowlist, closed by default, for the same reason this
// package's environment is one: a list of the writes to refuse goes stale
// every time gh grows a subcommand, and it goes stale in the direction that
// posts something to a colleague's pull request. A read cr needs and this does
// not yet name is a one-line, deliberate widening; a write nobody anticipated
// is refused without anybody having anticipated it.
//
// `api` is the whole list today because it is the whole of what cr reads
// today. Ingestion is `gh api graphql`, which §3.5.1 needs for a thread's
// resolution state, and that is the only invocation this package builds.
func readOnly(args []string) (read bool, why string) {
	if len(args) == 0 {
		return false, "no subcommand"
	}
	if args[0] != "api" {
		return false, fmt.Sprintf("%q is not one of the subcommands the boundary reads with", args[0])
	}

	call := readAPI(args[1:])
	// --input is judged ahead of the endpoint, GraphQL included: a document
	// arriving from a file or from standard input is one the boundary
	// cannot read, and a call carrying both a query field and an --input
	// body is one where gh, not cr, decides which of them is sent.
	if call.input {
		return false, "--input sends a request body"
	}
	if call.endpoint == "graphql" {
		document, ok := call.fields["query"]
		if !ok {
			return false, "a GraphQL call carrying no query field says nothing about what it does"
		}
		return documentReads(document)
	}
	if call.method != "" && call.method != "GET" && call.method != "HEAD" {
		return false, fmt.Sprintf("--method %s writes", call.method)
	}
	// gh defaults the method to POST as soon as a field flag is given, so a
	// field with no method is a write however innocent the endpoint looks.
	// With GET named explicitly the same fields become query parameters.
	if call.body {
		return false, "a field flag with no --method GET makes the request a POST"
	}
	return true, ""
}

// documentReads reports whether a GraphQL document only reads.
//
// The HTTP verb answers nothing here: gh sends every GraphQL call as a POST,
// the ingestion queries of §3.5.1 included, so a boundary reading verbs would
// have to let POST through and would then let every mutation through with it.
//
// The operation keyword is what separates the two, and GraphQL has exactly
// three of them — query, mutation, subscription. That set is fixed by the
// language, not by what gh or GitHub support this month, so naming the two
// that write is exhaustive rather than a denylist that ages. The document must
// open with a query, written as the keyword or as the anonymous shorthand, and
// no later operation may be one of the other two — which is the only way a
// mutation could ride into a document behind a leading query.
//
// Anything else is refused: a document opening with a fragment reads fine and
// is still refused, because cr sends none, and a boundary that guesses at the
// documents it has not seen is guessing about a network write.
func documentReads(document string) (read bool, why string) {
	tokens := graphqlTokens(document)
	if len(tokens) == 0 {
		return false, "the GraphQL document is empty"
	}
	if tokens[0] != "query" && tokens[0] != "{" {
		return false, fmt.Sprintf("the GraphQL document opens with %q rather than a query", tokens[0])
	}
	for _, token := range tokens[1:] {
		if token == "mutation" || token == "subscription" {
			return false, fmt.Sprintf("the GraphQL document names a %s past its first operation", token)
		}
	}
	return true, ""
}

// graphqlTokens splits a GraphQL document into its names and its punctuation,
// with comments removed and whitespace and commas dropped as GraphQL drops
// them.
//
// String contents are tokenised like everything else rather than skipped. A
// literal holding the word mutation is then read as an operation and the
// document is refused, which is wrong in the direction that sends nothing.
func graphqlTokens(document string) []string {
	var tokens []string
	var name strings.Builder
	commented := false
	flush := func() {
		if name.Len() > 0 {
			tokens = append(tokens, name.String())
			name.Reset()
		}
	}
	for _, r := range document {
		switch {
		case commented:
			if r == '\n' {
				commented = false
			}
		case r == '#':
			flush()
			commented = true
		case r == '_' || unicode.IsLetter(r) || unicode.IsDigit(r):
			name.WriteRune(r)
		default:
			flush()
			if !unicode.IsSpace(r) && r != ',' {
				tokens = append(tokens, string(r))
			}
		}
	}
	flush()
	return tokens
}
