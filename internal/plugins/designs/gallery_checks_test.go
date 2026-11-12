package designs

import (
	"strings"
	"testing"
)

// The cases that must be flagged are listed by index in failure messages so a
// test run never prints a slur.
func TestMaturityFlags(t *testing.T) {
	flagged := []string{
		"free porn here", "S.E.X room", "s e x", "s.e.x", "sh1t counter", "FUUUUCK", "f u c k", "p0rn",
		"nüde", "n u d e", "a $lut", "s-l-u-t", "blow job", "blowjob", "bl0w j0b", "whore house", "ass-hole", "a s s h o l e",
		"The Sexy Counter", "hentai", "N.S.F.W", "fuuuuuuuck", "rap3", "dick pic", "motherfucker",
	}
	for i, in := range flagged {
		if !defaultMaturity.flags(in) {
			t.Errorf("flagged case %d was not flagged", i)
		}
	}
}

// Scunthorpe and friends: ordinary words and names must pass.
func TestMaturityAllowsOrdinaryText(t *testing.T) {
	clean := []string{
		"Assassin's Guild", "Scunthorpe", "Cockatrice Nest", "Dick the Smith", "Analysis Desk", "The Therapist",
		"Grape Arbor", "Essex Cellar", "Sussex Bench", "Middlesex Shop", "Peacock Room", "Cocktail Hour", "Classic Oak",
		"Grapevine Ward", "Bass Drum", "Massive Hall", "Open island shop", "Pen is mightier", "Shiitake Stall", "Sextant Corner",
		"Moonsilver Seven-Star", "Pilgrim's Folding Bench", "The Gilded Kettle", "Rooster at dawn", "a b", "I a m here",
		"Room 101", "Café Noir", "Pass the salt", "Mississippi", "Cumberland Market", "Skeletons and Circles", "",
	}
	for i, in := range clean {
		if defaultMaturity.flags(in) {
			t.Errorf("clean case %d (%q) was flagged", i, in)
		}
	}
}

// Every listed term must catch itself, so a typo in the list file shows up.
func TestEveryListedTermIsCaught(t *testing.T) {
	for i, term := range defaultMaturity.phrases {
		if !defaultMaturity.flags(term) {
			t.Errorf("listed term %d is not caught by the filter", i)
		}
		if defaultMaturity.allow[term] {
			t.Errorf("listed term %d is also on the allowlist", i)
		}
	}
	if len(defaultMaturity.phrases) < 40 || len(defaultMaturity.allow) < 10 {
		t.Errorf("the embedded list looks truncated: %d terms, %d allowed", len(defaultMaturity.phrases), len(defaultMaturity.allow))
	}
}

func TestFoldText(t *testing.T) {
	tests := []struct{ in, want string }{
		{"Café", "cafe"}, {"s3x", "sex"}, {"$h!t", "sh t"}, {"@ss", "ass"}, {"a.b-c", "a b c"}, {"7ime 4ll", "time all"},
	}
	for _, tc := range tests {
		if got := strings.TrimSpace(foldText(tc.in, false)); got != tc.want {
			t.Errorf("foldText(%q) = %q, want %q", tc.in, got, tc.want)
		}
	}
	if got := collapseRuns("aaaabb cccc", 3, 2); got != "aabb cc" {
		t.Errorf("collapseRuns = %q", got)
	}
}

func TestTextReasons(t *testing.T) {
	tests := []struct {
		name, note, maker string
		want              []string
	}{
		{"Ward", "warm", "Ann", nil},
		{"Ward", "see http://x.example/y", "Ann", []string{ReasonLink}},
		{"Ward", "visit www.shop", "Ann", []string{ReasonLink}},
		{"Ward", "join discord.gg/abc", "Ann", []string{ReasonLink}},
		{"Ward", "my site is cheap.com", "Ann", []string{ReasonLink}},
		{"Ward", "write me a@b.org", "Ann", []string{ReasonLink, ReasonEmail}},
		{"Ward", "dm @someone now", "Ann", []string{ReasonHandle}},
		{"Ward", "e.g. a good ward, i.e. strong", "Ann", nil},
		{"Ward", "", "Ann", nil},
		{"BUY NOW CHEAP", "", "Ann", []string{ReasonCaps}},
		{"Ward", "", "ANNABELLE LEE", []string{ReasonCaps}},
		{"Ward", "NO CAP", "Ann", nil},
		{"Ward", "wooooow!!", "Ann", nil},
		{"Ward", "wooooooow", "Ann", []string{ReasonRepeat}},
		{"Ward", "......", "Ann", []string{ReasonRepeat}},
		{"Ward", ".....", "Ann", nil},
		{"Ward", "ward ward ward", "Ann", []string{ReasonNameRepeat}},
		{"Ward", "Ward ward", "Ann", nil},
		{"Ox", "ox ox ox", "Ann", nil},
		{"Ward", "free p0rn at http://x.io", "Ann", []string{ReasonMaturity, ReasonLink}},
		{"Ward", "", "Ann s.e.x", []string{ReasonMaturity}},
	}
	for i, tc := range tests {
		got := textReasons(defaultMaturity, tc.name, tc.note, tc.maker)
		if strings.Join(got, ",") != strings.Join(tc.want, ",") {
			t.Errorf("case %d: reasons = %v, want %v", i, got, tc.want)
		}
	}
}

func TestReasonTextNeverLeaksCodes(t *testing.T) {
	for _, code := range []string{ReasonMaturity, ReasonLink, ReasonEmail, ReasonHandle, ReasonCaps, ReasonRepeat, ReasonNameRepeat, ReasonRate, ReasonDuplicate, ReasonReported, ReasonResubmitted, ReasonRemoved, "unknown"} {
		w := ReasonText(code)
		if w == "" || w == code {
			t.Errorf("reason %q has no plain wording", code)
		}
	}
}
