package entities

import (
	"context"
	"testing"
)

// pageObserver records field writes, text saves and removals.
type pageObserver struct {
	fakeFieldObserver
	texts   []string // "<userID>:<old name>→<new name>"
	trashed []string // "<userID>:<page id>"
}

func (p *pageObserver) TextChanged(_ context.Context, before, after *Entity, userID string) error {
	p.texts = append(p.texts, userID+":"+before.Name+"→"+after.Name)
	return nil
}

func (p *pageObserver) PageTrashed(_ context.Context, e *Entity, userID string) error {
	p.trashed = append(p.trashed, userID+":"+e.ID)
	return nil
}

// TestPageChangeObserver_Integration checks that a field observer which
// also wants page saves and removals hears about them with the person who
// made them, against a real MariaDB (page history and the Trash need it).
func TestPageChangeObserver_Integration(t *testing.T) {
	if testing.Short() {
		t.Skip("integration test requires a database; skipped under -short")
	}
	db := openTestDB(t)
	ctx := context.Background()

	bea := testUUID(t)
	campaignID := testUUID(t)
	mustExec(t, db, `INSERT INTO users (id, email, display_name, password_hash) VALUES (?, ?, ?, ?)`,
		bea, "bea-"+bea+"@example.test", "Bea", "x")
	mustExec(t, db, `INSERT INTO campaigns (id, name, slug, created_by) VALUES (?, ?, ?, ?)`,
		campaignID, "Observer Test", "observer-"+campaignID[:8], bea)
	typeRepo := NewEntityTypeRepository(db)
	et := &EntityType{
		CampaignID: campaignID, Slug: "characters", Name: "Character", NamePlural: "Characters",
		Icon: "fa-user", Color: "#111111", Fields: []FieldDefinition{}, Layout: DefaultLayout(),
		SortOrder: 1, Enabled: true,
	}
	mustCreate(t, typeRepo, ctx, et)

	svc := NewEntityService(NewEntityRepository(db), typeRepo, NewEntityPermissionRepository(db))
	svc.SetPageSafety(NewPageSafetyRepository(db))
	obs := &pageObserver{}
	svc.SetFieldChangeObserver(obs)

	asBea := WithActor(ctx, bea)
	tyne, err := svc.Create(asBea, campaignID, bea, CreateEntityInput{Name: "Tyne", EntityTypeID: et.ID})
	if err != nil {
		t.Fatal(err)
	}
	if len(obs.texts) != 0 {
		t.Fatalf("creating a page is not a text change: %v", obs.texts)
	}
	if err := svc.UpdateEntry(asBea, tyne.ID, `{"type":"doc","content":[]}`, "<p>Born in the hills.</p>"); err != nil {
		t.Fatal(err)
	}
	if len(obs.texts) != 1 || obs.texts[0] != bea+":Tyne→Tyne" {
		t.Fatalf("texts = %v, want one save by Bea", obs.texts)
	}
	if err := svc.Delete(asBea, tyne.ID); err != nil {
		t.Fatal(err)
	}
	if len(obs.trashed) != 1 || obs.trashed[0] != bea+":"+tyne.ID {
		t.Fatalf("trashed = %v, want Tyne removed by Bea", obs.trashed)
	}

}
