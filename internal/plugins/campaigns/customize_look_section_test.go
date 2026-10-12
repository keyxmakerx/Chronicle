package campaigns

import (
	"context"
	"encoding/json"
	"net/http"
	"reflect"
	"testing"

	"github.com/keyxmakerx/chronicle/internal/apperror"
)

func TestLookSection_ApplyMatchesAppearanceAPI(t *testing.T) {
	storedBackdrop := "stored-backdrop.png"
	cases := []struct {
		name      string
		role      Role
		settings  string
		backdrop  *string
		input     map[string]any
		owns      bool
		wantCode  int
		wantSaved int
	}{
		{"non-owner forbidden", RoleScribe, "", nil, map[string]any{}, true, http.StatusForbidden, 0},
		{"owner without pictures", RoleOwner, "", nil, map[string]any{}, false, 0, 1},
		{"unowned picture refused", RoleOwner, "", nil,
			map[string]any{"brand": map[string]any{"logo": "foreign.png"}}, false, http.StatusBadRequest, 0},
		{"owned picture accepted", RoleOwner, "", nil,
			map[string]any{"brand": map[string]any{"logo": "mine.png"}}, true, 0, 1},
		{"stored backdrop skips the ownership check", RoleOwner, "", &storedBackdrop,
			map[string]any{"brand": map[string]any{"backdrop": storedBackdrop}}, false, 0, 1},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			svc := &appearanceSvcStub{}
			h := NewHandler(svc)
			h.SetMediaUploader(&appearanceMediaStub{owns: tc.owns})
			audit := &auditRecorder{}
			h.SetAuditLogger(audit)

			raw, _ := json.Marshal(map[string]any{"draft": map[string]any{"ui": 1}, "input": tc.input, "_changes": []string{"Logo"}})
			rem, err := h.applyLookSection(context.Background(), ownerContext(tc.role, tc.settings, tc.backdrop), "owner-1", raw)

			if tc.wantCode != 0 {
				if apperror.SafeCode(err) != tc.wantCode {
					t.Fatalf("err = %v, want code %d", err, tc.wantCode)
				}
			} else if err != nil || rem != nil {
				t.Fatalf("remaining %s, err %v", rem, err)
			}
			if svc.saved != tc.wantSaved {
				t.Errorf("saved = %d, want %d", svc.saved, tc.wantSaved)
			}
			if tc.wantSaved == 1 && len(audit.events) != 1 {
				t.Errorf("audit events = %v", audit.events)
			}
		})
	}
}

func TestLookSection_DecodesInputLikeTheAPI(t *testing.T) {
	// The same JSON body through both paths must reach SaveAppearance as the
	// same value.
	body := `{"look":"classic","brand":{"logo":"mine.png"},"nav":{"style":"x"}}`

	viaAPI := &appearanceSvcStub{}
	hAPI := NewHandler(viaAPI)
	hAPI.SetMediaUploader(&appearanceMediaStub{owns: true})
	if err := saveRequest(t, hAPI, ownerContext(RoleOwner, "", nil), json.RawMessage(body)); err != nil {
		t.Fatal(err)
	}

	viaSection := &appearanceSvcStub{}
	hSec := NewHandler(viaSection)
	hSec.SetMediaUploader(&appearanceMediaStub{owns: true})
	raw := json.RawMessage(`{"draft":{},"input":` + body + `}`)
	if _, err := hSec.applyLookSection(context.Background(), ownerContext(RoleOwner, "", nil), "o", raw); err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(viaAPI.last, viaSection.last) {
		t.Errorf("API %+v != section %+v", viaAPI.last, viaSection.last)
	}
}

func TestLookSection_NoInputIsNothingToApply(t *testing.T) {
	svc := &appearanceSvcStub{}
	h := NewHandler(svc)
	for _, raw := range []string{`{}`, `{"draft":{"a":1}}`, `{"input":null}`} {
		rem, err := h.applyLookSection(context.Background(), ownerContext(RoleOwner, "", nil), "o", json.RawMessage(raw))
		if err != nil || rem != nil {
			t.Errorf("%s: remaining %s, err %v", raw, rem, err)
		}
	}
	if svc.saved != 0 {
		t.Errorf("saved %d times", svc.saved)
	}
	if _, err := h.applyLookSection(context.Background(), ownerContext(RoleOwner, "", nil), "o", json.RawMessage(`[1]`)); apperror.SafeCode(err) != http.StatusBadRequest {
		t.Errorf("malformed section err = %v", err)
	}
}

func TestLookDraftPictures(t *testing.T) {
	stored := "logo.png"
	cc := ownerContext(RoleOwner, `{"brand_logo":"`+stored+`"}`, nil)
	draftWith := func(input map[string]any) json.RawMessage {
		raw, _ := json.Marshal(map[string]any{"look": map[string]any{"input": input}})
		return raw
	}
	picMedia := &appearanceMediaStub{owns: false}
	// Owns only "mine.png".
	h := NewHandler(&appearanceSvcStub{})
	h.SetMediaUploader(ownsOnly{"mine.png", picMedia})

	tests := []struct {
		name  string
		draft json.RawMessage
		want  map[string]string
	}{
		{"stored picture", draftWith(map[string]any{"brand": map[string]any{"logo": stored}}),
			map[string]string{stored: "/media/" + stored}},
		{"owned upload", draftWith(map[string]any{"brand": map[string]any{"backdrop": "mine.png"}}),
			map[string]string{"mine.png": "/media/mine.png"}},
		{"another campaign's file is left out", draftWith(map[string]any{"brand": map[string]any{"backdrop": "foreign.png"}}),
			map[string]string{}},
		{"mixed keeps only the safe ones", draftWith(map[string]any{
			"brand": map[string]any{"logo": stored, "backdrop": "foreign.png"}, "sidebar": map[string]any{"banner": "mine.png"}}),
			map[string]string{stored: "/media/" + stored, "mine.png": "/media/mine.png"}},
		{"no look section", json.RawMessage(`{"categories":{}}`), map[string]string{}},
		{"empty draft", json.RawMessage(`{}`), map[string]string{}},
		{"garbage draft", json.RawMessage(`nope`), map[string]string{}},
		{"look without input", json.RawMessage(`{"look":{"draft":{}}}`), map[string]string{}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := h.LookDraftPictures(context.Background(), cc, tt.draft)
			if !reflect.DeepEqual(got, tt.want) {
				t.Errorf("got %v, want %v", got, tt.want)
			}
		})
	}

	t.Run("no uploader still serves stored pictures only", func(t *testing.T) {
		bare := NewHandler(&appearanceSvcStub{})
		got := bare.LookDraftPictures(context.Background(), cc, draftWith(map[string]any{
			"brand": map[string]any{"logo": stored, "backdrop": "mine.png"}}))
		if !reflect.DeepEqual(got, map[string]string{stored: "/media/" + stored}) {
			t.Errorf("got %v", got)
		}
	})
}

// ownsOnly is a MediaUploader that owns exactly one file name.
type ownsOnly struct {
	name string
	*appearanceMediaStub
}

func (o ownsOnly) OwnsFile(_ context.Context, _, filename string) (bool, error) {
	return filename == o.name, nil
}
