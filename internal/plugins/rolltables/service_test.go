package rolltables

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strings"
	"testing"

	"github.com/keyxmakerx/chronicle/internal/apperror"
)

// fakeRepo stores raw bytes per campaign, like the real table.
type fakeRepo struct {
	rows   map[string][]byte
	puts   int
	failOn bool
}

func newFakeRepo() *fakeRepo { return &fakeRepo{rows: map[string][]byte{}} }

func (f *fakeRepo) Get(_ context.Context, campaignID string) ([]byte, error) {
	if f.failOn {
		return nil, errors.New("boom")
	}
	return f.rows[campaignID], nil
}

func (f *fakeRepo) Put(_ context.Context, campaignID string, data []byte, _ string) error {
	if f.failOn {
		return errors.New("boom")
	}
	f.puts++
	f.rows[campaignID] = data
	return nil
}

func errCode(err error) int {
	var ae *apperror.AppError
	if errors.As(err, &ae) {
		return ae.Code
	}
	return 0
}

func tableJSON(id, name string, entries int) string {
	es := make([]string, entries)
	for i := range es {
		es[i] = fmt.Sprintf(`{"name":"e%d"}`, i)
	}
	return fmt.Sprintf(`{"id":%q,"name":%q,"entries":[%s]}`, id, name, strings.Join(es, ","))
}

func TestService_PutValidation(t *testing.T) {
	manyTables := make([]string, MaxTables+1)
	for i := range manyTables {
		manyTables[i] = tableJSON(fmt.Sprintf("t%d", i), "T", 1)
	}
	tests := []struct {
		name     string
		body     string
		wantCode int
	}{
		{"ok minimal", `{"tables":[` + tableJSON("loot", "Loot", 1) + `]}`, 0},
		{"ok empty set", `{"tables":[]}`, 0},
		{"ok max tables", `{"tables":[` + strings.Join(manyTables[:MaxTables], ",") + `]}`, 0},
		{"too many tables", `{"tables":[` + strings.Join(manyTables, ",") + `]}`, http.StatusUnprocessableEntity},
		{"not json", `nope`, http.StatusUnprocessableEntity},
		{"tables missing", `{}`, http.StatusUnprocessableEntity},
		{"tables null", `{"tables":null}`, http.StatusUnprocessableEntity},
		{"body too large", `{"tables":[],"pad":"` + strings.Repeat("x", MaxBodyBytes) + `"}`, http.StatusUnprocessableEntity},
		{"id uppercase", `{"tables":[` + tableJSON("Loot", "Loot", 1) + `]}`, http.StatusUnprocessableEntity},
		{"id starts with digit", `{"tables":[` + tableJSON("1loot", "Loot", 1) + `]}`, http.StatusUnprocessableEntity},
		{"id empty", `{"tables":[` + tableJSON("", "Loot", 1) + `]}`, http.StatusUnprocessableEntity},
		{"id too long", `{"tables":[` + tableJSON("a"+strings.Repeat("b", 64), "Loot", 1) + `]}`, http.StatusUnprocessableEntity},
		{"id 64 ok", `{"tables":[` + tableJSON("a"+strings.Repeat("b", 63), "Loot", 1) + `]}`, 0},
		{"duplicate id", `{"tables":[` + tableJSON("loot", "A", 1) + `,` + tableJSON("loot", "B", 1) + `]}`, http.StatusUnprocessableEntity},
		{"name blank", `{"tables":[` + tableJSON("loot", "   ", 1) + `]}`, http.StatusUnprocessableEntity},
		{"name too long", `{"tables":[` + tableJSON("loot", strings.Repeat("n", MaxTableNameLen+1), 1) + `]}`, http.StatusUnprocessableEntity},
		{"no entries", `{"tables":[{"id":"loot","name":"Loot","entries":[]}]}`, http.StatusUnprocessableEntity},
		{"entries missing", `{"tables":[{"id":"loot","name":"Loot"}]}`, http.StatusUnprocessableEntity},
		{"500 entries ok", `{"tables":[` + tableJSON("loot", "Loot", MaxEntries) + `]}`, 0},
		{"501 entries", `{"tables":[` + tableJSON("loot", "Loot", MaxEntries+1) + `]}`, http.StatusUnprocessableEntity},
		{"entry name blank", `{"tables":[{"id":"loot","name":"Loot","entries":[{"name":" "}]}]}`, http.StatusUnprocessableEntity},
		{"entry name too long", `{"tables":[{"id":"loot","name":"Loot","entries":[{"name":"` + strings.Repeat("n", MaxEntryNameLen+1) + `"}]}]}`, http.StatusUnprocessableEntity},
		{"brief too long", `{"tables":[{"id":"loot","name":"Loot","entries":[{"name":"a","brief":"` + strings.Repeat("b", MaxEntryBriefLen+1) + `"}]}]}`, http.StatusUnprocessableEntity},
		{"brief at limit", `{"tables":[{"id":"loot","name":"Loot","entries":[{"name":"a","brief":"` + strings.Repeat("b", MaxEntryBriefLen) + `"}]}]}`, 0},
		{"weight zero ok", `{"tables":[{"id":"loot","name":"Loot","entries":[{"name":"a","weight":0}]}]}`, 0},
		{"weight 100 ok", `{"tables":[{"id":"loot","name":"Loot","entries":[{"name":"a","weight":100}]}]}`, 0},
		{"weight negative", `{"tables":[{"id":"loot","name":"Loot","entries":[{"name":"a","weight":-1}]}]}`, http.StatusUnprocessableEntity},
		{"weight too big", `{"tables":[{"id":"loot","name":"Loot","entries":[{"name":"a","weight":100.5}]}]}`, http.StatusUnprocessableEntity},
		{"weight a string", `{"tables":[{"id":"loot","name":"Loot","entries":[{"name":"a","weight":"2"}]}]}`, http.StatusUnprocessableEntity},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			repo := newFakeRepo()
			_, err := NewService(repo).Put(context.Background(), "camp-1", []byte(tt.body), "u1")
			if got := errCode(err); got != tt.wantCode {
				t.Fatalf("code = %d (err %v), want %d", got, err, tt.wantCode)
			}
			if tt.wantCode != 0 && repo.puts != 0 {
				t.Errorf("rejected body was stored")
			}
		})
	}
}

// Unknown fields are dropped, names are trimmed and a missing weight becomes
// 1, so the stored bytes are the normalized document and not the raw body.
func TestService_PutNormalizes(t *testing.T) {
	repo := newFakeRepo()
	svc := NewService(repo)
	body := `{"extra":1,"tables":[{"id":"loot","name":"  Loot  ","x":true,"entries":[{"name":" Gold ","brief":"","weight":2.5,"junk":"j"},{"name":"Gem"}]}]}`
	got, err := svc.Put(context.Background(), "camp-1", []byte(body), "u1")
	if err != nil {
		t.Fatal(err)
	}
	want := `{"tables":[{"id":"loot","name":"Loot","entries":[{"name":"Gold","brief":"","weight":2.5},{"name":"Gem","brief":"","weight":1}]}]}`
	if stored := string(repo.rows["camp-1"]); stored != want {
		t.Errorf("stored = %s\nwant   = %s", stored, want)
	}
	out, _ := json.Marshal(got)
	if string(out) != want {
		t.Errorf("returned = %s", out)
	}
}

func TestService_GetAndScoping(t *testing.T) {
	repo := newFakeRepo()
	svc := NewService(repo)
	ctx := context.Background()

	doc, err := svc.Get(ctx, "camp-1")
	if err != nil {
		t.Fatal(err)
	}
	if out, _ := json.Marshal(doc); string(out) != `{"tables":[]}` {
		t.Errorf("empty read = %s", out)
	}

	if _, err := svc.Put(ctx, "camp-1", []byte(`{"tables":[`+tableJSON("loot", "Loot", 1)+`]}`), ""); err != nil {
		t.Fatal(err)
	}
	if doc, _ := svc.Get(ctx, "camp-1"); len(doc.Tables) != 1 {
		t.Errorf("camp-1 tables = %d, want 1", len(doc.Tables))
	}
	if doc, _ := svc.Get(ctx, "camp-2"); len(doc.Tables) != 0 {
		t.Errorf("another campaign saw %d tables", len(doc.Tables))
	}

	// PUT replaces the whole set.
	if _, err := svc.Put(ctx, "camp-1", []byte(`{"tables":[]}`), ""); err != nil {
		t.Fatal(err)
	}
	if doc, _ := svc.Get(ctx, "camp-1"); len(doc.Tables) != 0 {
		t.Errorf("replace left %d tables", len(doc.Tables))
	}
}

func TestService_RepoFailureIsInternal(t *testing.T) {
	repo := newFakeRepo()
	repo.failOn = true
	svc := NewService(repo)
	if _, err := svc.Get(context.Background(), "c"); errCode(err) != http.StatusInternalServerError {
		t.Errorf("get code = %d", errCode(err))
	}
	if _, err := svc.Put(context.Background(), "c", []byte(`{"tables":[]}`), ""); errCode(err) != http.StatusInternalServerError {
		t.Errorf("put code = %d", errCode(err))
	}
}
