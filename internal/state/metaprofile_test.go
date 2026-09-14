package state

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// §2.4 makes a profile's id its file stem, so the profile_id meta.json records
// is refused when it is not one: a separator of either spelling, . or .., or a
// rooted path. An ordinary id, one with a dot inside it, and the empty id of a
// directory no round has resolved a profile for are all read.
func TestMetaReadsOnlyAProfileIDThatIsOneFileStem(t *testing.T) {
	cases := []struct {
		id      string
		refused bool
	}{
		{"", false},
		{"laravel-pest", false},
		{"go.v2", false},
		{"..x", false},
		{".", true},
		{"..", true},
		{"../../x", true},
		{"a/b", true},
		{`a\b`, true},
		{"/etc/x", true},
	}
	for _, c := range cases {
		t.Run(c.id, func(t *testing.T) {
			path := "/state/meta.json"
			m, err := decodeMeta([]byte(`{"profile_id":`+quote(c.id)+`}`), path)
			if !c.refused {
				require.NoError(t, err)
				assert.Equal(t, c.id, m.ProfileID)
				return
			}
			var file *FileError
			require.ErrorAs(t, err, &file)
			assert.Equal(t, UnusableHint, file.Hint())
			assert.Equal(t, "cannot use /state/meta.json: profile_id "+quote(c.id)+
				" is not one profile's file stem: §2.4 makes the id the name of a file directly under "+
				"~/.cr/profiles, so it holds no path separator and is not . or ..", err.Error())
		})
	}
}

// quote spells an id as a JSON and a %q string both do for these cases.
func quote(id string) string {
	out := []byte{'"'}
	for i := range len(id) {
		if id[i] == '\\' {
			out = append(out, '\\')
		}
		out = append(out, id[i])
	}
	return string(append(out, '"'))
}
