package systems

import (
	"context"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/labstack/echo/v4"

	"github.com/keyxmakerx/chronicle/internal/apperror"
)

func TestPluralBasename(t *testing.T) {
	cases := []struct{ in, want string }{
		{"ancestry", "ancestries"},
		{"kit", "kits"},
		{"race", "races"},
		{"class", "classes"},
		{"bonus", "bonuses"},
		{"key", "keys"},
		{"branch", "branches"},
		{"Ancestry", "ancestries"},
		{"character_class", "character_classes"},
	}
	for _, tc := range cases {
		t.Run(tc.in, func(t *testing.T) {
			if got := pluralBasename(tc.in); got != tc.want {
				t.Errorf("pluralBasename(%q) = %q, want %q", tc.in, got, tc.want)
			}
		})
	}
}

func TestCleanSummary(t *testing.T) {
	cases := []struct{ name, in, want string }{
		{"plain", "A short one.", "A short one."},
		{"reference term", "You are {@condition taunted}.", "You are taunted."},
		{"reference display", "You are {@condition taunted|taunts}.", "You are taunts."},
		{"html", "<p>Hello <b>there</b></p>", "Hello there"},
		{"whitespace", "a \n\n  b", "a b"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := cleanSummary(tc.in); got != tc.want {
				t.Errorf("got %q, want %q", got, tc.want)
			}
		})
	}
	long := cleanSummary(strings.Repeat("x", 500))
	if n := len([]rune(long)); n != maxChoiceSummary {
		t.Errorf("long summary has %d runes, want %d", n, maxChoiceSummary)
	}
}

func TestLoadChoicesFile(t *testing.T) {
	cases := []struct {
		name     string
		content  string // "" = file absent
		wantOK   bool
		wantN    int
		first    Choice
		wantDesc string
	}{
		{"slug shape", `[{"slug":"human","name":"Human","summary":"Plain {@condition x} folk"}]`, true, 1,
			Choice{Slug: "human", Name: "Human", Summary: "Plain x folk", Source: "package"}, ""},
		{"id shape with description", `[{"id":"elf","name":"Elf","description":"<p>Tall</p>","properties":{"size":"M"}}]`, true, 1,
			Choice{Slug: "elf", Name: "Elf", Summary: "Tall", Source: "package"}, "<p>Tall</p>"},
		{"missing file", "", false, 0, Choice{}, ""},
		{"not an array", `{"a":1}`, false, 0, Choice{}, ""},
		{"skips nameless and duplicates", `[{"slug":"a"},{"name":"B"},{"name":"b"}]`, true, 1,
			Choice{Name: "B", Source: "package"}, ""},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			p := filepath.Join(t.TempDir(), "x.json")
			if tc.content != "" {
				if err := os.WriteFile(p, []byte(tc.content), 0o600); err != nil {
					t.Fatal(err)
				}
			}
			got, ok := loadChoicesFile(p)
			if ok != tc.wantOK || len(got) != tc.wantN {
				t.Fatalf("ok=%v n=%d, want ok=%v n=%d", ok, len(got), tc.wantOK, tc.wantN)
			}
			if tc.wantN == 0 {
				return
			}
			g := got[0]
			if g.Slug != tc.first.Slug || g.Name != tc.first.Name || g.Summary != tc.first.Summary ||
				g.Source != tc.first.Source || g.Description != tc.wantDesc {
				t.Errorf("got %+v, want %+v (desc %q)", g, tc.first, tc.wantDesc)
			}
		})
	}
}

type fakeAddons map[string]bool

func (f fakeAddons) IsEnabledForCampaign(_ context.Context, _ string, slug string) (bool, error) {
	return f[slug], nil
}

func newTestChoiceService(t *testing.T, files map[string]string, manifests []*SystemManifest, enabled fakeAddons) *characterChoiceService {
	t.Helper()
	root := t.TempDir()
	for rel, body := range files {
		p := filepath.Join(root, rel)
		if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(p, []byte(body), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	return &characterChoiceService{
		addons:    enabled,
		manifests: func() []*SystemManifest { return manifests },
		dirOf:     func(id string) string { return filepath.Join(root, id) },
	}
}

func TestCharacterChoices(t *testing.T) {
	ds := &SystemManifest{ID: "drawsteel", Name: "Draw Steel"}
	dnd := &SystemManifest{ID: "dnd5e", Name: "D&D 5.5e", EntityPresets: []EntityPresetDef{{
		Fields: []FieldDef{{Key: "race", Choices: "species"}},
	}}}
	files := map[string]string{
		"drawsteel/data/ancestries.json": `[{"slug":"orc","name":"Orc"},{"slug":"dwarf","name":"Dwarf"}]`,
		"dnd5e/data/species.json":        `[{"id":"elf","name":"Elf"}]`,
	}

	cases := []struct {
		name     string
		key      string
		enabled  fakeAddons
		wantErr  bool
		wantSys  string
		wantName []string
	}{
		{"convention plural, sorted", "ancestry", fakeAddons{"drawsteel": true}, false, "Draw Steel", []string{"Dwarf", "Orc"}},
		{"explicit choices file", "race", fakeAddons{"dnd5e": true}, false, "D&D 5.5e", []string{"Elf"}},
		{"no list gives system name and empty", "class", fakeAddons{"drawsteel": true}, false, "Draw Steel", nil},
		{"system not enabled", "ancestry", fakeAddons{}, false, "", nil},
		{"traversal key rejected", "../etc/passwd", fakeAddons{"drawsteel": true}, true, "", nil},
		{"empty key rejected", "", fakeAddons{"drawsteel": true}, true, "", nil},
		{"dotted key rejected", "a.b", fakeAddons{"drawsteel": true}, true, "", nil},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			svc := newTestChoiceService(t, files, []*SystemManifest{ds, dnd}, tc.enabled)
			l, err := svc.CharacterChoiceList(context.Background(), "c1", tc.key)
			if tc.wantErr {
				if err == nil {
					t.Fatal("expected error")
				}
				return
			}
			if err != nil {
				t.Fatal(err)
			}
			if l.SystemName != tc.wantSys {
				t.Errorf("system = %q, want %q", l.SystemName, tc.wantSys)
			}
			if len(l.Choices) != len(tc.wantName) {
				t.Fatalf("got %d choices, want %d", len(l.Choices), len(tc.wantName))
			}
			for i, n := range tc.wantName {
				if l.Choices[i].Name != n {
					t.Errorf("choice %d = %q, want %q", i, l.Choices[i].Name, n)
				}
			}
		})
	}
}

func TestRegisterChoiceSource(t *testing.T) {
	choiceSourcesMu.Lock()
	saved := choiceSources
	choiceSources = nil
	choiceSourcesMu.Unlock()
	t.Cleanup(func() {
		choiceSourcesMu.Lock()
		choiceSources = saved
		choiceSourcesMu.Unlock()
	})

	RegisterChoiceSource(func(_ context.Context, _, key string) ([]Choice, error) {
		return []Choice{{Name: "Homebrew " + key, Source: "campaign"}}, nil
	})
	svc := newTestChoiceService(t, nil, nil, fakeAddons{})
	got, err := svc.CharacterChoices(context.Background(), "c1", "ancestry")
	if err != nil || len(got) != 1 || got[0].Source != "campaign" {
		t.Fatalf("got %+v err %v", got, err)
	}
}

type stubChoiceSvc struct{ called bool }

func (s *stubChoiceSvc) CharacterChoices(ctx context.Context, c, k string) ([]Choice, error) {
	return nil, nil
}

func (s *stubChoiceSvc) CharacterChoiceList(ctx context.Context, c, k string) (*ChoiceList, error) {
	s.called = true
	if !ValidChoiceFieldKey(k) {
		return nil, apperror.NewBadRequest("invalid field key")
	}
	return &ChoiceList{FieldKey: k, Choices: []Choice{}}, nil
}

func TestValidChoiceFieldKey(t *testing.T) {
	cases := []struct {
		key  string
		want bool
	}{
		{"ancestry", true}, {"hero_class2", true}, {"Kit", true},
		{"", false}, {"1abc", false}, {"a-b", false}, {"a/b", false}, {"..", false},
		{"a b", false}, {strings.Repeat("a", 65), false},
	}
	for _, tc := range cases {
		if got := ValidChoiceFieldKey(tc.key); got != tc.want {
			t.Errorf("ValidChoiceFieldKey(%q) = %v, want %v", tc.key, got, tc.want)
		}
	}
}

// Without a campaign context the handler must refuse before reaching the service.
func TestChoicesAPIRequiresCampaignContext(t *testing.T) {
	stub := &stubChoiceSvc{}
	h := NewCharacterChoiceHandler(stub)
	e := echo.New()
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	c := e.NewContext(req, httptest.NewRecorder())
	c.SetParamNames("fieldKey")
	c.SetParamValues("ancestry")
	if err := h.ChoicesAPI(c); err == nil {
		t.Fatal("expected missing-context error")
	}
	if stub.called {
		t.Error("service must not be called without a campaign context")
	}
}
