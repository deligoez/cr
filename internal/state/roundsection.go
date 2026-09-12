package state

import (
	"encoding/json"
	"errors"
	"io/fs"
)

// ReadRoundSection reads one named field of one round's JSON artefact, and
// reports whether the document holds that field at all.
//
// It is UpdateRoundSection's reader, and the second return value is why it is
// a function rather than a decode at the call site. §10.3 has `cr merge`,
// `cr draft` and `cr post` each accumulate their own counts into one
// summary.json, so a section that is not there is a command that has not run
// for this round — which is a different fact from a count of zero, and §10.1.6
// has to report the two differently. A reader decoding the whole document into
// a struct of its own fields could not tell them apart: an absent field and a
// zero one decode alike.
//
// The document is therefore decoded as raw fields, which is the reading
// UpdateRoundSection already writes it back as: a field this version does not
// understand is neither read nor disturbed.
//
// A round nothing has written yet has no artefact at all, which holds exactly
// as many sections as an empty document does — the reading storeRecords already
// gives an absent store.
//
// It takes no lock, per §2.3.2.
func ReadRoundSection[T any](
	l Layout, owner, repo string, pr, round int, name, section string,
) (value T, recorded bool, err error) {
	if err := checkRoundFile(name); err != nil {
		return value, false, err
	}
	body, err := l.ReadRound(owner, repo, pr, round, name)
	if errors.Is(err, fs.ErrNotExist) {
		return value, false, nil
	}
	if err != nil {
		return value, false, err
	}
	path := l.RoundFile(owner, repo, pr, round, name)
	var document map[string]json.RawMessage
	if err := json.Unmarshal(body, &document); err != nil {
		return value, false, FileFailure("read", path, UnusableHint, err)
	}
	held, recorded := document[section]
	if !recorded {
		return value, false, nil
	}
	if err := json.Unmarshal(held, &value); err != nil {
		return value, false, FileFailure("read the "+section+" section of", path, UnusableHint, err)
	}
	return value, true, nil
}
