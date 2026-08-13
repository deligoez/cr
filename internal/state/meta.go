package state

import (
	"encoding/json"
	"fmt"
)

// Meta is meta.json: the header of one pull request's state directory, holding
// the §2.3 table's PR identity, issue key, profile id, active roles, round
// index, recorded head, and post_unresolved.
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
		return Meta{}, fmt.Errorf("cannot read %s: %w", l.PRFile(owner, repo, pr, FileMeta), err)
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
