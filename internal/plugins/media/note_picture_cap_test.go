// note_picture_cap_test.go pins the per-uploader cap on note pictures: the
// pure cap arithmetic, the service refusal, and (against a real MariaDB) that
// only the uploader's own rows in the same campaign are counted.
package media

import (
	"context"
	"errors"
	"testing"

	"github.com/keyxmakerx/chronicle/internal/apperror"
)

func TestNotePictureByteCap(t *testing.T) {
	const mb = 1024 * 1024
	tests := []struct {
		name        string
		campaignMax int64
		want        int64
	}{
		{"unlimited campaign uses the ceiling", 0, 100 * mb},
		{"large campaign uses the ceiling", 10 * 1024 * mb, 100 * mb},
		{"small campaign gets a quarter", 200 * mb, 50 * mb},
		{"quarter exactly at the ceiling", 400 * mb, 100 * mb},
		{"tiny campaign", 4 * mb, 1 * mb},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := notePictureByteCap(tt.campaignMax); got != tt.want {
				t.Errorf("notePictureByteCap(%d) = %d, want %d", tt.campaignMax, got, tt.want)
			}
		})
	}
}

func TestCheckNotePictureCap(t *testing.T) {
	const mb = 1024 * 1024
	tests := []struct {
		name        string
		campaignMax int64
		limiterErr  error
		usedBytes   int64
		usedCount   int
		usageErr    error
		fileSize    int64
		wantCode    int // 0 = allowed
	}{
		{"empty uploader", 0, nil, 0, 0, nil, 5 * mb, 0},
		{"exactly at the byte cap", 0, nil, 95 * mb, 10, nil, 5 * mb, 0},
		{"one byte over the byte cap", 0, nil, 95 * mb, 10, nil, 5*mb + 1, 400},
		{"quarter of a small campaign", 40 * mb, nil, 6 * mb, 2, nil, 5 * mb, 400},
		{"under quarter of a small campaign", 40 * mb, nil, 4 * mb, 2, nil, 5 * mb, 0},
		{"last allowed file", 0, nil, 199, 199, nil, 1, 0},
		{"file count cap", 0, nil, 200, 200, nil, 1, 400},
		{"limiter failure keeps the ceiling", 0, errors.New("down"), 99 * mb, 5, nil, 2 * mb, 400},
		{"usage failure fails closed", 0, nil, 0, 0, errors.New("db down"), 1, 500},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			repo := &mockMediaRepo{getUserNoteImageUsageFn: func(_ context.Context, camp, user string) (int64, int, error) {
				if camp != "c1" || user != "u1" {
					t.Errorf("usage asked for %q/%q, want c1/u1", camp, user)
				}
				return tt.usedBytes, tt.usedCount, tt.usageErr
			}}
			svc := newTestMediaService(repo)
			svc.limiter = &mockStorageLimiter{getEffectiveLimitsFn: func(context.Context, string, string) (int64, int64, int, error) {
				return 0, tt.campaignMax, 0, tt.limiterErr
			}}
			err := svc.checkNotePictureCap(context.Background(), UploadInput{CampaignID: "c1", UploadedBy: "u1", FileSize: tt.fileSize})
			if tt.wantCode == 0 {
				if err != nil {
					t.Fatalf("unexpected error: %v", err)
				}
				return
			}
			assertMediaAppError(t, err, tt.wantCode)
			if tt.wantCode == 400 {
				var ae *apperror.AppError
				if errors.As(err, &ae) && ae.Message != errNotePictureSpace {
					t.Errorf("message = %q, want the plain-language one", ae.Message)
				}
			}
		})
	}
}

// Only note pictures are capped, and the cap runs without a limiter wired.
func TestUpload_NotePictureCapOnlyForNotePictures(t *testing.T) {
	called := 0
	repo := &mockMediaRepo{getUserNoteImageUsageFn: func(context.Context, string, string) (int64, int, error) {
		called++
		return 1 << 40, 1000, nil
	}}
	svc := newTestMediaService(repo)
	svc.mediaPath = t.TempDir()
	img := tinyPNG(t)

	_, err := svc.Upload(context.Background(), UploadInput{CampaignID: "c1", UploadedBy: "u1", OriginalName: "a.png",
		MimeType: "image/png", FileSize: int64(len(img)), UsageType: UsageNoteImage, FileBytes: img})
	assertMediaAppError(t, err, 400)

	if _, err := svc.Upload(context.Background(), UploadInput{CampaignID: "c1", UploadedBy: "u1", OriginalName: "a.png",
		MimeType: "image/png", FileSize: int64(len(img)), UsageType: UsageEntityImage, FileBytes: img}); err != nil {
		t.Fatalf("an entity image must not be touched by the note-picture cap: %v", err)
	}
	if called != 1 {
		t.Errorf("usage lookup ran %d times, want 1 (note picture only)", called)
	}
}

func TestDB_NotePictureCapCountsOnlyThatUploader(t *testing.T) {
	db := newADR058ScratchDB(t)
	ctx := context.Background()
	camp, gm := seedADR058Campaign(t, db)
	otherCamp, _ := seedADR058Campaign(t, db)
	ana, bo := seedADR058User(t, db, "Ana"), seedADR058User(t, db, "Bo")
	for _, id := range []string{gm, ana, bo} {
		seedADR058Member(t, db, camp, id, "player")
	}

	// Campaign quota 12000 bytes -> a quarter is 3000 per member. Seeded rows
	// are 1024 bytes each.
	const quota = 12000
	svc := NewMediaService(NewMediaRepository(db), t.TempDir(), 10*1024*1024).(*mediaService)
	svc.limiter = &mockStorageLimiter{getEffectiveLimitsFn: func(context.Context, string, string) (int64, int64, int, error) {
		return 0, quota, 0, nil
	}}
	img := tinyPNG(t)
	upload := func(user, campaign string) error {
		_, err := svc.Upload(ctx, UploadInput{CampaignID: campaign, UploadedBy: user, OriginalName: "p.png",
			MimeType: "image/png", FileSize: int64(len(img)), UsageType: UsageNoteImage, FileBytes: img})
		return err
	}

	// Ana: two pictures (2048 bytes) -> room for one more small one.
	seedNotePicture(t, db, camp, ana)
	seedNotePicture(t, db, camp, ana)
	if err := upload(ana, camp); err != nil {
		t.Fatalf("under the cap, upload refused: %v", err)
	}

	// Bound or not makes no difference: Ana is now at 2048 + len(img), and a
	// third seeded row pushes her past 3000.
	seedNotePicture(t, db, camp, ana)
	assertMediaAppError(t, upload(ana, camp), 400)

	// Bo has nothing, so is not affected by Ana's pictures.
	if err := upload(bo, camp); err != nil {
		t.Fatalf("another member must keep their own space: %v", err)
	}
	// Ana's pictures in this campaign do not count in another one.
	if err := upload(ana, otherCamp); err != nil {
		t.Fatalf("the cap is per campaign: %v", err)
	}
	// Non-note rows of the uploader do not count.
	for i := 0; i < 5; i++ {
		seedADR058MediaFile(t, db, camp, bo)
	}
	used, n, err := NewMediaRepository(db).GetUserNoteImageUsage(ctx, camp, bo)
	if err != nil || n != 1 || used != int64(len(img)) {
		t.Errorf("Bo's note picture usage = %d bytes / %d files (err %v), want %d / 1", used, n, err, len(img))
	}
}
