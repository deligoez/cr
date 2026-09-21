package review

import (
	"slices"
	"strconv"
	"strings"

	"github.com/deligoez/cr/internal/finding"
	"github.com/deligoez/cr/internal/intent"
	"github.com/deligoez/cr/internal/proposal"
	"github.com/deligoez/cr/internal/render"
	"github.com/deligoez/cr/internal/role"
)

// findingWords says what §6.1's Required column means to the agent writing a
// record, one phrase per value of the column.
var findingWords = map[finding.Requirement]string{
	finding.Required: "required",
	finding.Optional: "optional",
	finding.Computed: "computed by cr",
	finding.Stamped:  "stamped by cr",
}

// claimWords is the same for §3.3's column, which is a table of its own and
// therefore a type of its own (see intent.Requirement).
var claimWords = map[intent.Requirement]string{
	intent.Required: "required",
	intent.Optional: "optional",
	intent.Computed: "computed by cr",
	intent.Stamped:  "stamped by cr",
}

// contract writes §4.6.2's part of the prompt: the NDJSON path the role writes
// to with the record ids it may write, the fields §6.1.4 forbids the agent to
// write, and the round's contract file, which holds §6.1's record schema.
//
// The schema is in the file rather than in every prompt because it is the same
// text for every prompt of the round, and a round's prompts are many. What a
// prompt keeps is what differs per prompt — the path and the ids — and the
// fence, which is the part a role must not miss.
//
// Every list here is read out of the tables the decoders refuse by, rather than
// written out, so the prompt cannot tell a role it may write a field `cr merge`
// will reject. cr owns this contract and a role only supplies persona and focus
// (§2.5), which is why none of it comes from the role's instructions.
func contract(p *page, lens *role.Role, output, contractFile string, round int, ids IDs) {
	p.section("Output (§4.6.2)")
	p.line("Write this role's records for this unit, one JSON object per line, to:")
	p.line("")
	p.line("    %s", output)
	p.line("")
	p.line("The file's name binds every record in it to role %s: cr merge attributes a record to "+
		"the role whose file it arrived in and rejects one naming another (§6.1.3). With nothing to "+
		"raise, write nothing.", lens.ID)
	p.line("")
	idBlockLine(p, round, ids)
	p.line("")
	p.line("A record's fields, the values each takes, and a citation's fields are §6.1's record schema, " +
		"in the round's contract file; read it before writing a record:")
	p.line("")
	p.line("    %s", contractFile)
	p.line("")
	// §6.1.1 is a MUST cr cannot check, since reading a language is a
	// judgement and cr forms none; the prompt is the one place every role
	// is certain to read it.
	p.line("%s", englishLine)
	p.line("")
	forbiddenLine(p)
}

// proposalSchema writes §5.7's table into the round's contract file, beside
// §6.1's, so a role reads both schemas in one place.
func proposalSchema(p *page) {
	p.section("Proposed experiment schema (§5.7)")
	p.line("A proposal carries §5.7's fields:")
	for _, field := range proposal.Fields() {
		p.line("- %s: %s", field.Name, findingWords[field.Requirement])
	}
	p.line("")
	p.line("kind is one of %s. target is a path:line inside the proposal's own unit, resolved against "+
		"the round's head. hypothesis and settles are English (§6.1.1). A proposal is never evidence: "+
		"only `cr probe run --proposal <id>` turns one into a probe record, and that is what re-grades "+
		"the record a proposal names in finding.", strings.Join(proposal.Kinds(), " or "))
}

// proposalContract writes §4.6.2's second half: the file the role writes §5.7's
// proposed experiments to, and the block of proposal ids it may write there.
//
// It is a section of its own rather than a paragraph of the record contract,
// because the two are different asks with different refusals. A role with a
// suspicion it cannot establish writes here as well as there, and the sentence
// that matters most is the last one: running the experiment is what changes the
// register, so proposing one is not a way to assert.
func proposalContract(p *page, proposals string, round int, asks IDs) {
	p.section("Proposed experiments (§5.7)")
	p.line("A suspicion you cannot establish from reading is what §5.7 exists for. Write the experiment " +
		"that would settle it, one JSON object per line, to:")
	p.line("")
	p.line("    %s", proposals)
	p.line("")
	p.line("A proposal carries %s. `kind` is one of %s; `target` is a path:line inside this unit; "+
		"`hypothesis` is what you believe and cannot establish; `settles` is the result that would "+
		"settle it; `input` is the unified diff for a mutation or the test file's content for a gap. "+
		"`finding` may name a record you wrote for this unit, and then running the proposal re-grades "+
		"that record.",
		strings.Join(proposalWritable(), ", "), strings.Join(proposal.Kinds(), " or "))
	p.line("")
	p.line("%s", scopeLine())
	p.line("")
	if first, last := asks.spelledAs(proposal.IDOf); first == "" {
		p.line("This prompt's block of proposal ids is spent: every id in it is held by a stored " +
			"proposal, so propose nothing here and say so to the operator.")
	} else {
		p.line("Give the proposals you write here the ids %s through %s, in order from %s. No other "+
			"prompt of round %d is given any of them (§4.6.2).", first, last, first, round)
	}
	p.line("")
	p.line("A proposal is **not evidence**. `probe` stays the id of an experiment that ran, and a " +
		"record naming a proposal there is rejected; an unestablished suspicion is still a question " +
		"(§4.1.4, §6.3). Only `cr probe run --proposal <id>` turns a proposal into evidence.")
	p.line("")
	p.line("You may not write %s; cr computes or stamps them (§5.7, §2.3.3).",
		strings.Join(proposal.Reserved(), ", "))
}

// scopeLine glosses §5.7's `filter` and `paths`, the two rows a proposal's
// sentence named without saying what they mean.
//
// Measured in M4 part B: they were the only two fields the sentence left
// unexplained, and `x36101` filled `paths` with the source file its hypothesis
// was about. §5.4.2 passes each value through `tests.paths_arg`, so the runner
// was handed `go test ./internal/probe/resolve.go`, which compiles that one file
// as a package of its own and exits on eight undefined symbols — no test ran, and
// the experiment settled nothing. A field an agent is judged by is one the prompt
// has to explain, and the failure it invites is silent: the run succeeds at
// running and answers nothing.
func scopeLine() string {
	return "`filter` and `paths` scope the run, never the code the experiment is about. `filter` is " +
		"the name of a test for the runner to select, not a runner argument string; `paths` are test " +
		"targets the runner understands, the package or directory whose tests are to run, never the " +
		"file the patch changes. Leave both out to run the whole suite, which is the safe answer " +
		"when you are unsure."
}

// proposalWritable is §5.7's rows an agent writes, each with what the Required
// column answers, read off proposal.Fields so the prompt cannot offer a field
// the decoder refuses.
func proposalWritable() []string {
	reserved := proposal.Reserved()
	words := make([]string, 0, len(proposal.Fields()))
	for _, field := range proposal.Fields() {
		if slices.Contains(reserved, field.Name) {
			continue
		}
		words = append(words, field.Name+" ("+findingWords[field.Requirement]+")")
	}
	return words
}

// englishLine states §6.1.1 to the one writing a record.
const englishLine = "Write summary and evidence in English (§6.1.1), whatever language the issue, the " +
	"threads or the code comments are in; reader-facing prose is produced from them at draft time (§8.1)."

// reservedLine states §8.1.3's sequence to the one writing a record.
//
// It is a fence a role is judged by and, until measurement 4, was never told:
// §8.1.2 composes a comment's body out of `summary` and `evidence`, so a record
// carrying the sequence in either is refused where the record enters and no
// later command repairs it. M4 part B lost a whole unit's work to that silence —
// a role reviewing cr's own draft machinery quoted the sequence in order to name
// it, `cr record` refused the record, and `cr proposals record` then refused the
// proposal that named it, for naming no record of the round.
//
// The sequence is render.Reserved rather than a literal, so the prompt names the
// bytes the refusal looks for.
func reservedLine() string {
	return "No field of a record may carry the sequence " + strconv.Quote(render.Reserved) +
		": §8.1.3 reserves it for the record marker and cr's own regions, and §8.1.2 composes a " +
		"comment's body out of summary and evidence, so a record carrying it is rejected with exit " +
		"code 1 where the record enters. When the code under review is cr's own marker machinery, " +
		"name the sequence in words rather than quoting it."
}

// forbiddenLine states §6.1.4's fence: every field an agent's line may not carry.
func forbiddenLine(p *page) {
	p.line("You may not write %s. cr computes or stamps them, and a record arriving with one is "+
		"rejected with exit code 1 (§6.1.4, §2.3.3).", strings.Join(forbidden(), ", "))
}

// Contract is the text of `rounds/<n>/contract.md` (§4.6.2): §6.1's record
// schema, read out of the tables the decoders refuse by — finding.Fields,
// finding.CitationFields, finding.Reserved — so the file cannot tell a role it
// may write a field `cr merge` will reject, or forbid one it accepts.
func Contract(round int) string {
	var p page
	p.line("# Record contract for round %d (§4.6.2)", round)
	p.line("")
	p.line("Every prompt of round %d names this file. A role writes its records to the path its prompt "+
		"names, one JSON object per line, with the ids its prompt names.", round)
	p.section("Record schema (§6.1)")
	p.line("A record carries §6.1's fields:")
	reserved := finding.Reserved()
	for _, field := range finding.Fields() {
		word := findingWords[field.Requirement]
		// §6.1's column calls disposition, duplicate_of and thread_id
		// optional, and §6.1.4 reserves them to cr all the same; a list
		// saying only "optional" would read as leave to write them.
		if field.Requirement != finding.Computed && slices.Contains(reserved, field.Name) {
			word += ", and §6.1.4 reserves it to cr"
		}
		p.line("- %s: %s", field.Name, word)
	}
	p.line("")
	domains(&p)
	p.line("")
	p.line("Each entry of citations carries:")
	for _, field := range finding.CitationFields() {
		p.line("- %s: %s", field.Name, findingWords[field.Requirement])
	}
	p.line("")
	p.line("%s", englishLine)
	p.line("A kind=question record's posted body must contain \"?\" (§8.1.5); that body is composed at draft " +
		"time, where a question body that does not ask is rewritten into one before `cr post` accepts it.")
	p.line("%s", reservedLine())
	p.line("")
	forbiddenLine(&p)
	proposalSchema(&p)
	return p.String()
}

// forbidden is every field an agent's line may not carry: §6.1.4's fence, a
// citation's computed fields, and the two §2.3.3 stamps.
func forbidden() []string {
	names := finding.Reserved()
	for _, field := range finding.CitationFields() {
		if field.Requirement == finding.Computed {
			names = append(names, "a citation's "+field.Name)
		}
	}
	for _, field := range finding.Fields() {
		if field.Requirement == finding.Stamped {
			names = append(names, field.Name)
		}
	}
	return names
}

// domains writes the values §6.1's fields take where the decoder holds them to
// more than presence: the id's spelling, kind's, severity's and
// suggestion_origin's closed sets, class's form, and §9.2's anchor object key
// by key.
//
// Every value is read out of what the decoder refuses by — finding.Kinds,
// finding.Severities, finding.Origins, finding.ClassForm, finding.IDForm and
// finding.AnchorFields — so a role told a value here is told one `cr merge`
// accepts, and a value the decoder stops accepting leaves this list with it.
func domains(p *page) {
	p.line("cr merge and cr record reject a record whose values fall outside these, with exit code 1 (§6.1, §9.2):")
	p.line("- id: %s", finding.IDForm)
	p.line("- kind: %s", oneOf(finding.Kinds()))
	p.line("- class: kebab-case, matching %s", finding.ClassForm)
	p.line("- severity: %s", oneOf(finding.Severities()))
	p.line("- anchor: an object carrying")
	for _, field := range finding.AnchorFields() {
		p.line("  - %s: %s; %s", field.Name, findingWords[field.Requirement], field.Values)
	}
	p.line("- suggestion_origin: %s", oneOf(finding.Origins()))
}

// oneOf names a closed set's values for the prompt, each quoted as a JSON line
// writes it, in the set's own order.
func oneOf[T ~string](values []T) string {
	quoted := make([]string, 0, len(values))
	for _, value := range values {
		quoted = append(quoted, strconv.Quote(string(value)))
	}
	return "one of " + strings.Join(quoted, ", ")
}

// claimSchema writes §3.3's claim record for an intent role, whose lens is the
// claims: the fields a claim line carries, the sources it may name, and the
// two hashes §6.1.4 names beside its own fence because cr computes them here.
func claimSchema(p *page) {
	p.section("Claim record (§3.3)")
	p.line("Claims are recorded through `cr claims record`, one JSON object per line, and a " +
		"claim carries §3.3's fields:")
	for _, field := range intent.ClaimFields() {
		p.line("- %s: %s", field.Name, claimWords[field.Requirement])
	}
	sources := make([]string, 0, 4)
	for _, source := range intent.ClaimSources() {
		sources = append(sources, source.String())
	}
	p.line("")
	p.line("source is one of %s; a claim drawn from a note names it in note_id and its span is "+
		"the note's body (§3.3.2).", strings.Join(sources, ", "))
	// The separator line is intent.Separator's, not a copy written out
	// here: §3.3.1 holds a span to one part of the issue text, and a
	// prompt showing a line other than the one cr writes would have a role
	// draw spans across a boundary it could not see.
	p.line("A claim drawn from an extra intent file carries source %s and names that file in file, "+
		"as the path was given to --intent-extra or intent.extra_files; the line `%s` above the file's "+
		"text names the same path. Its span must lie wholly inside that file's part of the issue text, "+
		"and no span may contain or cross such a line (§3.1.5, §3.3.1).",
		strconv.Quote(intent.ClaimFromFile.String()), intent.Separator("<path>"))
}
