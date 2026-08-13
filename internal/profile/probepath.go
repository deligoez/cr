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
func (p *Profile) ProbePath(probeID string) string {
	return strings.Replace(p.Tests.ProbePathTemplate, probeIDPlaceholder, probeID, 1)
}

// LeftoverGlob returns the glob §5.1.6 scans for a leftover probe artefact: the
// resolved template with every placeholder substituted except `<probe-id>`,
// which becomes `*`. A profile with no test axis returns the empty string, and a
// caller MUST read that as "nothing to scan" rather than as a pattern.
func (p *Profile) LeftoverGlob() string {
	return strings.Replace(p.Tests.ProbePathTemplate, probeIDPlaceholder, "*", 1)
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
		if token != probeIDPlaceholder && token != extPlaceholder {
			return "", &MalformedError{
				File:    file,
				Field:   probeTemplateField,
				Problem: fmt.Sprintf("uses %s, but §2.4's placeholder vocabulary is closed to %s and %s", token, probeIDPlaceholder, extPlaceholder),
			}
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
