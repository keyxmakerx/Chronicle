package database

import "testing"

// The reconcile rewrite is exactly the write paths' rule, and leaves text
// with no GM-only parts byte for byte.
func TestPublicTextStripped(t *testing.T) {
	tests := []struct {
		name    string
		col     PublicTextColumn
		in      string
		want    string
		changed bool
	}{
		{"html secret", PublicTextColumn{}, `<p>Feast. <span data-secret="true">Poisoned.</span></p>`, `<p>Feast. </p>`, true},
		{"html plain", PublicTextColumn{}, `<p>Feast, <em>mentions</em> data-secret in prose</p>`, `<p>Feast, <em>mentions</em> data-secret in prose</p>`, false},
		{"json secret", PublicTextColumn{JSON: true}, `{"type":"doc","content":[{"type":"text","text":"Poisoned.","marks":[{"type":"secret"}]}]}`, `{"content":null,"type":"doc"}`, true},
		{"json with the word secret", PublicTextColumn{JSON: true}, `{"type":"doc","content":[{"type":"text","text":"a \"secret\" door"}]}`, `{"type":"doc","content":[{"type":"text","text":"a \"secret\" door"}]}`, false},
		{"plain text", PublicTextColumn{JSON: true}, `the "secret" door`, `the "secret" door`, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, changed := publicTextStripped(tt.col, tt.in)
			if got != tt.want || changed != tt.changed {
				t.Errorf("got %q, %v; want %q, %v", got, changed, tt.want, tt.changed)
			}
		})
	}
}
