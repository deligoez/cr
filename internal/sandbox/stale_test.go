package sandbox

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// placed writes body at name under dir, and writes nothing for a nil body, so a
// case can say which side holds an entry by giving it bytes or not.
func placed(t *testing.T, dir, name string, body []byte) string {
	t.Helper()
	path := filepath.Join(dir, name)
	if body != nil {
		require.NoError(t, os.WriteFile(path, body, 0o600))
	}
	return path
}

// §5.1.2's copied file stands only when the sandbox holds the checkout's own
// bytes. A file of the same size with other bytes is a copy that differs, and
// the same bytes are no difference at all — including across the block the
// comparison reads at a time, so the second file's buffer is as large as the
// first's. An entry only one side holds is absent or left over, and one neither
// side holds is not compared.
func TestACopiedFileStandsOnlyWhenTheSandboxHoldsTheCheckoutsBytes(t *testing.T) {
	large := make([]byte, 80*1024)
	for i := range large {
		large[i] = byte('a' + i%26)
	}
	for name, tc := range map[string]struct {
		checkout, sandbox []byte
		want              string
	}{
		"the same bytes":              {[]byte("APP_ENV=testing\n"), []byte("APP_ENV=testing\n"), ""},
		"the same bytes past a block": {large, large, ""},
		"the same size, other bytes":  {[]byte("APP_ENV=testing\n"), []byte("APP_ENV=staging\n"), copyDiffers},
		"another size":                {[]byte("APP_ENV=testing\n"), []byte("APP_ENV=\n"), copyDiffers},
		"absent from the sandbox":     {[]byte("APP_ENV=testing\n"), nil, copyAbsent},
		"left in the sandbox":         {nil, []byte("APP_ENV=testing\n"), copyLeftover},
		"held by neither":             {nil, nil, ""},
	} {
		t.Run(name, func(t *testing.T) {
			source := placed(t, t.TempDir(), ".env.testing", tc.checkout)
			target := placed(t, t.TempDir(), ".env.testing", tc.sandbox)

			found, err := copyStanding(source, target)

			require.NoError(t, err)
			assert.Equal(t, tc.want, found)
		})
	}
}
