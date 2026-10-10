package app

import (
	"context"
	"os"
	"strings"
	"testing"

	"github.com/keyxmakerx/chronicle/internal/permissions"
	"github.com/keyxmakerx/chronicle/internal/plugins/media"
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

func (r *recordingNoteSvc) ViewerReadsMedia(_ context.Context, _, _ string, v permissions.Viewer) (bool, error) {
	r.viewer = v
	return true, nil
}

// The adapter hands the notes service the viewer's promoted role and user, so
// a GM-shared note counts for the GM and a private one never does.
func TestNoteMediaAccessAdapter_PassesTheViewerThrough(t *testing.T) {
	svc := &recordingNoteSvc{}
	ok, err := (&noteMediaAccessAdapter{svc: svc}).CanReadNoteMedia(context.Background(), "c", "m", permissions.RoleOwner, "u1")
	if err != nil || !ok {
		t.Fatalf("got %v, %v", ok, err)
	}
	if svc.viewer.UserID() != "u1" || svc.viewer.Role() != permissions.RoleOwner {
		t.Errorf("notes service saw role %d user %q", svc.viewer.Role(), svc.viewer.UserID())
	}
}

type pictureMediaSvc struct {
	media.MediaService
	file *media.MediaFile
}

func (p pictureMediaSvc) GetByID(context.Context, string) (*media.MediaFile, error) {
	return p.file, nil
}

func (pictureMediaSvc) FilePath(*media.MediaFile) string { return "/nonexistent/picture" }

// A page, a map or a map's picture source may not adopt a note picture: their
// readers are not the note's readers.
func TestMediaVerifiers_RefuseNotePictures(t *testing.T) {
	camp := "c1"
	file := func(usage string) *media.MediaFile {
		return &media.MediaFile{ID: "m", CampaignID: &camp, MimeType: "image/png", UsageType: usage}
	}
	ctx := context.Background()
	tests := []struct {
		name  string
		usage string
		want  bool
	}{
		{"page picture", media.UsageEntityImage, true},
		{"attachment", media.UsageAttachment, true},
		{"note picture", media.UsageNoteImage, false},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			svc := pictureMediaSvc{file: file(tc.usage)}
			if got, err := (&entityMediaVerifierAdapter{svc: svc}).MediaExistsInCampaign(ctx, "m", camp); err != nil || got != tc.want {
				t.Errorf("page verifier = %v, %v; want %v", got, err, tc.want)
			}
			if got, err := (&mapMediaVerifierAdapter{svc: svc}).ImageInCampaign(ctx, "m", camp); err != nil || got != tc.want {
				t.Errorf("map verifier = %v, %v; want %v", got, err, tc.want)
			}
			_, err := (&mapImageSourceAdapter{svc: svc}).ReadImage(ctx, camp, "m")
			if !tc.want && err == nil {
				t.Error("map image source read a note picture")
			}
		})
	}
}

// A campaign backdrop may be set to a file of the campaign, but never to a
// note picture, whose readers are the note's readers.
func TestBackdropOwnsFile_RefusesNotePictures(t *testing.T) {
	camp := "c1"
	tests := []struct {
		name  string
		usage string
		want  bool
	}{
		{"backdrop", media.UsageBackdrop, true},
		{"page picture", media.UsageEntityImage, true},
		{"note picture", media.UsageNoteImage, false},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			file := &media.MediaFile{ID: "m", CampaignID: &camp, Filename: "2026/01/m.png", UsageType: tc.usage}
			got, err := (&backdropUploaderAdapter{svc: pictureMediaSvc{file: file}}).OwnsFile(context.Background(), camp, "2026/01/m.png")
			if err != nil || got != tc.want {
				t.Errorf("OwnsFile = %v, %v; want %v", got, err, tc.want)
			}
		})
	}
}
