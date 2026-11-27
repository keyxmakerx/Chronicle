package app

import (
	"context"
	"errors"
	"testing"

	"github.com/keyxmakerx/chronicle/internal/apperror"
	"github.com/keyxmakerx/chronicle/internal/plugins/media"
)

// shopPictureMediaStub embeds the interface so only GetByID is real.
type shopPictureMediaStub struct {
	media.MediaService
	file *media.MediaFile
	err  error
}

func (s shopPictureMediaStub) GetByID(context.Context, string) (*media.MediaFile, error) {
	if s.err != nil {
		return nil, s.err
	}
	if s.file == nil {
		return nil, apperror.NewNotFound("media file not found")
	}
	return s.file, nil
}

// TestShopRoomPictureAdapter pins which files a shop room may show as its own
// picture: only an image of the same campaign, out of the Trash, that no note
// or page owns.
func TestShopRoomPictureAdapter(t *testing.T) {
	mine, theirs, batch := "camp-1", "camp-2", "batch-1"
	cases := []struct {
		name    string
		file    *media.MediaFile
		err     error
		want    bool
		wantErr bool
	}{
		{"this campaign's picture", &media.MediaFile{CampaignID: &mine, MimeType: "image/png", UsageType: media.UsageAttachment}, nil, true, false},
		{"a page's picture of this campaign", &media.MediaFile{CampaignID: &mine, MimeType: "image/jpeg", UsageType: media.UsageEntityImage}, nil, true, false},
		{"another campaign's picture", &media.MediaFile{CampaignID: &theirs, MimeType: "image/png"}, nil, false, false},
		{"a file with no campaign", &media.MediaFile{MimeType: "image/png", UsageType: media.UsageBackdrop}, nil, false, false},
		{"a sound", &media.MediaFile{CampaignID: &mine, MimeType: "audio/mpeg"}, nil, false, false},
		{"a note's picture", &media.MediaFile{CampaignID: &mine, MimeType: "image/png", UsageType: media.UsageNoteImage}, nil, false, false},
		{"a page file", &media.MediaFile{CampaignID: &mine, MimeType: "image/png", UsageType: media.UsagePageFile}, nil, false, false},
		{"a picture in the Trash", &media.MediaFile{CampaignID: &mine, MimeType: "image/png", TrashBatchID: &batch}, nil, false, false},
		{"a missing file", nil, nil, false, false},
		{"a failed lookup", nil, errors.New("db down"), false, true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			a := &shopRoomPictureAdapter{svc: shopPictureMediaStub{file: tc.file, err: tc.err}}
			got, err := a.IsCampaignPicture(context.Background(), mine, "0b9c2f4e-1111-4222-8333-444455556666")
			if (err != nil) != tc.wantErr || got != tc.want {
				t.Errorf("got %v, %v; want %v, error %v", got, err, tc.want, tc.wantErr)
			}
		})
	}
}
