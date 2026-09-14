package coverage

import "github.com/deligoez/cr/internal/role"

// Decode is DecodeInRound for a round whose remaining axes wait for nothing and
// which holds no context store, the round most of this package's tests decode
// against. It lives beside the tests rather than in cell.go because
// `cr cells record` always asks DecodeInRound about the round's standing, so a
// production wrapper without it would be code no command reaches.
func Decode(file string, body []byte, units []string, active []role.Role, raised Raised) ([]*Cell, error) {
	return DecodeInRound(file, body, units, active, raised, nil, nil)
}
