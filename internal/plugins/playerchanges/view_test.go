package playerchanges

import (
	"testing"
	"time"
)

func TestMark(t *testing.T) {
	tests := []struct {
		name  string
		s     string
		words []string
		want  string
	}{
		{"no words escapes", "<b>Elf</b>", nil, "&lt;b&gt;Elf&lt;/b&gt;"},
		{"marks a word ignoring case", "High Elf", []string{"elf"}, "High <mark>Elf</mark>"},
		{"overlapping words make one mark", "Stamina", []string{"stam", "amina"}, "<mark>Stamina</mark>"},
		{"escapes inside a mark", "a<b", []string{"a<b"}, "<mark>a&lt;b</mark>"},
		{"no match", "Gnome", []string{"elf"}, "Gnome"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := mark(tt.s, tt.words); got != tt.want {
				t.Errorf("mark(%q) = %q, want %q", tt.s, got, tt.want)
			}
		})
	}
}

func TestDayLine(t *testing.T) {
	now := time.Date(2026, 10, 20, 22, 0, 0, 0, time.UTC)
	tests := []struct {
		name string
		at   time.Time
		want string
	}{
		{"today", time.Date(2026, 10, 20, 21, 42, 0, 0, time.UTC), "Today, 9:42 pm"},
		{"yesterday", time.Date(2026, 10, 19, 22, 2, 0, 0, time.UTC), "Yesterday, 10:02 pm"},
		{"this year", time.Date(2026, 8, 2, 21, 10, 0, 0, time.UTC), "2 August, 9:10 pm"},
		{"another year", time.Date(2025, 8, 2, 9, 10, 0, 0, time.UTC), "2 August 2025, 9:10 am"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := dayLine(tt.at, time.UTC, now); got != tt.want {
				t.Errorf("dayLine = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestByLine(t *testing.T) {
	tests := []struct {
		c    Change
		want string
	}{
		{Change{UserName: "Bea"}, "Bea"},
		{Change{UserName: "Bea", Source: SourceAI, App: "Claude"}, "Bea's AI (Claude)"},
		{Change{UserName: "Bea", Source: SourceAI}, "Bea's AI"},
		{Change{}, "A player who has left"},
	}
	for _, tt := range tests {
		if got := byLine(tt.c); got != tt.want {
			t.Errorf("byLine(%+v) = %q, want %q", tt.c, got, tt.want)
		}
	}
}
