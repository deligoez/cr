package cli

import "runtime/debug"

// devVersion is what `cr --version` prints for a build that knows no version:
// a development marker, never an empty string a reader would take for a
// missing answer.
const devVersion = "dev"

// develModule is the main module version the Go toolchain records for a build
// it could not stamp from a tag or a commit.
const develModule = "(devel)"

// versionOf is §14.6's answer: the version `cr --version` prints.
//
// The version a release build injects with -X wins. `go install
// github.com/deligoez/cr/cmd/cr@<tag>` applies no ldflags, so §14.4's install
// would otherwise print the marker; the toolchain records the module version it
// installed in the binary's build information, and that is read next. Measured
// on go1.26.5: a `go install` from a clean checkout tagged v0.9.9 records mod
// version v0.9.9, and one from an untagged commit records a pseudo-version,
// which is still the version that was built. Only when neither says anything
// is the development marker printed.
func versionOf(injected string, read func() (*debug.BuildInfo, bool)) string {
	if injected != "" {
		return injected
	}
	if info, ok := read(); ok && info.Main.Version != "" && info.Main.Version != develModule {
		return info.Main.Version
	}
	return devVersion
}
