package rolltables

import (
	"context"
	"encoding/json"
	"net/http"
	"strings"
	"testing"
)

func whenBody(entry string) string {
	return `{"tables":[{"id":"omens","name":"Omens","entries":[` + entry + `]}]}`
}

// A line's day conditions and a table's "who sees it" are accepted when the
// engine knows them, rejected in plain words when it does not, and come back
// from storage as they went in.
func TestPut_WhenAndVisibility(t *testing.T) {
	tests := []struct {
		name     string
		body     string
		wantCode int
		wantWhen string
	}{
		{"full moon", whenBody(`{"name":"a","when":{"moon":{"name":"any","phase":"full","within":1}}}`), 0, `{"moon":{"name":"any","phase":"full","within":1}}`},
		{"season", whenBody(`{"name":"a","when":{"season":["winter"]}}`), 0, `{"season":["winter"]}`},
		{"weekday and month by name or number", whenBody(`{"name":"a","when":{"weekday":["Moonday",2],"month":[1,"Frost"]}}`), 0, `{"month":[1,"Frost"],"weekday":["Moonday",2]}`},
		{"not and chance", whenBody(`{"name":"a","when":{"not":{"rest":true},"chance":0.5}}`), 0, `{"chance":0.5,"not":{"rest":true}}`},
		{"empty means any day", whenBody(`{"name":"a","when":{}}`), 0, ``},
		{"null means any day", whenBody(`{"name":"a","when":null}`), 0, ``},
		{"unknown condition", whenBody(`{"name":"a","when":{"tide":["high"]}}`), http.StatusUnprocessableEntity, ""},
		{"unknown moon phase", whenBody(`{"name":"a","when":{"moon":{"phase":"purple"}}}`), http.StatusUnprocessableEntity, ""},
		{"season not a list", whenBody(`{"name":"a","when":{"season":"winter"}}`), http.StatusUnprocessableEntity, ""},
		{"season holds a number", whenBody(`{"name":"a","when":{"season":[3]}}`), http.StatusUnprocessableEntity, ""},
		{"month number out of range", whenBody(`{"name":"a","when":{"month":[0]}}`), http.StatusUnprocessableEntity, ""},
		{"chance above one", whenBody(`{"name":"a","when":{"chance":2}}`), http.StatusUnprocessableEntity, ""},
		{"rest not a bool", whenBody(`{"name":"a","when":{"rest":"yes"}}`), http.StatusUnprocessableEntity, ""},
		{"when not an object", whenBody(`{"name":"a","when":["full"]}`), http.StatusUnprocessableEntity, ""},
		{"moon within too long", whenBody(`{"name":"a","when":{"moon":{"within":99}}}`), http.StatusUnprocessableEntity, ""},
		{"not nested too deep", whenBody(`{"name":"a","when":{"not":{"not":{"not":{"rest":true}}}}}`), http.StatusUnprocessableEntity, ""},
		{"list too long", whenBody(`{"name":"a","when":{"season":[` + strings.Repeat(`"a",`, 40) + `"a"]}}`), http.StatusUnprocessableEntity, ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			svc := NewService(newFakeRepo())
			doc, err := svc.Put(context.Background(), "c1", []byte(tt.body), "u1")
			if tt.wantCode != 0 {
				if errCode(err) != tt.wantCode {
					t.Fatalf("code = %d (err %v), want %d", errCode(err), err, tt.wantCode)
				}
				return
			}
			if err != nil {
				t.Fatalf("put: %v", err)
			}
			if got := string(doc.Tables[0].Entries[0].When); got != tt.wantWhen {
				t.Errorf("stored when = %q, want %q", got, tt.wantWhen)
			}
			again, err := svc.Get(context.Background(), "c1")
			if err != nil || string(again.Tables[0].Entries[0].When) != tt.wantWhen {
				t.Errorf("read back when = %q (err %v), want %q", again.Tables[0].Entries[0].When, err, tt.wantWhen)
			}
		})
	}
}

func TestPut_Visibility(t *testing.T) {
	tests := []struct {
		name     string
		vis      string
		want     string
		wantCode int
	}{
		{"everyone", `"everyone"`, "everyone", 0},
		{"dm only", `"dm_only"`, "dm_only", 0},
		{"left out", ``, "", 0},
		{"something else", `"players"`, "", http.StatusUnprocessableEntity},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			field := ""
			if tt.vis != "" {
				field = `"visibility":` + tt.vis + `,`
			}
			body := `{"tables":[{"id":"omens","name":"Omens",` + field + `"entries":[{"name":"a"}]}]}`
			doc, err := NewService(newFakeRepo()).Put(context.Background(), "c1", []byte(body), "u1")
			if tt.wantCode != 0 {
				if errCode(err) != tt.wantCode {
					t.Fatalf("code = %d (err %v), want %d", errCode(err), err, tt.wantCode)
				}
				return
			}
			if err != nil || doc.Tables[0].Visibility != tt.want {
				t.Fatalf("visibility = %q (err %v), want %q", doc.Tables[0].Visibility, err, tt.want)
			}
		})
	}
}

// Replacing the whole document keeps the conditions of the lines the client
// sends back, and a client that leaves them out clears them: the stored
// document is exactly what was sent.
func TestPut_WholeReplaceKeepsWhat_IsSent(t *testing.T) {
	svc := NewService(newFakeRepo())
	withWhen := whenBody(`{"name":"a","when":{"season":["winter"]}}`)
	if _, err := svc.Put(context.Background(), "c1", []byte(withWhen), "u"); err != nil {
		t.Fatal(err)
	}
	got, _ := svc.Get(context.Background(), "c1")
	b, _ := json.Marshal(got)
	if _, err := svc.Put(context.Background(), "c1", b, "u"); err != nil {
		t.Fatalf("round trip of a stored document: %v", err)
	}
	again, _ := svc.Get(context.Background(), "c1")
	if string(again.Tables[0].Entries[0].When) != `{"season":["winter"]}` {
		t.Errorf("round trip lost the condition: %s", again.Tables[0].Entries[0].When)
	}
	if _, err := svc.Put(context.Background(), "c1", []byte(whenBody(`{"name":"a"}`)), "u"); err != nil {
		t.Fatal(err)
	}
	cleared, _ := svc.Get(context.Background(), "c1")
	if len(cleared.Tables[0].Entries[0].When) != 0 {
		t.Errorf("a line sent without conditions kept %s", cleared.Tables[0].Entries[0].When)
	}
}
