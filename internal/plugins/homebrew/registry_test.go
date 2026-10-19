package homebrew

import (
	"context"
	"encoding/json"
	"reflect"
	"strings"
	"testing"
)

// okCheck is a CheckFunc that accepts anything unchanged.
func okCheck(_ context.Context, _ string, raw json.RawMessage) (json.RawMessage, error) {
	return raw, nil
}

func TestRegistryNameRules(t *testing.T) {
	tests := []struct {
		name    string
		in      string
		wantErr bool
	}{
		{"simple word", "widget", false},
		{"dotted", "tracker.reached", false},
		{"dashed", "my-part", false},
		{"digits after first letter", "a1.b2-c3", false},
		{"exactly 40 chars", "a" + strings.Repeat("b", 39), false},
		{"41 chars", "a" + strings.Repeat("b", 40), true},
		{"empty", "", true},
		{"upper case", "Widget", true},
		{"leading digit", "1abc", true},
		{"leading dot", ".abc", true},
		{"trailing dot", "abc.", true},
		{"double dot", "a..b", true},
		{"dot then dash", "a.-b", true},
		{"underscore", "a_b", true},
		{"space", "a b", true},
		{"slash", "a/b", true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			for what, reg := range map[string]func(r *Registry) error{
				"part":   func(r *Registry) error { return r.RegisterPart(PartKind{Name: tt.in, Check: okCheck}) },
				"event":  func(r *Registry) error { return r.RegisterEvent(EventDef{Name: tt.in, Check: okCheck}) },
				"action": func(r *Registry) error { return r.RegisterAction(ActionDef{Name: tt.in, Check: okCheck}) },
			} {
				err := reg(NewRegistry())
				if (err != nil) != tt.wantErr {
					t.Errorf("%s %q: err = %v, wantErr %v", what, tt.in, err, tt.wantErr)
				}
			}
		})
	}
}

func TestRegistryNilCheckRefused(t *testing.T) {
	r := NewRegistry()
	if err := r.RegisterPart(PartKind{Name: "widget"}); err == nil {
		t.Error("part with nil Check was accepted")
	}
	if err := r.RegisterEvent(EventDef{Name: "widget.hit"}); err == nil {
		t.Error("event with nil Check was accepted")
	}
	if err := r.RegisterAction(ActionDef{Name: "bump"}); err == nil {
		t.Error("action with nil Check was accepted")
	}
	// A refused registration must leave nothing behind.
	c := r.Catalog()
	if len(c.Parts)+len(c.Events)+len(c.Actions) != 0 {
		t.Errorf("refused registrations leaked into the catalog: %+v", c)
	}
}

func TestRegistryDuplicatesRefused(t *testing.T) {
	r := NewRegistry()
	if err := r.RegisterPart(PartKind{Name: "widget", Check: okCheck}); err != nil {
		t.Fatal(err)
	}
	if err := r.RegisterEvent(EventDef{Name: "widget", Check: okCheck}); err != nil {
		t.Fatalf("same name in another namespace must be allowed: %v", err)
	}
	if err := r.RegisterAction(ActionDef{Name: "widget", Check: okCheck}); err != nil {
		t.Fatalf("same name in another namespace must be allowed: %v", err)
	}
	tests := []struct {
		name string
		reg  func() error
	}{
		{"part", func() error { return r.RegisterPart(PartKind{Name: "widget", Label: "Other", Check: okCheck}) }},
		{"event", func() error { return r.RegisterEvent(EventDef{Name: "widget", Label: "Other", Check: okCheck}) }},
		{"action", func() error { return r.RegisterAction(ActionDef{Name: "widget", Label: "Other", Check: okCheck}) }},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if err := tt.reg(); err == nil {
				t.Error("duplicate registration was accepted")
			}
		})
	}
	// The first owner keeps the name.
	if k, _ := r.part("widget"); k.Label == "Other" {
		t.Error("duplicate replaced the original part kind")
	}
}

func TestRegistryForceSee(t *testing.T) {
	tests := []struct {
		name    string
		see     string
		wantErr bool
	}{
		{"none", "", false},
		{"party", SeeParty, false},
		{"name", SeeName, false},
		{"director", SeeDirector, false},
		{"unknown", "everyone", true},
		{"wrong case", "Party", true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := NewRegistry().RegisterPart(PartKind{Name: "widget", ForceSee: tt.see, Check: okCheck})
			if (err != nil) != tt.wantErr {
				t.Errorf("err = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

func TestRegistryCatalog(t *testing.T) {
	r := NewRegistry()
	// Registered out of order on purpose: the catalog sorts by name.
	for _, n := range []string{"zeta", "alpha", "mid.part"} {
		if err := r.RegisterPart(PartKind{Name: n, Label: n + "!", ForceSee: SeeDirector, Check: okCheck}); err != nil {
			t.Fatal(err)
		}
	}
	for _, n := range []string{"b.ev", "a.ev"} {
		if err := r.RegisterEvent(EventDef{Name: n, Label: n, PartKinds: []string{"alpha"}, Check: okCheck}); err != nil {
			t.Fatal(err)
		}
	}
	for _, a := range []ActionDef{
		{Name: "run", Label: "Run", ClientRun: true, Check: okCheck},
		{Name: "act", Label: "Act", PartKinds: []string{"zeta"}, Check: okCheck},
	} {
		if err := r.RegisterAction(a); err != nil {
			t.Fatal(err)
		}
	}
	c := r.Catalog()

	names := func(es []CatalogEntry) []string {
		out := []string{}
		for _, e := range es {
			out = append(out, e.Name)
		}
		return out
	}
	if got, want := names(c.Parts), []string{"alpha", "mid.part", "zeta"}; !reflect.DeepEqual(got, want) {
		t.Errorf("parts = %v, want %v", got, want)
	}
	if got, want := names(c.Events), []string{"a.ev", "b.ev"}; !reflect.DeepEqual(got, want) {
		t.Errorf("events = %v, want %v", got, want)
	}
	if got, want := names(c.Actions), []string{"act", "run"}; !reflect.DeepEqual(got, want) {
		t.Errorf("actions = %v, want %v", got, want)
	}
	if c.Parts[0].ForceSee != SeeDirector || c.Parts[0].Label != "alpha!" {
		t.Errorf("part entry lost fields: %+v", c.Parts[0])
	}
	if !reflect.DeepEqual(c.Events[0].PartKinds, []string{"alpha"}) {
		t.Errorf("event entry lost PartKinds: %+v", c.Events[0])
	}
	if c.Actions[0].ClientRun || !c.Actions[1].ClientRun || !reflect.DeepEqual(c.Actions[0].PartKinds, []string{"zeta"}) {
		t.Errorf("action entries wrong: %+v", c.Actions)
	}
}

func TestRegistryEmptyCatalogIsNotNil(t *testing.T) {
	// The Workshop reads these as JSON arrays; null would break its lists.
	c := NewRegistry().Catalog()
	b, err := json.Marshal(c)
	if err != nil {
		t.Fatal(err)
	}
	if string(b) != `{"parts":[],"events":[],"actions":[]}` {
		t.Errorf("empty catalog JSON = %s", b)
	}
}

func TestRegistryFields(t *testing.T) {
	tests := []struct {
		name    string
		fields  []Field
		wantErr bool
	}{
		{"none", nil, false},
		{"whole and text", []Field{{Name: "by", Label: "By", Type: FieldWhole}, {Name: "note", Label: "Note", Type: FieldText}}, false},
		{"camelCase name", []Field{{Name: "dayCount", Label: "Days", Type: FieldWhole}}, false},
		{"repeated name", []Field{{Name: "by", Label: "By", Type: FieldWhole}, {Name: "by", Label: "Again", Type: FieldWhole}}, true},
		{"bad name", []Field{{Name: "By", Label: "By", Type: FieldWhole}}, true},
		{"no label", []Field{{Name: "by", Label: " ", Type: FieldWhole}}, true},
		{"unknown type", []Field{{Name: "by", Label: "By", Type: "dice"}}, true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			reg := NewRegistry()
			errE := reg.RegisterEvent(EventDef{Name: "e", Fields: tt.fields, Check: okCheck})
			errA := reg.RegisterAction(ActionDef{Name: "a", Fields: tt.fields, Check: okCheck})
			if (errE != nil) != tt.wantErr || (errA != nil) != tt.wantErr {
				t.Errorf("event err %v, action err %v, want error %v", errE, errA, tt.wantErr)
			}
			if !tt.wantErr {
				c := reg.Catalog()
				if len(c.Events[0].Fields) != len(tt.fields) || len(c.Actions[0].Fields) != len(tt.fields) {
					t.Errorf("catalog lost the fields: %+v", c)
				}
			}
		})
	}
}

func TestRegistryPreview(t *testing.T) {
	by := []Field{{Name: "by", Label: "By", Type: FieldWhole}, {Name: "note", Label: "Note", Type: FieldText}}
	parts := []string{"tracker"}
	tests := []struct {
		name    string
		reg     func(r *Registry) error
		wantErr bool
	}{
		{"part shape", func(r *Registry) error { return r.RegisterPart(PartKind{Name: "p", Preview: PreviewShape, Check: okCheck}) }, false},
		{"part unknown preview", func(r *Registry) error { return r.RegisterPart(PartKind{Name: "p", Preview: "dial", Check: okCheck}) }, true},
		{"event reach", func(r *Registry) error {
			return r.RegisterEvent(EventDef{Name: "e", PartKinds: parts, Fields: by, Preview: &Preview{Op: PreviewReach, Arg: "by"}, Check: okCheck})
		}, false},
		{"event may not add", func(r *Registry) error {
			return r.RegisterEvent(EventDef{Name: "e", PartKinds: parts, Fields: by, Preview: &Preview{Op: PreviewAdd, Arg: "by"}, Check: okCheck})
		}, true},
		{"reach needs an arg", func(r *Registry) error {
			return r.RegisterEvent(EventDef{Name: "e", PartKinds: parts, Preview: &Preview{Op: PreviewReach}, Check: okCheck})
		}, true},
		{"event with no part", func(r *Registry) error {
			return r.RegisterEvent(EventDef{Name: "e", Fields: by, Preview: &Preview{Op: PreviewReach, Arg: "by"}, Check: okCheck})
		}, true},
		{"add with no arg is 1", func(r *Registry) error {
			return r.RegisterAction(ActionDef{Name: "a", PartKinds: parts, Preview: &Preview{Op: PreviewAdd}, Check: okCheck})
		}, false},
		{"set reads a whole field", func(r *Registry) error {
			return r.RegisterAction(ActionDef{Name: "a", PartKinds: parts, Fields: by, Preview: &Preview{Op: PreviewSet, Arg: "by"}, Check: okCheck})
		}, false},
		{"set needs an arg", func(r *Registry) error {
			return r.RegisterAction(ActionDef{Name: "a", PartKinds: parts, Preview: &Preview{Op: PreviewSet}, Check: okCheck})
		}, true},
		{"arg must be declared", func(r *Registry) error {
			return r.RegisterAction(ActionDef{Name: "a", PartKinds: parts, Fields: by, Preview: &Preview{Op: PreviewAdd, Arg: "to"}, Check: okCheck})
		}, true},
		{"arg must be whole", func(r *Registry) error {
			return r.RegisterAction(ActionDef{Name: "a", PartKinds: parts, Fields: by, Preview: &Preview{Op: PreviewAdd, Arg: "note"}, Check: okCheck})
		}, true},
		{"action may not reach", func(r *Registry) error {
			return r.RegisterAction(ActionDef{Name: "a", PartKinds: parts, Fields: by, Preview: &Preview{Op: PreviewReach, Arg: "by"}, Check: okCheck})
		}, true},
		{"client-run may not preview", func(r *Registry) error {
			return r.RegisterAction(ActionDef{Name: "a", PartKinds: parts, ClientRun: true, Preview: &Preview{Op: PreviewAdd}, Check: okCheck})
		}, true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if err := tt.reg(NewRegistry()); (err != nil) != tt.wantErr {
				t.Errorf("err = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}

	t.Run("catalog carries it", func(t *testing.T) {
		r := NewRegistry()
		if err := r.RegisterPart(PartKind{Name: "p", Preview: PreviewShape, Check: okCheck}); err != nil {
			t.Fatal(err)
		}
		if err := r.RegisterAction(ActionDef{Name: "a", PartKinds: parts, Fields: by, Preview: &Preview{Op: PreviewAdd, Arg: "by"}, Check: okCheck}); err != nil {
			t.Fatal(err)
		}
		if err := r.RegisterAction(ActionDef{Name: "b", PartKinds: parts, Check: okCheck}); err != nil {
			t.Fatal(err)
		}
		b, _ := json.Marshal(r.Catalog())
		for _, want := range []string{`"name":"p","label":"","preview":"shape"`, `"preview":{"op":"add","arg":"by"}`, `{"name":"b","label":"","partKinds":["tracker"]}`} {
			if !strings.Contains(string(b), want) {
				t.Errorf("catalog %s lacks %s", b, want)
			}
		}
	})
}
