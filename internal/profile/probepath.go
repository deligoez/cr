package profile

import (
	"fmt"
	"regexp"
	"strings"
)

// The placeholder vocabulary of `tests.probe_path_template`. §2.4 closes it to
// these two: a token cr does not substitute reaches the sandbox literally, and
// §5.1.6's leftover scan — which replaces `<probe-id>` with `*` — would then
// hunt for a file no probe ever writes.
const (
	// probeIDPlaceholder stands for the probe id. §5.1.6 requires exactly
	// one occurrence, because it recognises a leftover artefact by
	// replacing that one position.
	probeIDPlaceholder = "<probe-id>"
	// extPlaceholder stands for the test file suffix, derived from
	// `tests.globs` and never from a supplied test file.
	extPlaceholder = "<ext>"
	// targetDirPlaceholder stands for the directory of the path in
	// `--target`, and may only be the template's first segment.
	//
	// It exists because a language that compiles a test into the package it
	// tests has no one directory that serves every probe. Measured in
	// measurement 4 part B: cr placed a `package probe` test at
	// `internal/cli/cr_probe_p2_test.go`, the one path the Go profile's
	// template could give, and `go test` exited 1 on `undefined: Target`
	// without running a test. No value of a whole-fixed template fixes
	// that, because the package differs per probe.
	targetDirPlaceholder = "<target-dir>"
)

// wildcards are the three glob metacharacters §2.4 names when it defines a
// template's root.
const wildcards = "*?["

// probeTemplateField is the offending field in the dotted §2.4 spelling every
// MalformedError from this file carries.
const probeTemplateField = "tests.probe_path_template"

// placeholderToken matches every `<...>` token in a template, so an unknown one
// is named at parse time rather than written into the sandbox.
var placeholderToken = regexp.MustCompile(`<[^<>]*>`)

// ProbePath returns the sandbox-relative path where `cr probe run --kind gap`
// places its test file, per §5.4.2. The template resolved at parse time already
// carries its `<ext>`, so the probe id is the only substitution left. A profile
// with no `tests.globs` has no test axis and therefore no probe path: it
// resolves to the empty template and returns the empty path.
// targetDir is the directory of the path in `--target`, slash-separated and
// empty when the target sits at the repository root. A template with no
// `<target-dir>` ignores it.
func (p *Profile) ProbePath(probeID, targetDir string) string {
	placed := strings.Replace(p.Tests.ProbePathTemplate, probeIDPlaceholder, probeID, 1)
	if !strings.Contains(placed, targetDirPlaceholder) {
		return placed
	}
	// The placeholder is the first segment, so an empty target directory
	// leaves the file at the root rather than under a leading separator.
	if targetDir == "" || targetDir == "." {
		return strings.TrimPrefix(placed, targetDirPlaceholder+"/")
	}
	return strings.Replace(placed, targetDirPlaceholder, targetDir, 1)
}

// TakesTargetDir reports whether the resolved template places its file under
// the directory of the probe's target, rather than at one path per profile.
//
// It is what a caller asks before it has a target in hand — `cr proposals
// record` checks that a gap proposal can be placed at all, and §5.1.6 scans a
// sandbox with no probe running.
func (p *Profile) TakesTargetDir() bool {
	return strings.Contains(p.Tests.ProbePathTemplate, targetDirPlaceholder)
}

// LeftoverGlob returns the glob §5.1.6 scans for a leftover probe artefact: the
// resolved template with every placeholder substituted except `<probe-id>`,
// which becomes `*`. A profile with no test axis returns the empty string, and a
// caller MUST read that as "nothing to scan" rather than as a pattern.
func (p *Profile) LeftoverGlob() string {
	glob := strings.Replace(p.Tests.ProbePathTemplate, probeIDPlaceholder, "*", 1)
	// `<target-dir>` stands for a directory of any depth, so the scan that
	// reads this glob has to look under all of them. `**` says so; the
	// caller is what decides how to walk it.
	return strings.Replace(glob, targetDirPlaceholder, "**", 1)
}

// probeTemplate returns the effective §2.4 probe path template: the profile's
// own when it sets one, the documented default otherwise, with `<ext>` already
// substituted so that `<probe-id>` is the single placeholder left standing. A
// template cr cannot resolve is a malformed profile, named by file and field so
// the abort of §2.5 item 3 says what to open and what to fix.
func (t *wireTests) probeTemplate(file string) (string, error) {
	if t == nil {
		return "", nil
	}
	ext, unresolvable := probeExt(t.Globs)
	template := t.ProbePathTemplate
	if template == "" {
		if len(t.Globs) == 0 {
			// No test globs means no test axis, so there is no probe
			// file to place and nothing to default.
			return "", nil
		}
		if unresolvable != "" {
			return "", &MalformedError{
				File:    file,
				Field:   probeTemplateField,
				Problem: fmt.Sprintf("cannot be defaulted because %s is unresolvable: %s; set the template explicitly", extPlaceholder, unresolvable),
			}
		}
		template = defaultProbeTemplate(probeRoot(t.Globs[0]), ext)
	}
	for _, token := range placeholderToken.FindAllString(template, -1) {
		if token != probeIDPlaceholder && token != extPlaceholder && token != targetDirPlaceholder {
			return "", &MalformedError{
				File:    file,
				Field:   probeTemplateField,
				Problem: fmt.Sprintf("uses %s, but §2.4's placeholder vocabulary is closed to %s, %s and %s", token, probeIDPlaceholder, extPlaceholder, targetDirPlaceholder),
			}
		}
	}
	if n := strings.Count(template, targetDirPlaceholder); n > 1 ||
		(n == 1 && !strings.HasPrefix(template, targetDirPlaceholder+"/")) {
		return "", &MalformedError{
			File:  file,
			Field: probeTemplateField,
			Problem: fmt.Sprintf(
				"uses %s outside the template's first segment; §5.4.2 resolves it to the "+
					"directory of --target, so it stands for the whole leading directory and "+
					"nothing else", targetDirPlaceholder),
		}
	}
	if n := strings.Count(template, probeIDPlaceholder); n != 1 {
		return "", &MalformedError{
			File:    file,
			Field:   probeTemplateField,
			Problem: fmt.Sprintf("contains %s %d times; §5.1.6 recognises a leftover artefact by replacing exactly one", probeIDPlaceholder, n),
		}
	}
	if strings.Contains(template, extPlaceholder) {
		if unresolvable != "" {
			return "", &MalformedError{
				File:    file,
				Field:   probeTemplateField,
				Problem: fmt.Sprintf("uses %s, which is unresolvable: %s", extPlaceholder, unresolvable),
			}
		}
		template = strings.ReplaceAll(template, extPlaceholder, ext)
	}
	return template, nil
}

// defaultProbeTemplate builds §2.4's default, `<root>/cr_probe_<probe-id><ext>`,
// dropping the separator for a glob whose literal prefix is empty.
func defaultProbeTemplate(root, ext string) string {
	name := "cr_probe_" + probeIDPlaceholder + ext
	if root == "" {
		return name
	}
	return root + "/" + name
}

// probeRoot returns the longest leading path prefix of glob free of `*`, `?`,
// and `[`. It is a path prefix, so it ends on a segment boundary:
// `tests/Feature*/*.php` roots at `tests`, never at `tests/Feature`.
func probeRoot(glob string) string {
	segments := strings.Split(glob, "/")
	literal := 0
	for _, segment := range segments {
		if strings.ContainsAny(segment, wildcards) {
			break
		}
		literal++
	}
	return strings.Join(segments[:literal], "/")
}

// probeExt returns `<ext>` and, when it cannot be resolved, the reason. `<ext>`
// is the literal suffix following the last wildcard segment of the first
// `tests.globs` entry, and is empty when that entry carries no wildcard at all.
//
// It is derived from the profile alone and never from a supplied test file,
// because §5.1.6's cleanliness scan resolves the template with no test file in
// hand. A suffix crossing a path separator is no suffix of the glob's final
// segment and names no extension, so it is unresolvable rather than silently
// wrong. A `[...]` class ends its segment at the opening bracket, which is what
// the explicit template each shipped profile sets is for.
func probeExt(globs []string) (ext, unresolvable string) {
	if len(globs) == 0 {
		return "", "tests.globs is empty, so no entry seeds it"
	}
	glob := globs[0]
	last := strings.LastIndexAny(glob, wildcards)
	if last < 0 {
		return "", ""
	}
	suffix := glob[last+1:]
	if strings.Contains(suffix, "/") {
		return "", fmt.Sprintf("the last wildcard of tests.globs %q is followed by a path separator, so no suffix of its final segment follows it", glob)
	}
	return suffix, ""
}
