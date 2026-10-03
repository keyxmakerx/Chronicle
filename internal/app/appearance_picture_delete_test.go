package app

import (
	"context"
	"testing"

	"github.com/keyxmakerx/chronicle/internal/apperror"
	"github.com/keyxmakerx/chronicle/internal/plugins/media"
)

// pictureMediaStub embeds the interface so only GetByID and Delete are real;
// any other call would mean DeletePicture strayed from its job.
type pictureMediaStub struct {
	media.MediaService
	file    *media.MediaFile
	deleted []string
}

func (s *pictureMediaStub) GetByID(_ context.Context, _ string) (*media.MediaFile, error) {
	if s.file == nil {
		return nil, apperror.NewNotFound("media file not found")
	}
	return s.file, nil
}

func (s *pictureMediaStub) Delete(_ context.Context, id string) error {
	s.deleted = append(s.deleted, id)
	return nil
}

// TestBackdropUploaderAdapter_DeletePicture pins the rule that only a file
// this campaign uploaded through the appearance-picture path can be deleted.
func TestBackdropUploaderAdapter_DeletePicture(t *testing.T) {
	const name = "2026/10/abc.png"
	mine, theirs := "camp-1", "camp-2"
	cases := []struct {
		name string
		file *media.MediaFile
		want bool
	}{
		{"own backdrop-type file", &media.MediaFile{ID: "abc", Filename: name, UsageType: media.UsageBackdrop, CampaignID: &mine}, true},
		{"another campaign's file", &media.MediaFile{ID: "abc", Filename: name, UsageType: media.UsageBackdrop, CampaignID: &theirs}, false},
		{"own file of another usage type", &media.MediaFile{ID: "abc", Filename: name, UsageType: "attachment", CampaignID: &mine}, false},
		{"file with no campaign", &media.MediaFile{ID: "abc", Filename: name, UsageType: media.UsageBackdrop}, false},
		{"id matches but stored name differs", &media.MediaFile{ID: "abc", Filename: "2025/01/abc.png", UsageType: media.UsageBackdrop, CampaignID: &mine}, false},
		{"already gone", nil, false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			stub := &pictureMediaStub{file: tc.file}
			a := &backdropUploaderAdapter{svc: stub}
			got, err := a.DeletePicture(context.Background(), mine, name)
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if got != tc.want || (len(stub.deleted) == 1) != tc.want {
				t.Errorf("deleted = %v (calls %v), want %v", got, stub.deleted, tc.want)
			}
		})
	}
}
