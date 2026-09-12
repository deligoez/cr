package brief

import "fmt"

// KeyRewriteError reports a `cr brief` that would replace the issue key a round
// already recorded.
//
// §11.2 codes it 4. Nothing about the invocation is malformed and every file it
// named was read; what refuses is that the pull request's recorded state and
// the key §3.2 just resolved disagree, which is where the round stands rather
// than what the user typed. The two ways forward are both named, because cr has
// no command that re-keys a round: name the recorded key with `--issue`, or
// remove the pull request's state directory and start it over under the new key.
type KeyRewriteError struct {
	// Owner, Repo, and PR name the pull request, so the hint is runnable.
	Owner string
	Repo  string
	PR    int
	// Recorded is the key meta.json holds, and Resolved the key §3.2
	// resolved this time. Resolved is empty when nothing matched, which
	// is the case the dogfood run found.
	Recorded string
	Resolved string
	// Pattern is `intent.key_pattern` as it now reads, because a rewrite
	// to no key at all is almost always this setting having changed.
	Pattern string
	// StateDir is the pull request's directory under §2.2's state root,
	// which is the only place the recorded key can be discarded from.
	StateDir string
}

func (e *KeyRewriteError) Error() string {
	return fmt.Sprintf(
		"round of pull request %d is recorded under issue key %s and §3.2 resolved %s "+
			"from intent.key_pattern %q: every claim of that round is recorded as %s#c<n> "+
			"per §3.3, and cr has no command that re-keys them; run "+
			"`cr brief %d --repo %s/%s --issue %s` to keep the recorded key, or remove %s "+
			"to start this pull request over under the new one",
		e.PR, e.Recorded, e.resolved(), e.Pattern, e.Recorded,
		e.PR, e.Owner, e.Repo, e.Recorded, e.StateDir)
}

// resolved names the key that would have replaced it, and says plainly when
// there is none. "resolved " followed by nothing reads as a truncated message,
// and no key at all is the case worth being clearest about: it is what a
// changed `intent.key_pattern` produces.
func (e *KeyRewriteError) resolved() string {
	if e.Resolved == "" {
		return "no key at all"
	}
	return e.Resolved
}
