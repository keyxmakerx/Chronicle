package armory

import (
	"reflect"
	"testing"
)

func TestToCp(t *testing.T) {
	tests := []struct {
		name     string
		total    Cents
		currency string
		want     int64
		wantOK   bool
	}{
		{"copper", 700, "cp", 7, true},
		{"silver", 500, "sp", 50, true},
		{"electrum", 200, "ep", 100, true},
		{"gold", 1250, "gp", 1250, true},
		{"platinum", 100, "pp", 1000, true},
		{"case and spacing ignored", 500, " SP ", 50, true},
		{"half a copper rounds up", 50, "cp", 1, true},
		{"quarter silver rounds up", 25, "sp", 3, true},
		{"one hundredth of a gold is one copper", 1, "gp", 1, true},
		{"not a coin", 500, "credits", 0, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, ok := toCp(tt.total, tt.currency)
			if got != tt.want || ok != tt.wantOK {
				t.Fatalf("toCp(%d, %q) = %d, %v; want %d, %v", tt.total, tt.currency, got, ok, tt.want, tt.wantOK)
			}
		})
	}
}

func TestPayFromPurse(t *testing.T) {
	type m = map[string]int64
	tests := []struct {
		name       string
		purse      m
		price      int64
		wantOK     bool
		wantDeltas m
		wantChange m
	}{
		{"exact copper", m{"cp": 10, "gp": 5}, 10, true, m{"cp": -10}, m{}},
		{"smallest coins go first and change nets out", m{"cp": 3, "sp": 2, "gp": 10, "pp": 0}, 50, true,
			m{"sp": 5, "gp": -1}, m{"sp": 7, "cp": 3}},
		{"breaking one gold", m{"cp": 0, "sp": 0, "gp": 1}, 30, true, m{"gp": -1, "sp": 7}, m{"sp": 7}},
		{"breaking one platinum", m{"cp": 0, "sp": 0, "gp": 0, "pp": 1}, 1, true,
			m{"pp": -1, "gp": 9, "sp": 9, "cp": 9}, m{"gp": 9, "sp": 9, "cp": 9}},
		{"electrum is spent but never given back", m{"cp": 0, "sp": 0, "ep": 1, "gp": 0}, 30, true,
			m{"ep": -1, "sp": 2}, m{"sp": 2}},
		{"short by one copper", m{"cp": 0, "gp": 1}, 101, false, nil, nil},
		{"empty purse", m{"cp": 0, "sp": 0, "gp": 0}, 1, false, nil, nil},
		{"gold-only purse", m{"gp": 3}, 200, true, m{"gp": -2}, m{}},
		{"gold-only purse cannot make change below a gold", m{"gp": 3}, 150, true, m{"gp": -2}, m{}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			deltas, change, ok := payFromPurse(tt.purse, tt.price)
			if ok != tt.wantOK {
				t.Fatalf("ok = %v, want %v", ok, tt.wantOK)
			}
			if !ok {
				return
			}
			// Net result must be the purse minus the price plus the change.
			after := m{}
			for k, v := range tt.purse {
				after[k] = v + deltas[k]
			}
			// Change below the smallest coin the sheet has stays with the merchant,
			// so that one case is allowed to lose value.
			if tt.name != "gold-only purse cannot make change below a gold" {
				if got := purseValueCp(tt.purse) - purseValueCp(after); got != tt.price {
					t.Errorf("purse lost %d cp, want %d", got, tt.price)
				}
			}
			for k, v := range after {
				if v < 0 {
					t.Errorf("coin %s went negative: %d", k, v)
				}
			}
			if change["ep"] != 0 || change["pp"] != 0 {
				t.Errorf("change contains ep/pp: %v", change)
			}
			if !reflect.DeepEqual(deltas, tt.wantDeltas) {
				t.Errorf("deltas = %v, want %v", deltas, tt.wantDeltas)
			}
			if !reflect.DeepEqual(change, tt.wantChange) {
				t.Errorf("change = %v, want %v", change, tt.wantChange)
			}
		})
	}
}

func TestFormatPurse(t *testing.T) {
	tests := []struct {
		name  string
		purse map[string]int64
		want  string
	}{
		{"mixed", map[string]int64{"cp": 3, "sp": 7, "gp": 9}, "9 gp 7 sp 3 cp"},
		{"all coins", map[string]int64{"pp": 1, "gp": 2, "ep": 3, "sp": 4, "cp": 5}, "1 pp 2 gp 3 ep 4 sp 5 cp"},
		{"zeros skipped", map[string]int64{"gp": 4, "sp": 0}, "4 gp"},
		{"empty", map[string]int64{}, "0 gp"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := formatPurse(tt.purse); got != tt.want {
				t.Fatalf("formatPurse = %q, want %q", got, tt.want)
			}
		})
	}
}
