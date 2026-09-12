package state

import (
	"encoding/json"
	"fmt"
)

// Meta is meta.json: the header of one pull request's state directory, holding
// the §2.3 table's PR identity, issue key, profile id, active roles, round
// index, recorded head, and post_unresolved, beside the mapping stamp `cr map
// record` leaves.
type Meta struct {
	// Owner, Repo, and PR are the pull request's identity. They repeat the
	// directory the file sits in, so a state directory copied or reported
	// out of place still says which pull request it belongs to.
	Owner string `json:"owner"`
	Repo  string `json:"repo"`
	PR    int    `json:"pr"`
	// IssueKey names the tracker issue that is the specification (P1).
	IssueKey string `json:"issue_key"`
	// ProfileID is the mechanical profile of §2.4 the round resolved to.
	ProfileID string `json:"profile_id"`
	// ActiveRoles are the ids of the roles reviewing this pull request.
	ActiveRoles []string `json:"active_roles"`
	// Round is the round index. §9.3.3 starts the count at 1, so the zero
	// this file is created with is a directory no round has been opened in
	// rather than a round of its own.
	Round int `json:"round"`
	// Head is the head recorded for that round, which §9.3.1 compares the
	// current head against.
	Head string `json:"head"`
	// PostUnresolved records a post whose outcome cr never learned (§8.4.4).
	PostUnresolved bool `json:"post_unresolved"`
	// MappingRound and MappingHead are the round and head `cr map record`
	// last stored §4.1.6's mapping for, and zero and empty before it has.
	//
	// They are round 9's mapping-existence-unobservable and round 12's
	// empty-vs-absent-mapping: an empty mapping, every unit mapped to zero
	// claims, leaves mapping.ndjson byte-identical to one nobody recorded,
	// so the existence §4.6.5's gate asks about is recorded here rather
	// than inferred from the file.
	MappingRound int    `json:"mapping_round"`
	MappingHead  string `json:"mapping_head"`
}

// MappingRecorded reports whether meta.json's mapping stamp names the round and
// head it records, which is §4.6.5's "a mapping exists for the current round
// and head". A stamp left by an earlier round names a round this one is not.
func (m *Meta) MappingRecorded() bool {
	return m.Round > 0 && m.MappingRound == m.Round && m.MappingHead == m.Head
}

// newMeta is the meta.json a freshly created state directory carries: the
// identity cr already knows, and nothing a round would have had to establish.
func newMeta(owner, repo string, pr int) *Meta {
	return &Meta{Owner: owner, Repo: repo, PR: pr, ActiveRoles: []string{}}
}

// ReadMeta reads meta.json. It takes no lock, per §2.3.2.
func (l Layout) ReadMeta(owner, repo string, pr int) (Meta, error) {
	body, err := l.ReadPR(owner, repo, pr, FileMeta)
	if err != nil {
		return Meta{}, err
	}
	var m Meta
	if err := json.Unmarshal(body, &m); err != nil {
		return Meta{}, FileFailure(
			"read", l.PRFile(owner, repo, pr, FileMeta), UnusableHint, err)
	}
	m.ActiveRoles = roleList(m.ActiveRoles)
	return m, nil
}

// WriteMeta publishes meta.json through the held lock. It reads m and never
// writes to it: the normalising encodeMeta does happens on a copy.
func (k *Lock) WriteMeta(m *Meta) error {
	body, err := encodeMeta(m)
	if err != nil {
		return err
	}
	return k.Write(FileMeta, body)
}

// encodeMeta renders meta.json: pretty-printed with two-space indentation, and
// newline-terminated so the file is a well-formed text file.
func encodeMeta(m *Meta) ([]byte, error) {
	out := *m
	out.ActiveRoles = roleList(m.ActiveRoles)
	body, err := json.MarshalIndent(out, "", "  ")
	if err != nil {
		return nil, fmt.Errorf("cannot encode %s: %w", FileMeta, err)
	}
	return append(body, '\n'), nil
}

// roleList copies a role list and turns an absent one into an empty one, so
// meta.json never serialises active_roles as null (§12.3).
func roleList(roles []string) []string {
	return append(make([]string, 0, len(roles)), roles...)
}
