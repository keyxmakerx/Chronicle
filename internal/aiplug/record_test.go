package aiplug

import (
	"strings"
	"testing"
	"time"
)

type upper struct{}

func (upper) HTML(b string) (*string, error) { s := strings.ToUpper(b); return &s, nil }
func (upper) Entry(b string) (*string, *string, error) {
	j, h := "doc", strings.ToUpper(b)
	return &j, &h, nil
}

func TestRecord_BodyEntry(t *testing.T) {
	tests := []struct {
		name    string
		r       Record
		html    string // "" for nil
		wantErr bool
	}{
		{"empty body renders nothing", Record{Body: "  \n", Text: upper{}}, "", false},
		{"empty body needs no Text", Record{Body: ""}, "", false},
		{"body goes through Text", Record{Body: "hi", Text: upper{}}, "HI", false},
		{"body without Text is refused", Record{Body: "hi"}, "", true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, h, err := tt.r.BodyEntry()
			if (err != nil) != tt.wantErr {
				t.Fatalf("err = %v", err)
			}
			got := ""
			if h != nil {
				got = *h
			}
			if got != tt.html {
				t.Errorf("html = %q, want %q", got, tt.html)
			}
		})
	}
}

func TestRecord_Date(t *testing.T) {
	r := Record{Fields: map[string]any{"a": time.Date(2026, 10, 24, 0, 0, 0, 0, time.UTC), "b": "2026-10-25", "c": "Friday"}}
	for key, want := range map[string]string{"a": "2026-10-24", "b": "2026-10-25", "c": "Friday", "d": ""} {
		if got := r.Date(key); got != want {
			t.Errorf("Date(%q) = %q, want %q", key, got, want)
		}
	}
}

func TestOneLine(t *testing.T) {
	if got := OneLine("a  b\n c", 80); got != "a b c" {
		t.Errorf("got %q", got)
	}
	if got := OneLine(strings.Repeat("word ", 40), 30); len(got) > 34 || !strings.HasSuffix(got, "…") {
		t.Errorf("got %q", got)
	}
}
