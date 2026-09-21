package cli

import (
	"go/ast"
	"go/parser"
	"go/token"
	"reflect"
	"slices"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/deligoez/cr/internal/coverage"
	"github.com/deligoez/cr/internal/finding"
	"github.com/deligoez/cr/internal/testadequacy"
)

// agentJudgement is one of §2.1.3's judgements that belong to the agent alone,
// with the field cr records the agent's decision in.
//
// Five of the six are recorded on a field of a value that arrives as the
// agent's own line. The sixth, whether a posted concern has been addressed, is
// recorded nowhere in v0.1 but in the note `cr answer` appends, and has its
// own test below.
type agentJudgement struct {
	// judgement is §2.1.3's wording.
	judgement string
	// carrier is the type the decision is recorded on, as package.Type.
	carrier string
	// field is the Go field holding the decision.
	field string
	// decoder is the carrier's one method that may assign the field from a
	// value it did not copy, because it is the agent's line being decoded.
	// Empty when encoding/json assigns the field itself.
	decoder string
}

// agentJudgements is §2.1.3's list, in its order, less the sixth.
//
// A rule hit is confirmed by a record naming the rule and citing the hit
// (rule.confirmed), and a reinvention candidate is taken up by a record citing
// it as `path:line` (§4.3.3), so both decisions are the record's own fields; a
// note explaining an unmapped unit is the cell's `note_id` (§4.1.5); a thread
// covering a finding is `suppressed_by` (§3.5.4); and the test-adequacy verdict
// is the cell's coverage classification (§4.4.1).
var agentJudgements = []agentJudgement{
	{judgement: "whether a rule hit is a real violation", carrier: "finding.Finding", field: "Rule"},
	{judgement: "whether a candidate symbol is semantic reinvention", carrier: "finding.Finding", field: "Citations"},
	{judgement: "whether a note explains an unmapped unit", carrier: "coverage.Cell", field: "NoteID"},
	{judgement: "whether an existing human thread already covers a finding", carrier: "finding.Finding", field: "SuppressedBy"},
	{
		judgement: "whether a unit is covered by tests", carrier: "testadequacy.Coverage",
		field: "classification", decoder: "UnmarshalJSON",
	},
}

// §2.1.3's "records the decision": every field above is one the agent's line
// supplies. A field §6.1.4 reserves, or one with no key on the wire, would be a
// decision nobody but cr could have written.
func TestEveryAgentJudgementIsRecordedOnAFieldTheAgentSupplies(t *testing.T) {
	carriers := map[string]reflect.Type{
		"finding.Finding":       reflect.TypeFor[finding.Finding](),
		"coverage.Cell":         reflect.TypeFor[coverage.Cell](),
		"testadequacy.Coverage": reflect.TypeFor[testadequacy.Coverage](),
	}
	for _, j := range agentJudgements {
		carrier, known := carriers[j.carrier]
		require.True(t, known, "%s: no carrier type %s", j.judgement, j.carrier)
		field, held := carrier.FieldByName(j.field)
		require.True(t, held, "%s: %s has no field %s", j.judgement, j.carrier, j.field)
		if j.decoder != "" {
			_, decodes := reflect.PointerTo(carrier).MethodByName(j.decoder)
			assert.True(t, decodes, "%s: %s has no %s to decode the agent's line", j.judgement, j.carrier, j.decoder)
			assert.False(t, field.IsExported(), "%s: an exported %s.%s can be set by anything", j.judgement, j.carrier, j.field)
			continue
		}
		key, _, _ := strings.Cut(field.Tag.Get("json"), ",")
		require.NotEmpty(t, key, "%s: %s.%s arrives under no key", j.judgement, j.carrier, j.field)
		assert.NotContains(t, finding.Reserved(), key,
			"%s: `%s` is reserved, so the agent could not have supplied it", j.judgement, key)
	}
}

// §2.1.3: cr locates the candidate, supplies it and records the decision, and
// has no code path producing a verdict for any of the five judgements above.
//
// Read off the source, a verdict is a write to the field that holds the
// decision whose value did not come from that same field. A copy — a draft
// block disclosing `record.Rule`, a triage event carrying it — moves the agent's
// decision somewhere else; anything else, a constant, a thread's id, a
// computed classification, is cr deciding. The carrier's decoder is the one
// exception, because what it assigns is the agent's line.
//
// The walk goes by field name, not by type, so a same-named field of an
// unrelated type that is assigned a non-copy would be reported too. None is
// today; one that appears is declared here with its reason, the way
// crossRoundReaders declares its reads.
func TestNoCodePathProducesAVerdictForAnAgentJudgement(t *testing.T) {
	guilty := parseGuilty(t, "guilty.go", `package guilty

import (
	"github.com/deligoez/cr/internal/coverage"
	"github.com/deligoez/cr/internal/finding"
)

func decide(record, other *finding.Finding, cell *coverage.Cell, thread struct{ ID string }) {
	record.SuppressedBy = thread.ID
	cell.NoteID = "CR-1#n1"
	record.Citations = append(record.Citations, other.Citations...)
	other.Rule = record.Rule
	_ = []*coverage.Cell{{NoteID: "CR-1#n2"}, {NoteID: cell.NoteID}}
}
`)
	assert.Equal(t, []string{
		"guilty.go: decide writes SuppressedBy",
		"guilty.go: decide writes NoteID",
		"guilty.go: decide writes Citations",
		"guilty.go: decide writes NoteID",
	}, verdictWrites("guilty.go", guilty),
		"the detector must see an assignment, an append and an elided literal, and pass a copy")
	decoded := parseGuilty(t, "coverage.go", `package testadequacy

func (c *Coverage) UnmarshalJSON(data []byte) error { c.classification = w.Classification; return nil }
func (c *Coverage) classify() { c.classification = Covered }
`)
	assert.Equal(t, []string{"coverage.go: classify writes classification"}, verdictWrites("coverage.go", decoded),
		"the decoder may assign the classification and nothing else of the carrier's may")

	var found []string
	eachSourceFile(t, func(rel string, file *ast.File) {
		found = append(found, verdictWrites(rel, file)...)
	})
	assert.Empty(t, found, "§2.1.3: these judgements belong to the agent alone, so cr may only copy them")
}

// §2.1.3's first two judgements are recorded by a record existing at all: a hit
// no record confirms is dismissed (§2.6.1.5), and a candidate no record cites
// was not reinvention. So a record cr built itself would be both verdicts at
// once, and nothing outside a test builds one — every finding.Finding cr holds
// was decoded from a line the agent wrote.
func TestNothingInCrBuildsARecord(t *testing.T) {
	guilty := parseGuilty(t, "guilty.go", `package guilty

import "github.com/deligoez/cr/internal/finding"

func confirm() []*finding.Finding {
	made := &finding.Finding{Rule: "no-panic"}
	return []*finding.Finding{made, {Class: "reinvention"}, new(finding.Finding)}
}
`)
	assert.Len(t, recordConstructions("guilty.go", guilty), 3,
		"the detector must see a literal, an elided literal and an allocation")

	var found []string
	eachSourceFile(t, func(rel string, file *ast.File) {
		found = append(found, recordConstructions(rel, file)...)
	})
	assert.Empty(t, found, "§2.1.3: a record cr built would be a verdict on a rule hit or a candidate")
}

// §2.1.3's sixth judgement, whether a posted concern has been addressed, is the
// one v0.5 gave a home to — and the guard has to change with it without letting
// go of what it was guarding.
//
// Through v0.4 the invariant was enforced by absence: `posted` was terminal, so
// no actor could move a record out of it and the judgement had nowhere to land.
// §9.5.5 now gives it somewhere, which makes the question sharper rather than
// moot: the judgement is still the agent's, and cr's part is to copy it in.
// §9.5.6 says so directly, because every signal cr can read — an outdated
// thread, a probe that stopped reproducing, an author writing "fixed" — is as
// consistent with a concern that was addressed as with one whose code was
// deleted.
//
// So the assertion is now about *which* actors may settle a posted record, and
// it is exactly the two commands that carry a judgement in from outside. Any
// third would be a command that settled one on cr's own reading.
func TestOnlyACarriedJudgementSettlesAPostedConcern(t *testing.T) {
	assert.False(t, finding.StatePosted.Terminal(),
		"§9.1.2 from v0.5: a posted concern nobody has settled is open")

	settling := map[finding.Actor][]finding.State{
		finding.ActorVerify:          {finding.StateAnswered, finding.StateAddressed},
		finding.ActorWithdrawConfirm: {finding.StateWithdrawn},
	}
	for _, to := range finding.States() {
		for _, by := range finding.Actors() {
			err := finding.MayTransition("f1", finding.Existing(finding.StatePosted), to, by)
			if slices.Contains(settling[by], to) {
				assert.NoError(t, err, "§9.1: %s settles a posted record as %s", by, to)
				continue
			}
			assert.Error(t, err,
				"%s may move a posted record to %s, which would be cr settling it", by, to)
		}
	}

	var moves []string
	eachSourceFile(t, func(rel string, file *ast.File) {
		if file.Name.Name != "note" {
			return
		}
		ast.Inspect(file, func(n ast.Node) bool {
			if call, isCall := n.(*ast.CallExpr); isCall &&
				slices.Contains([]string{"MayTransition", "NewJournal", "Move", "Existing"}, calledName(call)) {
				moves = append(moves, rel+": "+calledName(call))
			}
			return true
		})
	})
	assert.Empty(t, moves, "§3.6.2: an answer changes no record's state")
}

// parseGuilty parses a fixture source for a detector to be proved against.
func parseGuilty(t *testing.T, name, source string) *ast.File {
	t.Helper()
	parsed, err := parser.ParseFile(token.NewFileSet(), name, source, parser.SkipObjectResolution)
	require.NoError(t, err)
	return parsed
}

// verdictWrites lists every write in one file to a field agentJudgements names
// whose value is not a copy of the same field, outside the carrier's decoder.
//
// An assignment is matched by field name alone, since its target's type is not
// in the syntax. A keyed literal's is, so a key is matched only inside a
// literal of the carrier — its own type named, or elided inside a collection of
// them — and `Rule:` in a rule.Resolved or an activation reason is left alone.
func verdictWrites(rel string, file *ast.File) []string {
	var found []string
	for _, decl := range file.Decls {
		within, isFunc := decl.(*ast.FuncDecl)
		if !isFunc || within.Body == nil {
			continue
		}
		report := func(field string) {
			if !decodes(file.Name.Name, within, field) {
				found = append(found, rel+": "+within.Name.Name+" writes "+field)
			}
		}
		elided := map[*ast.CompositeLit]string{}
		ast.Inspect(within.Body, func(n ast.Node) bool {
			switch node := n.(type) {
			case *ast.AssignStmt:
				for i, lhs := range node.Lhs {
					target, isField := lhs.(*ast.SelectorExpr)
					if isField && judged("", target.Sel.Name) && !copies(assigned(node, i), target.Sel.Name) {
						report(target.Sel.Name)
					}
				}
			case *ast.CompositeLit:
				carrier := elided[node]
				if node.Type != nil {
					carrier = carrierOf(file.Name.Name, node.Type)
				}
				member := carrierOf(file.Name.Name, elementOf(node.Type))
				for _, elt := range node.Elts {
					if inner, isLit := elt.(*ast.CompositeLit); isLit && inner.Type == nil {
						elided[inner] = member
					}
					pair, isPair := elt.(*ast.KeyValueExpr)
					if !isPair {
						continue
					}
					key, isIdent := pair.Key.(*ast.Ident)
					if isIdent && judged(carrier, key.Name) && !copies(pair.Value, key.Name) {
						report(key.Name)
					}
				}
			}
			return true
		})
	}
	return found
}

// judged reports whether a field holds one of agentJudgements' decisions: on
// carrier, or on any carrier when carrier is empty.
func judged(carrier, field string) bool {
	return slices.ContainsFunc(agentJudgements, func(j agentJudgement) bool {
		return j.field == field && (carrier == "" || j.carrier == carrier)
	})
}

// carrierOf names the type expr spells, as package.Type, through a pointer; it
// is empty for anything else.
func carrierOf(pkg string, expr ast.Expr) string {
	if star, isStar := expr.(*ast.StarExpr); isStar {
		expr = star.X
	}
	switch named := expr.(type) {
	case *ast.SelectorExpr:
		if from, isIdent := named.X.(*ast.Ident); isIdent {
			return from.Name + "." + named.Sel.Name
		}
	case *ast.Ident:
		return pkg + "." + named.Name
	}
	return ""
}

// decodes reports whether within is the decoder agentJudgements allows to
// assign field, declared in the carrier's own package on the carrier.
func decodes(pkg string, within *ast.FuncDecl, field string) bool {
	if within.Recv == nil || len(within.Recv.List) != 1 {
		return false
	}
	receiver := within.Recv.List[0].Type
	if star, isStar := receiver.(*ast.StarExpr); isStar {
		receiver = star.X
	}
	named, isIdent := receiver.(*ast.Ident)
	return isIdent && slices.ContainsFunc(agentJudgements, func(j agentJudgement) bool {
		return j.field == field && j.decoder != "" && j.decoder == within.Name.Name &&
			j.carrier == pkg+"."+named.Name
	})
}

// assigned is the value an assignment gives its i-th target, or nil when one
// call supplies them all.
func assigned(stmt *ast.AssignStmt, i int) ast.Expr {
	if len(stmt.Rhs) != len(stmt.Lhs) {
		return nil
	}
	return stmt.Rhs[i]
}

// copies reports whether value is the same field read off another value.
func copies(value ast.Expr, field string) bool {
	read, isField := value.(*ast.SelectorExpr)
	return isField && read.Sel.Name == field
}

// recordConstructions lists every construct in one file that builds a
// finding.Finding: a literal of the type, a literal whose type is elided inside
// a slice or map of them, and a `new` of it.
func recordConstructions(rel string, file *ast.File) []string {
	var built []string
	names := func(expr ast.Expr) bool { return carrierOf(file.Name.Name, expr) == "finding.Finding" }
	ast.Inspect(file, func(n ast.Node) bool {
		switch node := n.(type) {
		case *ast.CompositeLit:
			if node.Type != nil && names(node.Type) {
				built = append(built, rel+": builds a finding.Finding literal")
			}
			if element := elementOf(node.Type); element != nil && names(element) {
				for _, elt := range node.Elts {
					if inner, elided := elt.(*ast.CompositeLit); elided && inner.Type == nil {
						built = append(built, rel+": builds an elided finding.Finding literal")
					}
				}
			}
		case *ast.CallExpr:
			fun, isIdent := node.Fun.(*ast.Ident)
			if isIdent && fun.Name == "new" && len(node.Args) == 1 && names(node.Args[0]) {
				built = append(built, rel+": allocates a finding.Finding")
			}
		}
		return true
	})
	return built
}

// elementOf is the element type of a slice, array or map literal's type, or nil.
func elementOf(expr ast.Expr) ast.Expr {
	switch collection := expr.(type) {
	case *ast.ArrayType:
		return collection.Elt
	case *ast.MapType:
		return collection.Value
	}
	return nil
}
