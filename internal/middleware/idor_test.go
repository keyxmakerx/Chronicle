package middleware

import (
	"context"
	"errors"
	"net/http"
	"testing"

	"github.com/labstack/echo/v4"
)

// testResource is a mock model that implements CampaignScoped.
type testResource struct {
	ID         string
	campaignID string
}

func (r *testResource) GetCampaignID() string { return r.campaignID }

func TestRequireInCampaign_Success(t *testing.T) {
	resource := &testResource{ID: "res-1", campaignID: "camp-1"}
	fetchFn := func(ctx context.Context, id string) (*testResource, error) {
		if id == "res-1" {
			return resource, nil
		}
		return nil, echo.NewHTTPError(http.StatusNotFound, "not found")
	}

	result, err := RequireInCampaign(context.Background(), fetchFn, "res-1", "camp-1", "resource")
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
	if result.ID != "res-1" {
		t.Errorf("expected ID res-1, got %s", result.ID)
	}
}

func TestRequireInCampaign_WrongCampaign(t *testing.T) {
	resource := &testResource{ID: "res-1", campaignID: "camp-2"}
	fetchFn := func(ctx context.Context, id string) (*testResource, error) {
		return resource, nil
	}

	_, err := RequireInCampaign(context.Background(), fetchFn, "res-1", "camp-1", "resource")
	if err == nil {
		t.Fatal("expected error, got nil")
	}

	httpErr, ok := err.(*echo.HTTPError)
	if !ok {
		t.Fatalf("expected *echo.HTTPError, got %T", err)
	}
	if httpErr.Code != http.StatusNotFound {
		t.Errorf("expected 404, got %d", httpErr.Code)
	}
}

func TestRequireInCampaign_FetchError(t *testing.T) {
	fetchErr := errors.New("db connection lost")
	fetchFn := func(ctx context.Context, id string) (*testResource, error) {
		return nil, fetchErr
	}

	_, err := RequireInCampaign(context.Background(), fetchFn, "res-1", "camp-1", "resource")
	if err == nil {
		t.Fatal("expected error, got nil")
	}
	if !errors.Is(err, fetchErr) {
		t.Errorf("expected fetch error, got %v", err)
	}
}
