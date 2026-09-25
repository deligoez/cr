package render

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/deligoez/cr/internal/finding"
	"github.com/deligoez/cr/internal/probe"
)

// §8.1.7: the evidence region's field names are built in for every language
// render.lang admits, as §8.1.4's labels are. A language that joins the
// enumeration without joining the table would render its comments' regions in
// another language's field names.
func TestEveryLanguageCarriesBuiltInEvidenceFieldNames(t *testing.T) {
	require.Len(t, evidenceFieldNames, len(Langs()), "one row per language, and no row for a language cr does not render")
	for _, lang := range Langs() {
		found := 0
		for _, row := range evidenceFieldNames {
			if row.lang == lang {
				found++
			}
		}
		assert.Equal(t, 1, found, "language %s has exactly one row of evidence field names", lang)
	}
}

// §8.1.7 under render.lang `tr`: every field name of a probed record's region
// is Turkish, and every value beneath it — the probe's kind, its target, its
// `result` and the runner's output — is written as the record holds it.
//
// Measured with cr 0.15.0 on a real pull request on a private Laravel
// repository: the region read `kind:`, `target:`, `paths:`, `result:` and
// `input:` in an otherwise Turkish comment.
func TestAProbedRegionUnderTurkishNamesItsFieldsInTurkish(t *testing.T) {
	region, err := ProbeEvidence(LangTR, "f1", &probe.Record{
		ID:         "p1",
		Kind:       probe.Mutation,
		Target:     "src/Order.php:34",
		Filter:     "charges shipping",
		Paths:      []string{"tests/Feature"},
		Result:     "no-test-failed",
		Input:      aPatch,
		OutputTail: "  Tests:  1 passed (1 assertions)\n",
	}, inputCap, "", []*probe.Record{{ID: "p2", Result: "test-failed"}})
	require.NoError(t, err)

	assert.Equal(t, strings.Join([]string{
		"<!-- cr:evidence -->",
		"tür: mutation",
		"hedef: src/Order.php:34",
		"filtre: charges shipping",
		"yollar: tests/Feature",
		"sonuç: no-test-failed",
		"girdi:",
		"```",
		strings.TrimSuffix(aPatch, "\n"),
		"```",
		"test çıktısı:",
		"```",
		"  Tests:  1 passed (1 assertions)",
		"```",
		"yeniden koşu: p2, sonuç: test-failed",
		"<!-- cr:/evidence -->",
	}, "\n"), region)
}

