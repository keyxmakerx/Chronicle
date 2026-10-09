package systems

import (
	"context"
	"encoding/json"
	"strings"
	"testing"

	"github.com/keyxmakerx/chronicle/internal/apperror"
	"github.com/keyxmakerx/chronicle/internal/patch"
)

// fakeEntryRepo is an in-memory SystemEntryRepository.
type fakeEntryRepo struct {
	rows   []SystemEntry
	nextID int64
}

func (f *fakeEntryRepo) List(_ context.Context, camp, sys, field string, dirs bool) ([]SystemEntry, error) {
	out := []SystemEntry{}
	for _, r := range f.rows {
		if r.CampaignID != camp || r.SystemID != sys || (field != "" && r.FieldKey != field) {
			continue
		}
		if !dirs && r.Visibility != EntryVisibilityEveryone {
			continue
		}
		out = append(out, r)
	}
	return out, nil
}
func (f *fakeEntryRepo) Get(_ context.Context, camp string, id int64) (*SystemEntry, error) {
	for i := range f.rows {
		if f.rows[i].ID == id && f.rows[i].CampaignID == camp {
			c := f.rows[i]
			return &c, nil
		}
	}
	return nil, apperror.NewNotFound("entry not found")
}
func (f *fakeEntryRepo) FindByName(_ context.Context, camp, sys, field, name string) (*SystemEntry, error) {
	for i := range f.rows {
		r := f.rows[i]
		if r.CampaignID == camp && r.SystemID == sys && r.FieldKey == field && strings.EqualFold(r.Name, name) {
			return &r, nil
		}
	}
	return nil, nil
}
func (f *fakeEntryRepo) SlugExists(_ context.Context, camp, sys, field, slug string) (bool, error) {
	for _, r := range f.rows {
		if r.CampaignID == camp && r.SystemID == sys && r.FieldKey == field && r.Slug == slug {
			return true, nil
		}
	}
	return false, nil
}
func (f *fakeEntryRepo) Count(_ context.Context, camp, sys, field string) (int, error) {
	l, _ := f.List(context.Background(), camp, sys, field, true)
	return len(l), nil
}
func (f *fakeEntryRepo) Create(_ context.Context, e *SystemEntry) error {
	f.nextID++
	e.ID = f.nextID
	f.rows = append(f.rows, *e)
	return nil
}
func (f *fakeEntryRepo) Update(_ context.Context, e *SystemEntry) error {
	for i := range f.rows {
		if f.rows[i].ID == e.ID {
			f.rows[i] = *e
		}
	}
	return nil
}
func (f *fakeEntryRepo) Delete(_ context.Context, camp string, id int64) (bool, error) {
	for i := range f.rows {
		if f.rows[i].ID == id && f.rows[i].CampaignID == camp {
			f.rows = append(f.rows[:i], f.rows[i+1:]...)
			return true, nil
		}
	}
	return false, nil
}

func entryTestManifest() *SystemManifest {
	return &SystemManifest{ID: "sys", EntityPresets: []EntityPresetDef{
		{Slug: "hero", Category: "character", Fields: []FieldDef{
			{Key: "ancestry", Type: "string"}, {Key: "level", Type: "number"}}},
		{Slug: "loot", Category: "item", Fields: []FieldDef{{Key: "rarity", Type: "string"}}},
	}}
}

func newEntrySvc(m *SystemManifest) (SystemEntryService, *fakeEntryRepo) {
	repo := &fakeEntryRepo{}
	return NewSystemEntryService(repo, func(context.Context, string) *SystemManifest { return m }), repo
}

var (
	dirActor    = EntryActor{UserID: "dm", IsDirector: true}
	playerActor = EntryActor{UserID: "pl"}
)

func TestSystemEntryService_Create(t *testing.T) {
	long := strings.Repeat("x", maxEntryName+1)
	tests := []struct {
		name    string
		actor   EntryActor
		in      CreateSystemEntryInput
		wantErr int // HTTP code, 0 = ok
	}{
		{"director ok", dirActor, CreateSystemEntryInput{FieldKey: "ancestry", Name: "Stoneborn"}, 0},
		{"player refused", playerActor, CreateSystemEntryInput{FieldKey: "ancestry", Name: "Stoneborn"}, 403},
		{"field not on character preset", dirActor, CreateSystemEntryInput{FieldKey: "rarity", Name: "Rare"}, 422},
		{"field not a string", dirActor, CreateSystemEntryInput{FieldKey: "level", Name: "Ten"}, 422},
		{"unknown field", dirActor, CreateSystemEntryInput{FieldKey: "nope", Name: "X"}, 422},
		{"path-like field", dirActor, CreateSystemEntryInput{FieldKey: "../x", Name: "X"}, 422},
		{"name required", dirActor, CreateSystemEntryInput{FieldKey: "ancestry", Name: "  "}, 422},
		{"name too long", dirActor, CreateSystemEntryInput{FieldKey: "ancestry", Name: long}, 422},
		{"bad visibility", dirActor, CreateSystemEntryInput{FieldKey: "ancestry", Name: "A", Visibility: "players"}, 422},
		{"nested property", dirActor, CreateSystemEntryInput{FieldKey: "ancestry", Name: "A", Properties: map[string]any{"a": map[string]any{"b": 1}}}, 422},
		{"null property", dirActor, CreateSystemEntryInput{FieldKey: "ancestry", Name: "A", Properties: map[string]any{"a": nil}}, 422},
		{"bad property key", dirActor, CreateSystemEntryInput{FieldKey: "ancestry", Name: "A", Properties: map[string]any{"1a": "x"}}, 422},
		{"scalar properties ok", dirActor, CreateSystemEntryInput{FieldKey: "ancestry", Name: "A", Properties: map[string]any{"size": "Medium", "speed": 5.0, "fly": true}}, 0},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			svc, _ := newEntrySvc(entryTestManifest())
			_, err := svc.Create(context.Background(), "c1", tt.actor, tt.in)
			got := 0
			if err != nil {
				ae, ok := err.(*apperror.AppError)
				if !ok {
					t.Fatalf("non-AppError returned: %T %v", err, err)
				}
				got = ae.Code
			}
			if got != tt.wantErr {
				t.Fatalf("code = %d (%v), want %d", got, err, tt.wantErr)
			}
		})
	}
}

func TestSystemEntryService_NoSystem(t *testing.T) {
	svc, _ := newEntrySvc(nil)
	if _, err := svc.Create(context.Background(), "c1", dirActor, CreateSystemEntryInput{FieldKey: "ancestry", Name: "A"}); err == nil {
		t.Fatal("expected an error when the campaign has no game system")
	}
	if l, err := svc.List(context.Background(), "c1", "", dirActor); err != nil || len(l) != 0 {
		t.Fatalf("list with no system = %v, %v; want empty", l, err)
	}
}

func TestSystemEntryService_SlugAndDuplicates(t *testing.T) {
	svc, _ := newEntrySvc(entryTestManifest())
	ctx := context.Background()
	a, err := svc.Create(ctx, "c1", dirActor, CreateSystemEntryInput{FieldKey: "ancestry", Name: "Half-Elf!"})
	if err != nil || a.Slug != "half-elf" {
		t.Fatalf("slug = %v err %v, want half-elf", a, err)
	}
	// Same name, any casing, in the same field is a conflict.
	if _, err := svc.Create(ctx, "c1", dirActor, CreateSystemEntryInput{FieldKey: "ancestry", Name: "HALF-ELF!"}); err == nil {
		t.Fatal("duplicate name accepted")
	}
	// Different names that slugify alike get distinct slugs.
	b, err := svc.Create(ctx, "c1", dirActor, CreateSystemEntryInput{FieldKey: "ancestry", Name: "Half Elf"})
	if err != nil || b.Slug != "half-elf-2" {
		t.Fatalf("second slug = %v err %v, want half-elf-2", b, err)
	}
	// A name with no ASCII letters still gets a slug.
	c, err := svc.Create(ctx, "c1", dirActor, CreateSystemEntryInput{FieldKey: "ancestry", Name: "龍"})
	if err != nil || c.Slug != "entry" {
		t.Fatalf("fallback slug = %v err %v", c, err)
	}
}

func TestSystemEntryService_DescriptionSanitized(t *testing.T) {
	svc, _ := newEntrySvc(entryTestManifest())
	e, err := svc.Create(context.Background(), "c1", dirActor, CreateSystemEntryInput{
		FieldKey: "ancestry", Name: "A", Description: `<p onclick="x()">hi</p><script>alert(1)</script>`})
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(e.Description, "script") || strings.Contains(e.Description, "onclick") {
		t.Fatalf("description not sanitized: %q", e.Description)
	}
}

func TestSystemEntryService_VisibilityFiltering(t *testing.T) {
	svc, _ := newEntrySvc(entryTestManifest())
	ctx := context.Background()
	for _, in := range []CreateSystemEntryInput{
		{FieldKey: "ancestry", Name: "Open"},
		{FieldKey: "ancestry", Name: "Secret", Visibility: EntryVisibilityDirectors},
	} {
		if _, err := svc.Create(ctx, "c1", dirActor, in); err != nil {
			t.Fatal(err)
		}
	}
	tests := []struct {
		name  string
		actor EntryActor
		want  int
	}{{"director sees both", dirActor, 2}, {"player sees one", playerActor, 1}}
	for _, tt := range tests {
		l, _ := svc.List(ctx, "c1", "ancestry", tt.actor)
		if len(l) != tt.want {
			t.Errorf("%s: got %d entries, want %d", tt.name, len(l), tt.want)
		}
	}
	if e, _ := svc.FindByName(ctx, "c1", "ancestry", "Secret", playerActor); e != nil {
		t.Error("a player found a Directors-only entry by name")
	}
	if e, _ := svc.FindByName(ctx, "c1", "ancestry", "Secret", dirActor); e == nil {
		t.Error("a Director could not find their own entry")
	}
	// The pick-list hook never carries Directors-only entries.
	ch, _ := svc.Choices(ctx, "c1", "ancestry")
	if len(ch) != 1 || ch[0].Name != "Open" || ch[0].Source != ChoiceSourceCampaign {
		t.Fatalf("choices = %+v, want only Open from source campaign", ch)
	}
	if other, _ := svc.Choices(ctx, "other-campaign", "ancestry"); len(other) != 0 {
		t.Fatalf("another campaign's choices leaked: %+v", other)
	}
}

func TestSystemEntryService_UpdatePartialContract(t *testing.T) {
	ctx := context.Background()
	seed := func() (SystemEntryService, int64) {
		svc, _ := newEntrySvc(entryTestManifest())
		e, err := svc.Create(ctx, "c1", dirActor, CreateSystemEntryInput{
			FieldKey: "ancestry", Name: "Orig", Summary: "sum", Description: "<p>desc</p>",
			Properties: map[string]any{"size": "Medium"}, Visibility: EntryVisibilityDirectors})
		if err != nil {
			t.Fatal(err)
		}
		return svc, e.ID
	}
	tests := []struct {
		name  string
		body  string
		check func(t *testing.T, e *SystemEntry)
		code  int
	}{
		{"name only preserves everything else", `{"name":"New"}`, func(t *testing.T, e *SystemEntry) {
			if e.Name != "New" || e.Summary != "sum" || e.Description != "<p>desc</p>" ||
				e.Visibility != EntryVisibilityDirectors || e.Properties["size"] != "Medium" || e.Slug != "orig" {
				t.Fatalf("rename clobbered a field: %+v", e)
			}
		}, 0},
		{"null summary clears", `{"summary":null}`, func(t *testing.T, e *SystemEntry) {
			if e.Summary != "" || e.Description != "<p>desc</p>" {
				t.Fatalf("%+v", e)
			}
		}, 0},
		{"null properties clears", `{"properties":null}`, func(t *testing.T, e *SystemEntry) {
			if len(e.Properties) != 0 {
				t.Fatalf("%+v", e)
			}
		}, 0},
		{"present properties replaces", `{"properties":{"speed":6}}`, func(t *testing.T, e *SystemEntry) {
			if _, has := e.Properties["size"]; has || e.Properties["speed"] != 6.0 {
				t.Fatalf("%+v", e)
			}
		}, 0},
		{"visibility flips only when sent", `{"visibility":"everyone"}`, func(t *testing.T, e *SystemEntry) {
			if e.Visibility != EntryVisibilityEveryone || e.Name != "Orig" {
				t.Fatalf("%+v", e)
			}
		}, 0},
		{"null name refused", `{"name":null}`, nil, 422},
		{"null visibility refused", `{"visibility":null}`, nil, 422},
		{"empty name refused", `{"name":""}`, nil, 422},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			svc, id := seed()
			var in UpdateSystemEntryInput
			if err := readJSONForTest(tt.body, &in); err != nil {
				t.Fatal(err)
			}
			e, err := svc.Update(ctx, "c1", dirActor, id, in)
			if tt.code != 0 {
				if ae, ok := err.(*apperror.AppError); !ok || ae.Code != tt.code {
					t.Fatalf("err = %v, want code %d", err, tt.code)
				}
				return
			}
			if err != nil {
				t.Fatal(err)
			}
			tt.check(t, e)
		})
	}
}

func TestSystemEntryService_UpdateDeletePermissionsAndScope(t *testing.T) {
	svc, _ := newEntrySvc(entryTestManifest())
	ctx := context.Background()
	e, _ := svc.Create(ctx, "c1", dirActor, CreateSystemEntryInput{FieldKey: "ancestry", Name: "A"})

	if _, err := svc.Update(ctx, "c1", playerActor, e.ID, UpdateSystemEntryInput{Name: patch.Of("B")}); err == nil {
		t.Error("player updated an entry")
	}
	if err := svc.Delete(ctx, "c1", playerActor, e.ID); err == nil {
		t.Error("player deleted an entry")
	}
	// Another campaign's Director cannot reach it by id.
	if err := svc.Delete(ctx, "c2", dirActor, e.ID); err == nil {
		t.Error("delete crossed campaigns")
	}
	if err := svc.Delete(ctx, "c1", dirActor, e.ID); err != nil {
		t.Errorf("director delete: %v", err)
	}
	if err := svc.Delete(ctx, "c1", dirActor, e.ID); err == nil {
		t.Error("second delete should be not-found")
	}
}

func TestSystemEntryService_RenameConflict(t *testing.T) {
	svc, _ := newEntrySvc(entryTestManifest())
	ctx := context.Background()
	_, _ = svc.Create(ctx, "c1", dirActor, CreateSystemEntryInput{FieldKey: "ancestry", Name: "A"})
	b, _ := svc.Create(ctx, "c1", dirActor, CreateSystemEntryInput{FieldKey: "ancestry", Name: "B"})
	if _, err := svc.Update(ctx, "c1", dirActor, b.ID, UpdateSystemEntryInput{Name: patch.Of("a")}); err == nil {
		t.Fatal("rename onto an existing name accepted")
	}
	// Re-casing its own name is not a conflict.
	if _, err := svc.Update(ctx, "c1", dirActor, b.ID, UpdateSystemEntryInput{Name: patch.Of("b")}); err != nil {
		t.Fatalf("re-case own name: %v", err)
	}
}

func TestSystemEntryService_PerFieldCap(t *testing.T) {
	svc, repo := newEntrySvc(entryTestManifest())
	for i := 0; i < maxEntriesPerField; i++ {
		repo.rows = append(repo.rows, SystemEntry{ID: int64(i + 1), CampaignID: "c1", SystemID: "sys", FieldKey: "ancestry"})
	}
	if _, err := svc.Create(context.Background(), "c1", dirActor, CreateSystemEntryInput{FieldKey: "ancestry", Name: "One too many"}); err == nil {
		t.Fatal("cap not enforced")
	}
}

func readJSONForTest(body string, dst any) error { return json.Unmarshal([]byte(body), dst) }
