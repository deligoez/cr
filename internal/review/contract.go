package review

import (
	"slices"
	"strconv"
	"strings"

	"github.com/deligoez/cr/internal/finding"
	"github.com/deligoez/cr/internal/intent"
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
// to, §6.1's record schema, and the fields §6.1.4 forbids the agent to write.
//
// Every list here is read out of the tables the decoders refuse by —
// finding.Fields, finding.CitationFields, finding.Reserved — rather than
// written out, so the prompt cannot tell a role it may write a field `cr merge`
// will reject, or forbid one it accepts. cr owns this contract and a role only
// supplies persona and focus (§2.5), which is why none of it comes from the
// role's instructions.
func contract(p *page, lens *role.Role, output string) {
	p.section("Output (§4.6.2)")
	p.line("Write this role's records for this unit, one JSON object per line, to:")
	p.line("")
	p.line("    %s", output)
	p.line("")
	p.line("The file's name binds every record in it to role %s: cr merge attributes a record to "+
		"the role whose file it arrived in and rejects one naming another (§6.1.3). With nothing to "+
		"raise, write nothing.", lens.ID)
	p.line("")
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
	domains(p)
	p.line("")
	p.line("Each entry of citations carries:")
	for _, field := range finding.CitationFields() {
		p.line("- %s: %s", field.Name, findingWords[field.Requirement])
	}
	p.line("")
	// §6.1.1 is a MUST cr cannot check, since reading a language is a
	// judgement and cr forms none; the prompt is the only place it can be
	// stated to the one writing the record.
	p.line("Write summary and evidence in English (§6.1.1), whatever language the issue, the " +
		"threads or the code comments are in; reader-facing prose is produced from them at draft time (§8.1).")
	p.line("")
	p.line("You may not write %s. cr computes or stamps them, and a record arriving with one is "+
		"rejected with exit code 1 (§6.1.4, §2.3.3).", strings.Join(forbidden(), ", "))
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
}
