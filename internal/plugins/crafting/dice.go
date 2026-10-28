package crafting

import (
	"crypto/rand"
	"math/big"
)

// cryptoRoll is one die of n sides, rolled on the server so a player's
// browser never decides a project roll. A failed read of the system's
// random source rolls a 1 rather than failing the roll.
func cryptoRoll(n int) int {
	if n < 2 {
		return 1
	}
	v, err := rand.Int(rand.Reader, big.NewInt(int64(n)))
	if err != nil {
		return 1
	}
	return int(v.Int64()) + 1
}
