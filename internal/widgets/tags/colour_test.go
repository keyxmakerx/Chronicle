package tags

import (
	"context"
	"errors"
	"testing"
)

// fakeColours is a campaign colour lookup that counts its calls.
type fakeColours struct {
	marks, accent map[string]string
	err           error
	calls         map[string]int
}

func (f *fakeColours) TagColours(_ context.Context, campaignID string) (string, string, error) {
	if f.calls == nil {
		f.calls = map[string]int{}
	}
	f.calls[campaignID]++
	return f.marks[campaignID], f.accent[campaignID], f.err
}

func colourService(repo *mockTagRepo, c CampaignColours) TagService {
	svc := NewTagService(repo)
	if c != nil {
		svc.SetCampaignColours(c)
	}
	return svc
}

func TestCreate_StoresColourOrFollows(t *testing.T) {
	src := &fakeColours{marks: map[string]string{"camp-1": "#d97706"}}
	tests := []struct {
		name       string
		color      string
		wantStored string
		wantShown  string
		wantOwn    bool
	}{
		{"no colour follows the campaign", "", "", "#d97706", false},
		{"blank colour follows the campaign", "   ", "", "#d97706", false},
		{"a colour is its own", "#ff5733", "#ff5733", "#ff5733", true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var stored string
			repo := &mockTagRepo{createFn: func(_ context.Context, tag *Tag) error {
				stored = tag.Color
				tag.ID = 1
				return nil
			}}
			got, err := colourService(repo, src).Create(context.Background(), "camp-1", "Salt road", tt.color, false)
			if err != nil {
				t.Fatal(err)
			}
			if stored != tt.wantStored {
				t.Errorf("stored %q, want %q", stored, tt.wantStored)
			}
			if got.Color != tt.wantShown || got.OwnColor != tt.wantOwn {
				t.Errorf("shown %q own=%v, want %q own=%v", got.Color, got.OwnColor, tt.wantShown, tt.wantOwn)
			}
			if got.StoredColor() != tt.wantStored {
				t.Errorf("StoredColor %q, want %q", got.StoredColor(), tt.wantStored)
			}
		})
	}
}

func TestUpdate_ColourKeys(t *testing.T) {
	src := &fakeColours{accent: map[string]string{"camp-1": "#2563eb"}}
	str := func(s string) *string { return &s }
	tests := []struct {
		name       string
		stored     string
		color      *string
		wantStored string
		wantShown  string
		wantOwn    bool
	}{
		{"empty colour puts an own-colour tag back to following", "#8b5cf6", str(""), "", "#2563eb", false},
		{"absent colour keeps following", "", nil, "", "#2563eb", false},
		{"absent colour keeps its own", "#8b5cf6", nil, "#8b5cf6", "#8b5cf6", true},
		{"a colour gives a following tag its own", "", str("#22c55e"), "#22c55e", "#22c55e", true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var written string
			repo := &mockTagRepo{
				findByIDFn: func(_ context.Context, id int) (*Tag, error) {
					return &Tag{ID: id, CampaignID: "camp-1", Name: "Harbour", Slug: "harbour", Color: tt.stored}, nil
				},
				updateFn: func(_ context.Context, tag *Tag) error { written = tag.Color; return nil },
			}
			got, err := colourService(repo, src).Update(context.Background(), 1, UpdateTagInput{Name: "Harbour", Color: tt.color})
			if err != nil {
				t.Fatal(err)
			}
			if written != tt.wantStored {
				t.Errorf("wrote %q, want %q", written, tt.wantStored)
			}
			if got.Color != tt.wantShown || got.OwnColor != tt.wantOwn {
				t.Errorf("shown %q own=%v, want %q own=%v", got.Color, got.OwnColor, tt.wantShown, tt.wantOwn)
			}
		})
	}
}

func TestFollowingTag_ResolutionOrder(t *testing.T) {
	tests := []struct {
		name string
		src  CampaignColours
		want string
	}{
		{"Tags and marks first", &fakeColours{marks: map[string]string{"c": "#d97706"}, accent: map[string]string{"c": "#2563eb"}}, "#d97706"},
		{"then the accent", &fakeColours{accent: map[string]string{"c": "#2563eb"}}, "#2563eb"},
		{"a bad Tags and marks value falls to the accent", &fakeColours{marks: map[string]string{"c": "amber"}, accent: map[string]string{"c": "#2563eb"}}, "#2563eb"},
		{"then Chronicle's default", &fakeColours{}, DefaultTagColour},
		{"a failed lookup still draws", &fakeColours{err: errors.New("down")}, DefaultTagColour},
		{"nothing wired", nil, DefaultTagColour},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			repo := &mockTagRepo{findByIDFn: func(_ context.Context, id int) (*Tag, error) {
				return &Tag{ID: id, CampaignID: "c", Name: "Wreckers"}, nil
			}}
			got, err := colourService(repo, tt.src).GetByID(context.Background(), 1)
			if err != nil {
				t.Fatal(err)
			}
			if got.Color != tt.want || got.OwnColor {
				t.Errorf("got %q own=%v, want %q", got.Color, got.OwnColor, tt.want)
			}
		})
	}
}

// Every read path asks each campaign once, however many tags it returns, and
// never touches a tag that has its own colour.
func TestReads_OneLookupPerCampaign(t *testing.T) {
	stored := []Tag{
		{ID: 1, CampaignID: "c", Name: "smugglers", Color: "#0d9488"},
		{ID: 2, CampaignID: "c", Name: "salt road"},
		{ID: 3, CampaignID: "c", Name: "wreckers"},
	}
	clone := func() []Tag { return append([]Tag(nil), stored...) }
	repo := &mockTagRepo{
		listByCampaignFn: func(context.Context, string, bool) ([]Tag, error) { return clone(), nil },
		getEntityTagsFn:  func(context.Context, string, bool) ([]Tag, error) { return clone(), nil },
		getEntityTagsBatchFn: func(context.Context, []string, bool) (map[string][]Tag, error) {
			return map[string][]Tag{"a": clone(), "b": clone()}, nil
		},
	}
	reads := map[string]func(TagService) ([]Tag, error){
		"list": func(s TagService) ([]Tag, error) { return s.ListByCampaign(context.Background(), "c", true) },
		"page": func(s TagService) ([]Tag, error) { return s.GetEntityTags(context.Background(), "a", true) },
		"batch": func(s TagService) ([]Tag, error) {
			by, err := s.GetEntityTagsBatch(context.Background(), []string{"a", "b"}, true)
			return append(by["a"], by["b"]...), err
		},
	}
	for name, read := range reads {
		t.Run(name, func(t *testing.T) {
			src := &fakeColours{marks: map[string]string{"c": "#d97706"}}
			got, err := read(colourService(repo, src))
			if err != nil {
				t.Fatal(err)
			}
			if src.calls["c"] != 1 {
				t.Errorf("asked the campaign %d times, want 1", src.calls["c"])
			}
			for _, tg := range got {
				want, own := "#d97706", false
				if tg.ID == 1 {
					want, own = "#0d9488", true
				}
				if tg.Color != want || tg.OwnColor != own {
					t.Errorf("%s: %q own=%v, want %q own=%v", tg.Name, tg.Color, tg.OwnColor, want, own)
				}
			}
		})
	}
}
