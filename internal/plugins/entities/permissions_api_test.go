// permissions_api_test.go — pins the C-PERMISSIONS-SAVE-FIX
// invariants on both the wrap-and-preserve helper and the wire-
// shape responder. The original bug (cordinator Issue #5) hid every
// stage's failure as the generic "An unexpected error occurred"
// because every sub-error was smothered as apperror.NewInternal.
// These tests fail if that regression returns OR if the wire-
// contract shape regresses to the legacy { error, message } pair.
package entities

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/labstack/echo/v4"

	"github.com/keyxmakerx/chronicle/internal/apperror"
)

// TestWrapPermissionError_PreservesTypedAppError pins the
// load-bearing fix: a typed *AppError sub-cause (e.g. NotFound
// from UpdateVisibility hitting a row deleted by a concurrent
// client) must NOT be smothered as Internal. Smothering is the
// exact bug the operator hit.
func TestWrapPermissionError_PreservesTypedAppError(t *testing.T) {
	cases := []struct {
		name      string
		cause     error
		wantType  string
		wantCode  int
	}{
		{"NotFound passes through", apperror.NewNotFound("entity not found"), "not_found", http.StatusNotFound},
		{"BadRequest passes through", apperror.NewBadRequest("permission 0: invalid subject type \"x\""), "bad_request", http.StatusBadRequest},
		{"Validation passes through", apperror.NewValidation("subject_id required"), "validation_error", http.StatusUnprocessableEntity},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			wrapped := wrapPermissionError(context.Background(), "test stage", tc.cause)
			var ae *apperror.AppError
			if !errors.As(wrapped, &ae) {
				t.Fatalf("wrapped is not *AppError: %T", wrapped)
			}
			if ae.Type != tc.wantType {
				t.Errorf("Type = %q, want %q — typed AppError should pass through unchanged",
					ae.Type, tc.wantType)
			}
			if ae.Code != tc.wantCode {
				t.Errorf("Code = %d, want %d", ae.Code, tc.wantCode)
			}
		})
	}
}

// TestWrapPermissionError_WrapsUntypedWithStage pins the second
// part of the fix: an untyped (raw DB) error must come out as a
// typed Internal AppError whose Message names the stage. Without
// the stage hint the operator's only diagnostic was the generic
// "An unexpected error occurred" text — Issue #5.
func TestWrapPermissionError_WrapsUntypedWithStage(t *testing.T) {
	raw := errors.New("Error 1062 (23000): Duplicate entry 'ent-1-role-2-edit' for key 'uq_entity_perm'")
	wrapped := wrapPermissionError(context.Background(), "writing grant rows", raw)

	var ae *apperror.AppError
	if !errors.As(wrapped, &ae) {
		t.Fatalf("wrapped is not *AppError: %T", wrapped)
	}
	if ae.Type != "internal_error" {
		t.Errorf("Type = %q, want %q (untyped raw error should become Internal)",
			ae.Type, "internal_error")
	}
	if ae.Code != http.StatusInternalServerError {
		t.Errorf("Code = %d, want 500", ae.Code)
	}
	// The Message must include the stage so the wire response
	// distinguishes failure-during-grant-write from failure-during-
	// visibility-flip. This is the explicit Issue #5 user-facing
	// improvement.
	wantStage := "writing grant rows"
	if !containsSubstring(ae.Message, wantStage) {
		t.Errorf("Message does not name the stage:\n  got:  %q\n  want substring: %q",
			ae.Message, wantStage)
	}
	// The underlying cause must be wrapped in Internal for log
	// access via errors.Unwrap.
	if !errors.Is(wrapped, raw) {
		t.Error("wrapped error does not chain to raw cause — operator log loses the underlying error")
	}
}

// TestWrapPermissionError_DoesNotEmitGenericText pins the
// regression itself: the wire body must NOT contain the
// "An unexpected error occurred. Please try again." text that
// matches apperror.NewInternal's default Message verbatim. If
// this string ever shows up in the wire body for a wrapped error,
// we've regressed to the Issue #5 state.
func TestWrapPermissionError_DoesNotEmitGenericText(t *testing.T) {
	raw := errors.New("db boom")
	wrapped := wrapPermissionError(context.Background(), "switching visibility to custom", raw)
	var ae *apperror.AppError
	if !errors.As(wrapped, &ae) {
		t.Fatalf("wrapped is not *AppError: %T", wrapped)
	}
	const generic = "An unexpected error occurred. Please try again."
	if ae.Message == generic {
		t.Errorf("Message regressed to the generic text the operator reported in Issue #5: %q\n"+
			"wrapPermissionError must include the stage in Message so users see what failed.",
			ae.Message)
	}
}

// TestRespondPermissionsError_WireContractShape pins the
// { error, message, category } wire shape on the permissions
// endpoint. Renders a 422 BadRequest through respondPermissionsError
// and asserts every required key is present with the right value.
func TestRespondPermissionsError_WireContractShape(t *testing.T) {
	cases := []struct {
		name         string
		ae           *apperror.AppError
		wantCategory string
		wantStatus   int
	}{
		{"BadRequest -> validation", apperror.NewBadRequest("bad payload"), "validation", http.StatusBadRequest},
		{"Validation -> validation", apperror.NewValidation("subject_id required"), "validation", http.StatusUnprocessableEntity},
		{"NotFound -> not_found", apperror.NewNotFound("entity not found"), "not_found", http.StatusNotFound},
		{"Internal -> internal", apperror.NewInternal(errors.New("db boom")), "internal", http.StatusInternalServerError},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			e := echo.New()
			req := httptest.NewRequest(http.MethodPut, "/campaigns/c1/entities/e1/permissions", nil)
			rec := httptest.NewRecorder()
			c := e.NewContext(req, rec)

			if err := respondPermissionsError(c, tc.ae); err != nil {
				t.Fatalf("respondPermissionsError returned error: %v", err)
			}
			if rec.Code != tc.wantStatus {
				t.Fatalf("status = %d, want %d; body=%s", rec.Code, tc.wantStatus, rec.Body.String())
			}
			var body map[string]string
			if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
				t.Fatalf("body did not unmarshal as map[string]string: %v\nbody: %s", err, rec.Body.String())
			}
			for _, k := range []string{"error", "message", "category"} {
				if _, ok := body[k]; !ok {
					t.Errorf("response body missing required wire-contract key %q\nbody: %v", k, body)
				}
			}
			if body["category"] != tc.wantCategory {
				t.Errorf("category = %q, want %q", body["category"], tc.wantCategory)
			}
			if body["error"] != tc.ae.Type {
				t.Errorf("error code = %q, want %q (AppError.Type)", body["error"], tc.ae.Type)
			}
			if body["message"] != tc.ae.Message {
				t.Errorf("message = %q, want %q", body["message"], tc.ae.Message)
			}
		})
	}
}

// TestRespondPermissionsError_PassesUntypedThrough pins the
// safety net: an untyped error must NOT get a wire-shape body
// (it would mask the framework error handler's own logging /
// rendering). Untyped errors are returned unchanged.
func TestRespondPermissionsError_PassesUntypedThrough(t *testing.T) {
	e := echo.New()
	req := httptest.NewRequest(http.MethodPut, "/x", nil)
	rec := httptest.NewRecorder()
	c := e.NewContext(req, rec)

	raw := errors.New("raw db error")
	got := respondPermissionsError(c, raw)
	if got != raw {
		t.Errorf("untyped error should be returned unchanged so the framework handler logs it.\n  got:  %v\n  want: %v", got, raw)
	}
	if rec.Code != http.StatusOK {
		t.Errorf("recorder Code should remain unset (default 200) when error passes through; got %d", rec.Code)
	}
}

// --- service-level test: the original Issue #5 user-visible bug
//     was that EVERY stage's failure looked like a generic Internal.
//     This test pins that VisibilityCustom's UpdateVisibility path,
//     when its sub-error is a typed NotFound (e.g. entity row deleted
//     by a concurrent client between SetPermissions and UpdateVisibility),
//     surfaces as a typed NotFound — not Internal.

func TestSetEntityPermissions_PreservesNotFoundFromUpdateVisibility(t *testing.T) {
	entityRepo := &mockEntityRepo{
		findByIDFn: func(_ context.Context, id string) (*Entity, error) {
			return &Entity{ID: id, CampaignID: "camp-1", Visibility: VisibilityDefault}, nil
		},
	}
	typeRepo := &mockEntityTypeRepo{}
	permRepo := &mockPermissionRepo{
		setPermissionsFn: func(_ context.Context, _ string, _ []PermissionGrant) error {
			return nil
		},
		updateVisibilityFn: func(_ context.Context, _ string, _ VisibilityMode) error {
			// Simulate: row was deleted between SetPermissions and
			// UpdateVisibility, so the UPDATE matched zero rows and
			// the repo correctly returned NotFound.
			return apperror.NewNotFound("entity not found")
		},
	}
	svc := NewEntityService(entityRepo, typeRepo, permRepo)
	err := svc.SetEntityPermissions(context.Background(), "ent-1", SetPermissionsInput{
		Visibility:  VisibilityCustom,
		Permissions: []PermissionGrant{{SubjectType: SubjectRole, SubjectID: "1", Permission: PermView}},
	})
	if err == nil {
		t.Fatal("expected an error from UpdateVisibility NotFound")
	}
	var ae *apperror.AppError
	if !errors.As(err, &ae) {
		t.Fatalf("error is not *AppError: %T (%v)", err, err)
	}
	if ae.Type != "not_found" {
		t.Errorf("Type = %q, want %q — Issue #5 regression: typed sub-error smothered as Internal again",
			ae.Type, "not_found")
	}
	if ae.Code != http.StatusNotFound {
		t.Errorf("Code = %d, want 404", ae.Code)
	}
}

// TestSetEntityPermissions_WrapsRawDBErrorWithStage pins the
// other side: an UNTYPED (raw) DB error gets wrapped with a
// stage-naming Message rather than the generic Internal text the
// operator reported.
func TestSetEntityPermissions_WrapsRawDBErrorWithStage(t *testing.T) {
	entityRepo := &mockEntityRepo{
		findByIDFn: func(_ context.Context, id string) (*Entity, error) {
			return &Entity{ID: id, CampaignID: "camp-1", Visibility: VisibilityDefault}, nil
		},
	}
	typeRepo := &mockEntityTypeRepo{}
	permRepo := &mockPermissionRepo{
		setPermissionsFn: func(_ context.Context, _ string, _ []PermissionGrant) error {
			return errors.New("ERROR 1062 (23000): Duplicate entry")
		},
	}
	svc := NewEntityService(entityRepo, typeRepo, permRepo)
	err := svc.SetEntityPermissions(context.Background(), "ent-1", SetPermissionsInput{
		Visibility:  VisibilityCustom,
		Permissions: []PermissionGrant{{SubjectType: SubjectRole, SubjectID: "1", Permission: PermView}},
	})
	if err == nil {
		t.Fatal("expected an error")
	}
	var ae *apperror.AppError
	if !errors.As(err, &ae) {
		t.Fatalf("error is not *AppError: %T", err)
	}
	if ae.Type != "internal_error" {
		t.Errorf("Type = %q, want internal_error", ae.Type)
	}
	if !containsSubstring(ae.Message, "writing grant rows") {
		t.Errorf("Message does not name the stage:\n  got: %q\n  want substring: %q",
			ae.Message, "writing grant rows")
	}
	// The wire user no longer sees the verbatim generic text.
	if ae.Message == "An unexpected error occurred. Please try again." {
		t.Error("Issue #5 regression: wrapped raw error reverted to the generic Internal Message")
	}
}

// TestSetPermissionsAPI_EmitsWireContractOnError pins the wire-
// shape on the HTTP boundary for a representative failure path:
// invalid JSON body → 400 BadRequest → wire body has all three
// required keys with the validation category.
func TestSetPermissionsAPI_EmitsWireContractOnError(t *testing.T) {
	// We exercise the handler path that bails BEFORE service-level
	// dependencies are reached (invalid JSON), so we can use a
	// nearly-empty Handler. The campaign-context middleware is
	// what'd normally set cc; we skip that and let the nil-check
	// fail through the wire-shape path.
	h := &Handler{}

	e := echo.New()
	req := httptest.NewRequest(http.MethodPut, "/campaigns/c1/entities/e1/permissions",
		bytes.NewReader([]byte(`not json`)))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	c := e.NewContext(req, rec)
	c.SetParamNames("id", "eid")
	c.SetParamValues("c1", "e1")

	if err := h.SetPermissionsAPI(c); err != nil {
		t.Fatalf("handler returned error (should have been written to rec): %v", err)
	}
	// Without a campaign context, the handler bails at the cc==nil
	// guard, which now flows through respondPermissionsError too.
	var body map[string]string
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatalf("body did not unmarshal: %v\nbody: %s", err, rec.Body.String())
	}
	for _, k := range []string{"error", "message", "category"} {
		if _, ok := body[k]; !ok {
			t.Errorf("wire-contract key %q missing from error response\nbody: %v", k, body)
		}
	}
}

// containsSubstring is a tiny local helper to avoid importing
// strings just for one Contains call.
func containsSubstring(haystack, needle string) bool {
	if len(needle) == 0 {
		return true
	}
	for i := 0; i+len(needle) <= len(haystack); i++ {
		if haystack[i:i+len(needle)] == needle {
			return true
		}
	}
	return false
}
