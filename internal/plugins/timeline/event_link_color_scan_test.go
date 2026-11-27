package timeline

import (
	"testing"
)

// fakeLinkRow scans fixed values into scanEventLinkOwn's destinations.
type fakeLinkRow struct{ color *string }

func (r fakeLinkRow) Scan(dest ...any) error {
	*(dest[0].(*int)) = 1
	*(dest[1].(*string)) = "tl-1"
	*(dest[2].(*string)) = "evt-1"
	*(dest[7].(**string)) = r.color
	return nil
}

// A colour stored before LinkEvent checked it must never reach a reader.
func TestScanEventLinkOwn_DropsStoredNonHexColor(t *testing.T) {
	str := func(s string) *string { return &s }
	cases := []struct {
		name  string
		color *string
		want  *string
	}{
		{"markup", str(`red"><img src=x onerror=alert(1)>`), nil},
		{"not hex", str("blue"), nil},
		{"hex", str("#a1B2c3"), str("#a1B2c3")},
		{"short hex", str("#abc"), str("#abc")},
		{"none", nil, nil},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			el, err := scanEventLinkOwn(fakeLinkRow{color: tc.color})
			if err != nil {
				t.Fatal(err)
			}
			if (el.ColorOverride == nil) != (tc.want == nil) || (tc.want != nil && *el.ColorOverride != *tc.want) {
				t.Errorf("got %v, want %v", el.ColorOverride, tc.want)
			}
		})
	}
}

func TestIsHexColor(t *testing.T) {
	for s, want := range map[string]bool{"#abc": true, "#A1B2C3": true, "#abcd": false, "red": false, "": false, `#abc"><b>`: false} {
		if got := IsHexColor(s); got != want {
			t.Errorf("IsHexColor(%q) = %v, want %v", s, got, want)
		}
	}
}
