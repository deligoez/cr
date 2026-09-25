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

