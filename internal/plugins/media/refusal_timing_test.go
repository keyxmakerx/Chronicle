package media

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/labstack/echo/v4"

	"github.com/keyxmakerx/chronicle/internal/apperror"
)

// slowFoundMediaService makes finding a real file take longer than missing
// one, a wider gap than any real lookup, so the test sees whether it leaks.
type slowFoundMediaService struct {
	idKeyedMediaService
	gap time.Duration
}

func (s *slowFoundMediaService) GetByID(ctx context.Context, id string) (*MediaFile, error) {
	f, err := s.idKeyedMediaService.GetByID(ctx, id)
	if err == nil {
		time.Sleep(s.gap)
	}
	return f, err
}

func timedAnonymousServe(t *testing.T, h *Handler, id string, thumb bool) (time.Duration, *apperror.AppError) {
	t.Helper()
	path := "/media/" + id
	if thumb {
		path += "/thumb/300"
	}
	c := echo.New().NewContext(httptest.NewRequest(http.MethodGet, path, nil), httptest.NewRecorder())
	c.SetParamNames("id", "size")
	c.SetParamValues(id, "300")
	start := time.Now()
	var err error
	if thumb {
		err = h.ServeThumbnail(c)
	} else {
		err = h.Serve(c)
	}
	elapsed := time.Since(start)
	appErr, ok := err.(*apperror.AppError)
	if !ok {
		t.Fatalf("serve %q: want an AppError refusal, got %T %v", id, err, err)
	}
	return elapsed, appErr
}

// A refusal of a real private file and of an unknown id leave at the same
// moment, so timing can't confirm that a guessed media id is real.
func TestServe_RefusalTimeDoesNotRevealARealFile(t *testing.T) {
	const floor, gap = 100 * time.Millisecond, 40 * time.Millisecond
	h := &Handler{
		signer:        NewURLSigner("test-secret"),
		memberChecker: &stubMemberChecker{},
		service: &slowFoundMediaService{
			idKeyedMediaService: idKeyedMediaService{files: map[string]*MediaFile{"real-private": privateMediaFile()}},
			gap:                 gap,
		},
		entityVisibility: &fakeEntityVisibilityFilter{},
		refusalFloor:     floor,
	}
	for _, thumb := range []bool{false, true} {
		real, realErr := timedAnonymousServe(t, h, "real-private", thumb)
		unknown, unknownErr := timedAnonymousServe(t, h, "no-such-id", thumb)
		if realErr.Code != http.StatusNotFound || unknownErr.Code != http.StatusNotFound {
			t.Fatalf("thumb=%v: want two 404s, got %d and %d", thumb, realErr.Code, unknownErr.Code)
		}
		if real < floor || unknown < floor {
			t.Errorf("thumb=%v: refusals answered before the floor: real %v, unknown %v", thumb, real, unknown)
		}
		if d := real - unknown; d > gap*3/4 || d < -gap*3/4 {
			t.Errorf("thumb=%v: a real file's refusal took %v, an unknown id's %v; the lookup gap shows", thumb, real, unknown)
		}
	}
}

// Only "not found" is held; any other answer goes at once.
func TestHoldRefusal_HoldsOnlyNotFound(t *testing.T) {
	h := &Handler{refusalFloor: 2 * time.Second}
	c := echo.New().NewContext(httptest.NewRequest(http.MethodGet, "/media/x", nil), httptest.NewRecorder())
	for _, err := range []error{nil, apperror.NewForbidden("signed URL required"), apperror.NewBadRequest("invalid thumbnail size")} {
		start := time.Now()
		if got := h.holdRefusal(c, start, err); got != err {
			t.Fatalf("holdRefusal changed the error: %v -> %v", err, got)
		}
		if time.Since(start) > time.Second {
			t.Errorf("holdRefusal held %v", err)
		}
	}
}

func TestNewHandler_SetsTheRefusalFloor(t *testing.T) {
	if h := NewHandler(nil); h.refusalFloor != mediaRefusalFloor || mediaRefusalFloor <= 0 {
		t.Fatalf("refusalFloor = %v, want %v", h.refusalFloor, mediaRefusalFloor)
	}
}
