package rolls

import (
	"crypto/rand"
	"math/big"
	"strconv"
	"strings"
)

// tableSides are the dice the Table roller offers.
var tableSides = map[int]bool{2: true, 3: true, 4: true, 6: true, 8: true, 10: true, 12: true, 20: true, 100: true}

// term is one part of a formula: count dice of sides, or a constant when
// sides is 0. sign is +1 or -1.
type term struct {
	sign, count, sides int
}

// parseFormula reads a formula like "2d10+2", "d20" or "4d6-1". It returns
// the terms and the formula as it will be shown, or ok=false when the formula
// is not one the roller can make. Dice are only ever added, never taken away.
func parseFormula(src string) (terms []term, shown string, ok bool) {
	s := strings.ToLower(strings.Join(strings.Fields(src), ""))
	if s == "" || len(s) > FormulaMax {
		return nil, "", false
	}
	dice := 0
	var b strings.Builder
	for i := 0; i < len(s); {
		sign := 1
		switch s[i] {
		case '+':
			i++
		case '-':
			sign = -1
			i++
		default:
			if i > 0 {
				return nil, "", false
			}
		}
		n, j := digits(s, i)
		if j < len(s) && s[j] == 'd' {
			count := 1
			if j > i {
				count = n
			}
			sides, k := digits(s, j+1)
			if k == j+1 || count < 1 || sign < 0 || !tableSides[sides] {
				return nil, "", false
			}
			dice += count
			if dice > tableDiceMax {
				return nil, "", false
			}
			terms = append(terms, term{sign: 1, count: count, sides: sides})
			if b.Len() > 0 {
				b.WriteByte('+')
			}
			b.WriteString(strconv.Itoa(count) + "d" + strconv.Itoa(sides))
			i = k
		} else {
			if j == i || n > MaxModifier {
				return nil, "", false
			}
			terms = append(terms, term{sign: sign, count: n})
			if sign < 0 {
				b.WriteByte('-')
			} else if b.Len() > 0 {
				b.WriteByte('+')
			}
			b.WriteString(strconv.Itoa(n))
			i = j
		}
		if len(terms) > tableTermsMax {
			return nil, "", false
		}
	}
	if dice == 0 {
		return nil, "", false
	}
	return terms, b.String(), true
}

// digits reads the run of digits at s[i:], returning its value and where it
// ends; a run too long to be a sensible number reads as out of range.
func digits(s string, i int) (int, int) {
	j := i
	for j < len(s) && s[j] >= '0' && s[j] <= '9' {
		j++
	}
	if j == i {
		return 0, j
	}
	if j-i > 6 {
		return MaxModifier + 1, j
	}
	n, _ := strconv.Atoi(s[i:j])
	return n, j
}

// cryptoRoll is one die of n sides from the system's secure random source.
// A failed read rolls a 1 rather than failing the roll.
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

// throw rolls every die of the terms and adds the constants.
func throw(terms []term, roll func(int) int) (dice []Die, modifier, total int) {
	for _, t := range terms {
		if t.sides == 0 {
			modifier += t.sign * t.count
			continue
		}
		for i := 0; i < t.count; i++ {
			v := roll(t.sides)
			dice = append(dice, Die{Sides: t.sides, Value: v})
			total += v
		}
	}
	return dice, modifier, total + modifier
}
