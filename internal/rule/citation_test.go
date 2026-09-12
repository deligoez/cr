package rule

import (
	"reflect"
	"testing"

	"github.com/deligoez/cr/internal/finding"
	"github.com/deligoez/cr/internal/probe"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// aHit is one §2.6.1.1 hit, as detection produces it.
func aHit() Hit {
	return Hit{
		RuleID: "no-raw-sql",
		Path:   "app/Models/Order.php",
		Line:   42,
		Text:   "$rows = DB::raw('select 1');",
	}
}

// §2.6.1.3: a hit is recorded as a citations entry carrying the matched path,
// the matched line, and `origin: rule`.
func TestAHitBecomesACitationCarryingPathLineAndRuleOrigin(t *testing.T) {
	hit := aHit()

	citation := hit.Citation()

	assert.Equal(t, finding.Citation{
		Path:   "app/Models/Order.php",
		Line:   42,
		Origin: finding.OriginRule,
	}, citation)
}

// §6.2.3 has cr compute the content hash at record time, against the head the
// record is being written for, and §6.2's `cited` row reads its presence as the
// mark of an entry cr actually resolved. A hash written here would claim a
// resolution that has not happened, and would buy the grade on the strength of
// it.
func TestTheCitationCarriesNoContentHashYet(t *testing.T) {
	hit := aHit()

	assert.Empty(t, hit.Citation().ContentHash)
}

// §2.6 item 3 puts the rule id on the record's own `rule` field, not in the
// citation. §6.2.5 matches a citation to a hit by head, rule id, path and line
// together, so an id inside the citation would be a second copy of one of the
// four halves of that match — able to disagree with the record carrying it.
//
// The assertion walks the citation rather than naming its fields, so a `rule`
// added to §6.1's citation shape years from now fails here.
func TestTheRuleIDIsCarriedOnTheRecordAndNotInTheCitation(t *testing.T) {
	hit := aHit()
	require.NotEmpty(t, hit.RuleID, "a hit with no rule id would pass this vacuously")

	citation := hit.Citation()

	value := reflect.ValueOf(citation)
	for i := range value.NumField() {
		field := value.Field(i)
		if field.Kind() != reflect.String {
			continue
		}
		assert.NotEqual(t, hit.RuleID, field.String(),
			"%s carries the rule id; §2.6 item 3 puts it on the record",
			value.Type().Field(i).Name)
	}

	// The record's field is where it belongs, and it is a different field
	// from anything the citation holds.
	record := finding.Finding{Rule: hit.RuleID}
	assert.Equal(t, "no-raw-sql", record.Rule)
}

// §6.2.5 stamps `origin` and rejects a record arriving with any value in it,
// because that field is what buys §6.2's `cited` row for a citation inside the
// record's own unit. An agent able to write it could name any rule, point
// anywhere inside its own unit, and be graded as though a detector had found
// it.
//
// So the assertion is about the signature and not only the value: Citation
// takes nothing but the hit, which means the only way to obtain a rule-origin
// citation is to have run detection and be holding its output. A parameter
// added here would be the hole, and it fails this rather than a grading test
// three sections away.
func TestTheOriginIsStampedByCrAndCannotBeSupplied(t *testing.T) {
	method := reflect.TypeOf((*Hit).Citation)

	require.Equal(t, 1, method.NumIn(), "the receiver is the only input; nothing is passed in")
	assert.Equal(t, reflect.TypeFor[*Hit](), method.In(0))
	require.Equal(t, 1, method.NumOut())
	assert.Equal(t, reflect.TypeFor[finding.Citation](), method.Out(0))

	hit := aHit()
	assert.Equal(t, finding.OriginRule, hit.Citation().Origin)
}

// §2.6.1.3 says a hit *may* grade the record `cited` per §6.2 — not that it
// does. Three things have to hold, and each of them is a way the same citation
// comes back `argued`.
//
// The test axis is the one §2.6.1.3 could be misread about. §6.2's `cited` row
// excludes it outright, because §4.4.2 has a test-adequacy finding assert only
// with an experiment: asserting a unit is untested on the strength of having
// read the tests is exactly the claim §5.3's `no-test-failed` exists to settle,
// and a `cited` grade would let it be made without running anything.
func TestARuleCitationMayGradeCitedRatherThanDoes(t *testing.T) {
	hit := aHit()
	resolved := hit.Citation()
	// §6.2.3's hash, as cr stamps it at record time. Without it the entry
	// is one no resolution ever reached.
	resolved.ContentHash = "0f1e2d3c4b5a6978"

	nothingProbed := finding.Resolved(nil, nil, nil, "", probe.Baseline{}, probe.ClaimMapping{})

	for _, c := range []struct {
		name      string
		axis      string
		citations []finding.Citation
		grade     finding.Grade
	}{
		{
			name: "on an axis that is not test, once cr resolved it",
			axis: "convention", citations: []finding.Citation{resolved},
			grade: finding.GradeCited,
		},
		{
			name: "the same citation on the test axis",
			axis: "test", citations: []finding.Citation{resolved},
			grade: finding.GradeArgued,
		},
		{
			name: "before cr resolved it against the head",
			axis: "convention", citations: []finding.Citation{hit.Citation()},
			grade: finding.GradeArgued,
		},
		{
			name: "on an axis cr never computed",
			axis: "", citations: []finding.Citation{resolved},
			grade: finding.GradeArgued,
		},
	} {
		t.Run(c.name, func(t *testing.T) {
			record := &finding.Finding{Axis: c.axis, Citations: c.citations}

			assert.Equal(t, c.grade, finding.ComputeGrade(record, nothingProbed))
		})
	}
}
