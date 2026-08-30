package role

import (
	"regexp"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// repositoryArtefacts are the shapes a project's own standards are written in.
//
// §4.3.5 puts project conventions in the rule corpus of §2.6, never in prose
// buried inside role instructions, and §2.6 states the division outright: a
// role is a lens, a rule is a specific standard that lens enforces. Care alone
// does not hold that line. The drift arrives one plausible sentence at a time,
// a role that begins naming how code is written reads like a better role, and
// nothing fails.
//
// The mechanical stand-in is what the two are made of. A shipped role declares
// no profiles, so it serves every repository cr will ever review and can name
// nothing a repository owns: a path, a file, a command, an identifier. A rule
// is made of little else. The guard is therefore blind to a convention phrased
// in pure English, which is why the paragraph above still has to be read — but
// it catches the shape the drift actually arrives in, and it judges four files
// this repository controls, so a later failure is an author being told where
// the sentence belongs rather than a flake.
var repositoryArtefacts = []struct {
	name    string
	pattern *regexp.Regexp
}{
	{"a path", regexp.MustCompile(`[/\\]`)},
	{"a file name", regexp.MustCompile(`\w\.\w{2,4}\b`)},
	{"a code identifier", regexp.MustCompile(`\w_\w|[a-z][A-Z]`)},
	{"a command or an expression", regexp.MustCompile("\\(\\)|[`$*=]")},
}

// The prose is the highest-leverage text cr ships, because it is what every
// reviewing agent actually reads, and its two failure modes are opposite. It
// can say too much about a project, which §4.3.5 forbids and which this guard
// answers. It can also say too little about the register: §6.2 lets only a
// probe or an outside citation assert, §6.3 forces everything else to a
// question, and a lens that pushes towards assertion changes none of that — it
// only produces a reviewer who wrote as though certain and is then read as
// hedging, spending the trust §1.6 says is spent once.
func TestTheShippedRolesGiveALensAndNeverAStandard(t *testing.T) {
	for id, content := range Builtins() {
		t.Run(id, func(t *testing.T) {
			r, err := Parse(id+".json", []byte(content))
			require.NoError(t, err)
			prose := strings.Join(append([]string{r.Title, r.Instructions}, r.Focus...), "\n")

			for _, artefact := range repositoryArtefacts {
				assert.NotRegexp(t, artefact.pattern, prose,
					"%s names %s; §4.3.5 keeps a project's standards in the rule corpus of §2.6",
					id, artefact.name)
			}
		})
	}

	// The same requirement stated positively, on the one lens that would
	// carry the standards if any role did: the convention role has to send
	// its reader to the corpus instead of listing what the corpus holds.
	r, err := Parse(conventionID+".json", []byte(Builtins()[conventionID]))
	require.NoError(t, err)
	assert.Contains(t, r.Instructions, "rule",
		"the convention lens must point at the rules of §2.6, where a project's standards live")
}
