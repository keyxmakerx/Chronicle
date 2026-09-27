package notes

import (
	"context"
	"testing"
)

// TestDB_AttachmentSaves is the regression for the attachment id width: the
// service mints 36-character ids and notes have 36-character ids, and the
// insert must fit them.
func TestDB_AttachmentSaves(t *testing.T) {
	db := newNotesScratchDB(t)
	ctx := context.Background()
	repo := NewNoteRepository(db)
	camp, u := seedNotesCampaign(t, db, "gm")
	n := &Note{ID: newUUID(t), CampaignID: camp, UserID: u["gm"], Title: "Session 12", Content: []Block{}, Color: "#374151"}
	if err := repo.Create(ctx, n); err != nil {
		t.Fatal(err)
	}
	svc := NewNoteServiceWithAttachments(repo, NewAttachmentRepository(db))
	att := &NoteAttachment{NoteID: n.ID, CampaignID: camp, FilePath: "2026/09/x.mp3", OriginalName: "recap.mp3", MimeType: "audio/mpeg", FileSize: 42}
	if err := svc.CreateAttachment(ctx, att); err != nil {
		t.Fatalf("saving an attachment must work: %v", err)
	}
	list, err := svc.ListAttachments(ctx, n.ID)
	if err != nil || len(list) != 1 || list[0].ID != att.ID || list[0].NoteID != n.ID {
		t.Fatalf("the attachment must read back whole: %+v, %v", list, err)
	}
	if err := svc.UpdateTranscript(ctx, att.ID, "Party reaches the gatehouse."); err != nil {
		t.Fatalf("transcript: %v", err)
	}
}
