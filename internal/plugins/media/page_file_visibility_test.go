package media

import (
	"bytes"
	"net/http"
	"testing"

	"github.com/keyxmakerx/chronicle/internal/apperror"
	"github.com/keyxmakerx/chronicle/internal/plugins/campaigns"
)

// A body that does not say gmOnly must change nothing: the old plain bool read
// a missing or misspelled field as false and published a hidden file.
func TestPageFileHandler_SetVisibilityBody(t *testing.T) {
	tests := []struct {
		name     string
		body     string
		wantCode int
		wantGM   bool
	}{
		{"empty object", `{}`, http.StatusBadRequest, true},
		{"snake case spelling", `{"gm_only":false}`, http.StatusBadRequest, true},
		{"snake case true", `{"gm_only":true}`, http.StatusBadRequest, true},
		{"null", `{"gmOnly":null}`, http.StatusBadRequest, true},
		{"camel case false", `{"gmOnly":false}`, http.StatusOK, false},
		{"camel case true", `{"gmOnly":true}`, http.StatusOK, true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			f := newPageFileFixture(t)
			h := NewPageFileHandler(f.svc, f.media)
			pf := f.attach(t, owner, campA, pgOpen, "a.txt", []byte("hi"), true)

			c, rec := pageFileCtx(http.MethodPut, "/x", bytes.NewBufferString(tt.body), "application/json", "owner",
				campaigns.RoleOwner, map[string]string{"eid": pgOpen, "fid": pf.ID})
			err := h.SetVisibility(c)
			got := rec.Code
			if err != nil {
				got = apperror.SafeCode(err)
			}
			if got != tt.wantCode {
				t.Fatalf("status = %d (%v), want %d", got, err, tt.wantCode)
			}
			if f.binds.files[pf.ID].GMOnly != tt.wantGM {
				t.Errorf("GMOnly after = %v, want %v", f.binds.files[pf.ID].GMOnly, tt.wantGM)
			}
		})
	}
}

func TestPageFileHandler_UploadGMOnlyField(t *testing.T) {
	tests := []struct {
		name     string
		fields   map[string]string
		wantCode int
		wantGM   bool
	}{
		{"absent", nil, http.StatusCreated, false},
		{"checkbox on", map[string]string{"gmOnly": "on"}, http.StatusCreated, true},
		{"true", map[string]string{"gmOnly": "true"}, http.StatusCreated, true},
		{"false", map[string]string{"gmOnly": "false"}, http.StatusCreated, false},
		{"unknown value", map[string]string{"gmOnly": "maybe"}, http.StatusBadRequest, false},
		{"old spelling", map[string]string{"gm_only": "1"}, http.StatusBadRequest, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			f := newPageFileFixture(t)
			h := NewPageFileHandler(f.svc, f.media)
			body, ctype := multipartFile(t, "a.txt", []byte("hi"), tt.fields)
			c, rec := pageFileCtx(http.MethodPost, "/x", body, ctype, "owner", campaigns.RoleOwner, map[string]string{"eid": pgOpen})
			err := h.Upload(c)
			got := rec.Code
			if err != nil {
				got = apperror.SafeCode(err)
			}
			if got != tt.wantCode {
				t.Fatalf("status = %d (%v), want %d", got, err, tt.wantCode)
			}
			if tt.wantCode != http.StatusCreated {
				if len(f.stored) != 0 {
					t.Errorf("a refused upload stored %d files", len(f.stored))
				}
				return
			}
			for _, pf := range f.binds.files {
				if pf.GMOnly != tt.wantGM {
					t.Errorf("GMOnly = %v, want %v", pf.GMOnly, tt.wantGM)
				}
			}
		})
	}
}

// A picture is re-encoded on upload, so its type can differ from the name it
// came in with; the offered name follows the type that is actually sent.
func TestDownloadName_MatchesServedType(t *testing.T) {
	tests := []struct {
		name, mime, want string
	}{
		{"map.webp", "image/png", "map.png"},
		{"map.png", "image/png", "map.png"},
		{"photo.jpeg", "image/jpeg", "photo.jpeg"},
		{"anim.gif", "image/gif", "anim.gif"},
		{"anim.gif", "image/png", "anim.png"},
		{"notes.txt", mimeText, "notes.txt"},
	}
	for _, tt := range tests {
		t.Run(tt.name+" as "+tt.mime, func(t *testing.T) {
			if got := downloadName(&PageFile{Name: tt.name, MimeType: tt.mime}); got != tt.want {
				t.Errorf("downloadName = %q, want %q", got, tt.want)
			}
		})
	}
}
