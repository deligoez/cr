package profile

import _ "embed"

// laravelPestID is the id, and therefore the file stem, of the profile §2.4.5
// requires v0.1 to ship for a Laravel repository whose suite is Pest.
const laravelPestID = "laravel-pest"

// laravelPest is the shipped profile file, embedded rather than built from a
// struct literal so what cr writes is the file itself, byte for byte. §2.5.2
// asks exactly that of the built-in roles, and a profile written from a literal
// could not answer it: the bytes would be whatever the JSON encoder chose that
// day, not the reviewed file in this repository.
//
//go:embed builtin/laravel-pest.json
var laravelPest string

// Builtins returns the profile files cr ships, keyed by profile id, for `cr
// init` to write into the profiles directory of §2.2. The values are file
// contents, not parsed profiles, because writing them out is the whole purpose
// and a round trip through Profile would not reproduce them.
//
// The map is rebuilt per call so no caller can edit the shipped set.
func Builtins() map[string]string {
	return map[string]string{
		laravelPestID: laravelPest,
	}
}
