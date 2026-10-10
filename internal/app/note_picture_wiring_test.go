package app

import (
	"context"
	"os"
	"strings"
	"testing"

	"github.com/keyxmakerx/chronicle/internal/permissions"
	"github.com/keyxmakerx/chronicle/internal/widgets/notes"
)

// Without the note-reader rule the media server admits only a picture's
// uploader, and without the picture store players cannot add pictures, so
// both calls are pinned in source.
func TestRoutes_WiresNotePictures(t *testing.T) {
	src, err := os.ReadFile("routes.go")
	if err != nil {
		t.Fatal(err)
	}
	for _, call := range []string{
		"mediaHandler.SetNoteMediaAccess(&noteMediaAccessAdapter{svc: noteSvc})",
		"noteHandler.SetPictureUploader(&mediaUploadAdapter{svc: mediaService})",
	} {
		if !strings.Contains(string(src), call) {
			t.Errorf("routes.go no longer calls %s", call)
		}
	}
}

type recordingNoteSvc struct {
	notes.NoteService
	viewer permissions.Viewer
}

func (r *recordingNoteSvc) ViewerReadsMedia(_ context.Context, _, _ string, v, _ permissions.Viewer) (bool, error) {
	r.viewer = v
	return true, nil
}

// The adapter hands the notes service the viewer's promoted role and user, so
// a GM-shared note counts for the GM and a private one never does.
func TestNoteMediaAccessAdapter_PassesTheViewerThrough(t *testing.T) {
	svc := &recordingNoteSvc{}
	ok, err := (&noteMediaAccessAdapter{svc: svc}).CanReadNoteMedia(context.Background(), "c", "m", permissions.RoleOwner, "u1", permissions.RolePlayer, "u2")
	if err != nil || !ok {
		t.Fatalf("got %v, %v", ok, err)
	}
	if svc.viewer.UserID() != "u1" || svc.viewer.Role() != permissions.RoleOwner {
		t.Errorf("notes service saw role %d user %q", svc.viewer.Role(), svc.viewer.UserID())
	}
}
