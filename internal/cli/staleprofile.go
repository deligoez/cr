package cli

import "github.com/deligoez/cr/internal/profile"

// staleProfile is the honesty sentence owed for the profile file at file when
// it is, byte for byte, a profile an earlier release shipped, and empty for
// every other file (profile.Profile.StaleDisclosures says which).
//
// It is for a command that names the round's profile without holding the value
// it loaded — the loaded profile is several calls down, or read only on some
// paths — so it reads the file again rather than threading the value up. A file
// that does not load is answered with nothing: the command's own load reports
// that failure, or never needed the profile, and a disclosure is not the place
// to refuse a run.
func staleProfile(file string) []string {
	loaded, err := profile.Load(file)
	if err != nil {
		return []string{}
	}
	return loaded.StaleDisclosures()
}
