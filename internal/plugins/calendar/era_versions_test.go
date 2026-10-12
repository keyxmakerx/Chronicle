package calendar

import (
	"context"
	"errors"
	"testing"

	"github.com/keyxmakerx/chronicle/internal/apperror"
	"github.com/keyxmakerx/chronicle/internal/patch"
	"github.com/keyxmakerx/chronicle/internal/permissions"
)

func ip(n int) *int { return &n }

func TestEraIndexAt(t *testing.T) {
	// Embers has no end, so it gives way to Ash; Frost is explicitly over
	// before Thaw begins.
	eras := SortedEras([]Era{
		{ID: 3, Name: "Frost", StartYear: 1100, StartMonth: 1, StartDay: 1, EndYear: ip(1200), EndMonth: ip(1), EndDay: ip(1)},
		{ID: 1, Name: "Embers", StartYear: 1, StartMonth: 1, StartDay: 1},
		{ID: 2, Name: "Ash", StartYear: 841, StartMonth: 1, StartDay: 1},
	})
	if eras[0].ID != 1 || eras[1].ID != 2 || eras[2].ID != 3 {
		t.Fatalf("not sorted oldest first: %+v", eras)
	}
	tests := []struct {
		name    string
		y, m, d int
		want    int // era ID, 0 for none
	}{
		{"before any era", 0, 1, 1, 0},
		{"first era", 500, 1, 1, 1},
		{"an open era gives way to the next", 900, 1, 1, 2},
		{"latest begun era", 1150, 1, 1, 3},
		{"after an era that ended, the open one before it", 1300, 1, 1, 2},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := 0
			if i := EraIndexAt(eras, tt.y, tt.m, tt.d); i >= 0 {
				got = eras[i].ID
			}
			if got != tt.want {
				t.Errorf("got era %d, want %d", got, tt.want)
			}
		})
	}
}

func TestPageErasExtents(t *testing.T) {
	cal := &Calendar{Months: []Month{{Name: "One", Days: 10}}, CurrentYear: 15, CurrentMonth: 1, CurrentDay: 6}
	eras := SortedEras([]Era{
		{ID: 1, StartYear: 1, StartMonth: 1, StartDay: 1},
		{ID: 2, StartYear: 11, StartMonth: 1, StartDay: 1, EndYear: ip(20)},
	})
	pe := &PageEras{Calendar: cal, Eras: eras, Current: &eras[1]}
	x := pe.Extents()
	if x[0].Days != 100 || x[0].Here != -1 || x[0].Span != "From year 1" {
		t.Errorf("open era before another: %+v", x[0])
	}
	if x[1].Days != 100 || x[1].Span != "Year 11 to 20" {
		t.Errorf("ended era: %+v", x[1])
	}
	if x[1].Here < 0.44 || x[1].Here > 0.46 {
		t.Errorf("here = %v, want about 0.45", x[1].Here)
	}
}

// fakeEraRepo records writes and answers scope checks.
type fakeEraRepo struct {
	EraVersionRepository
	eraOK, entOK bool
	saved        bool
	cur          *EntityEraVersion
	gotHTML      string
}

func (f *fakeEraRepo) EraInCampaign(context.Context, int, string) (bool, error) { return f.eraOK, nil }
func (f *fakeEraRepo) EntityInCampaign(context.Context, string, string) (bool, error) {
	return f.entOK, nil
}
func (f *fakeEraRepo) SaveEntry(_ context.Context, _ string, _ int, _, html string, _ *int, _ string) (int, bool, error) {
	f.gotHTML = html
	return 4, f.saved, nil
}
func (f *fakeEraRepo) Get(context.Context, string, int) (*EntityEraVersion, error) { return f.cur, nil }
func (f *fakeEraRepo) SetIdentity(context.Context, string, int, patch.Field[string], patch.Field[string], string) (bool, error) {
	return f.cur != nil, nil
}

type fakeMainCal struct{ err error }

func (f fakeMainCal) GetDefaultCalendarForViewer(context.Context, string, permissions.Viewer) (*Calendar, error) {
	return nil, f.err
}

func TestEraVersionServiceWrites(t *testing.T) {
	ctx := context.Background()
	t.Run("an era or page from another campaign is not found", func(t *testing.T) {
		s := NewEraVersionService(&fakeEraRepo{eraOK: false, entOK: true}, fakeMainCal{})
		_, _, err := s.SaveEntry(ctx, "c1", "e1", 9, `{}`, "<p>x</p>", nil, "u")
		var ae *apperror.AppError
		if !errors.As(err, &ae) || ae.Code != 404 {
			t.Fatalf("got %v, want not found", err)
		}
	})
	t.Run("text is sanitized and must be present", func(t *testing.T) {
		r := &fakeEraRepo{eraOK: true, entOK: true, saved: true}
		s := NewEraVersionService(r, fakeMainCal{})
		if _, _, err := s.SaveEntry(ctx, "c1", "e1", 9, " ", "", nil, "u"); err == nil {
			t.Fatal("empty entry accepted")
		}
		rev, clash, err := s.SaveEntry(ctx, "c1", "e1", 9, `{}`, `<p onclick="x()">hi</p><script>bad()</script>`, nil, "u")
		if err != nil || clash != nil || rev != 4 {
			t.Fatalf("rev=%d clash=%v err=%v", rev, clash, err)
		}
		if r.gotHTML != "<p>hi</p>" {
			t.Errorf("stored HTML not sanitized: %q", r.gotHTML)
		}
	})
	t.Run("a stale save returns the stored text", func(t *testing.T) {
		by := "u2"
		r := &fakeEraRepo{eraOK: true, entOK: true, cur: &EntityEraVersion{EntryRev: 7, UpdatedBy: &by}}
		s := NewEraVersionService(r, fakeMainCal{})
		base := 3
		_, clash, err := s.SaveEntry(ctx, "c1", "e1", 9, `{}`, "<p>x</p>", &base, "u")
		if err != nil || clash == nil || clash.Rev != 7 || clash.ByUserID != "u2" {
			t.Fatalf("clash=%+v err=%v", clash, err)
		}
	})
	t.Run("a name that is too long is refused", func(t *testing.T) {
		r := &fakeEraRepo{eraOK: true, entOK: true, cur: &EntityEraVersion{}}
		s := NewEraVersionService(r, fakeMainCal{})
		long := make([]rune, MaxEraVersionName+1)
		for i := range long {
			long[i] = 'a'
		}
		if err := s.SetIdentity(ctx, "c1", "e1", 9, patch.Of(string(long)), patch.Field[string]{}, "u"); err == nil {
			t.Fatal("long name accepted")
		}
	})
	t.Run("no main calendar means no eras", func(t *testing.T) {
		s := NewEraVersionService(&fakeEraRepo{}, fakeMainCal{err: apperror.NewNotFound("no calendar")})
		pe, err := s.PageEras(ctx, "c1", permissions.Viewer{})
		if err != nil || pe != nil {
			t.Fatalf("pe=%v err=%v", pe, err)
		}
	})
}
