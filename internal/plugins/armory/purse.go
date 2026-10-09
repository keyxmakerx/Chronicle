// purse.go holds the D&D 5e coin arithmetic for shop purchases: converting a
// listed price into copper, choosing which coins pay it, and making change.
// Everything here is pure so the rounding rules are pinned by table tests.
package armory

import (
	"fmt"
	"strings"
)

// coinRateCp is each 5e coin's value in copper.
var coinRateCp = map[string]int64{"cp": 1, "sp": 10, "ep": 50, "gp": 100, "pp": 1000}

// spendOrder is the order coins are spent in: smallest first, so a player's
// big coins are only broken when the small ones run out.
var spendOrder = []string{"cp", "sp", "ep", "gp", "pp"}

// changeOrder is the order change is made in. Electrum and platinum are never
// handed back: ep is a coin few tables use, and breaking a pp into pp change
// would only move the same value around.
var changeOrder = []string{"gp", "sp", "cp"}

// displayOrder lists a purse the way a sheet reads, largest first.
var displayOrder = []string{"pp", "gp", "ep", "sp", "cp"}

// toCp converts an amount of cents in the listing currency to copper, rounding
// up so a merchant never sells for a fraction of a copper. It reports false
// when the currency is not a 5e coin.
func toCp(total Cents, currency string) (int64, bool) {
	rate, ok := coinRateCp[strings.ToLower(strings.TrimSpace(currency))]
	if !ok {
		return 0, false
	}
	n := int64(total) * rate
	return (n + 99) / 100, true
}

// purseValueCp is the copper value of whole-coin counts. Coins outside the
// rate table are ignored.
func purseValueCp(purse map[string]int64) int64 {
	var v int64
	for coin, n := range purse {
		v += n * coinRateCp[coin]
	}
	return v
}

// payFromPurse chooses the coins that cover priceCp. purse holds whole-coin
// counts and lists every coin the sheet has (a held count of zero still marks
// the coin as available for change).
//
// It returns the net change to each coin (spent coins negative, change
// positive), the change handed back as coin counts, and whether the purse
// could pay. Change is made only in coins the sheet has; a remainder smaller
// than the smallest of those stays with the merchant.
func payFromPurse(purse map[string]int64, priceCp int64) (deltas, change map[string]int64, ok bool) {
	deltas, change = map[string]int64{}, map[string]int64{}
	remaining := priceCp
	for _, coin := range spendOrder {
		held, has := purse[coin]
		if !has || held <= 0 || remaining <= 0 {
			continue
		}
		rate := coinRateCp[coin]
		use := (remaining + rate - 1) / rate
		if use > held {
			use = held
		}
		deltas[coin] -= use
		remaining -= use * rate
	}
	if remaining > 0 {
		return nil, nil, false
	}
	back := -remaining
	for _, coin := range changeOrder {
		if _, has := purse[coin]; !has {
			continue
		}
		n := back / coinRateCp[coin]
		if n == 0 {
			continue
		}
		change[coin] = n
		deltas[coin] += n
		back -= n * coinRateCp[coin]
	}
	for coin, d := range deltas {
		if d == 0 {
			delete(deltas, coin)
		}
	}
	return deltas, change, true
}

// formatPurse writes coin counts as "9 gp 7 sp 3 cp", largest first, skipping
// empty coins.
func formatPurse(purse map[string]int64) string {
	var parts []string
	for _, coin := range displayOrder {
		if n := purse[coin]; n > 0 {
			parts = append(parts, fmt.Sprintf("%d %s", n, coin))
		}
	}
	if len(parts) == 0 {
		return "0 gp"
	}
	return strings.Join(parts, " ")
}

// coinCounts reads each coin field of a purse sheet: whole-coin counts, and the
// raw cents behind them so a fractional count (2.5 gp) survives a write. A
// negative field reads as empty; a non-number is an error.
func coinCounts(purse map[string]string, fields map[string]any) (counts map[string]int64, cents map[string]Cents, err error) {
	counts, cents = make(map[string]int64, len(purse)), make(map[string]Cents, len(purse))
	for coin, key := range purse {
		c, err := toCents(fields[key])
		if err != nil {
			return nil, nil, err
		}
		if c < 0 {
			c = 0
		}
		cents[coin] = c
		counts[coin] = int64(c) / 100
	}
	return counts, cents, nil
}
