package homebrew

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

// codeOf returns the HTTP code of an apperror, or 0 when err is not one.
func codeOf(err error) int {
	var ae *apperror.AppError
	if errors.As(err, &ae) {
		return ae.Code
	}
	return 0
}

func msgOf(err error) string {
	var ae *apperror.AppError
	if errors.As(err, &ae) {
		return ae.Message
	}
	return ""
}

var errDB = errors.New("db is down: secret detail")

// testRegistry registers a small world: parts "widget" (plain), "secret"
// (forced to director), "picky" (rejects), "rewriter" (rewrites its data) and
// "broken" (owner 500); events tied to widget and free; actions tied to
// widget and free.
func testRegistry(t *testing.T) *Registry {
	t.Helper()
	reg := NewRegistry()
	reject := func(_ context.Context, _ string, _ json.RawMessage) (json.RawMessage, error) {
		return nil, apperror.NewValidation("bad thing")
	}
	rewrite := func(_ context.Context, _ string, _ json.RawMessage) (json.RawMessage, error) {
		return json.RawMessage(`{"rewritten":true}`), nil
	}
	broken := func(_ context.Context, _ string, _ json.RawMessage) (json.RawMessage, error) {
		return nil, apperror.NewInternal(errDB)
	}
	empty := func(_ context.Context, _ string, _ json.RawMessage) (json.RawMessage, error) {
		return nil, nil
	}
	must := func(err error) {
		t.Helper()
		if err != nil {
			t.Fatal(err)
		}
	}
	must(reg.RegisterPart(PartKind{Name: "widget", Label: "Widget", Check: okCheck}))
	must(reg.RegisterPart(PartKind{Name: "other", Label: "Other", Check: okCheck}))
	must(reg.RegisterPart(PartKind{Name: "secret", Label: "Secret", ForceSee: SeeDirector, Check: okCheck}))
	must(reg.RegisterPart(PartKind{Name: "party-only", Label: "Party", ForceSee: SeeParty, Check: okCheck}))
	must(reg.RegisterPart(PartKind{Name: "picky", Label: "Picky", Check: reject}))
	must(reg.RegisterPart(PartKind{Name: "rewriter", Label: "Rewriter", Check: rewrite}))
	must(reg.RegisterPart(PartKind{Name: "broken", Label: "Broken", Check: broken}))
	must(reg.RegisterPart(PartKind{Name: "silent", Label: "Silent", Check: empty}))
	must(reg.RegisterEvent(EventDef{Name: "widget.hit", PartKinds: []string{"widget"}, Check: okCheck}))
	must(reg.RegisterEvent(EventDef{Name: "moon", Check: okCheck}))
	must(reg.RegisterEvent(EventDef{Name: "picky.event", Check: reject}))
	must(reg.RegisterAction(ActionDef{Name: "bump", PartKinds: []string{"widget"}, Check: okCheck}))
	must(reg.RegisterAction(ActionDef{Name: "ping", Check: okCheck}))
	must(reg.RegisterAction(ActionDef{Name: "picky.action", Check: reject}))
	must(reg.RegisterAction(ActionDef{Name: "rewrite", Check: rewrite}))
	return reg
}

func w(key string) Part { return Part{Key: key, Kind: "widget", Label: "W " + key, See: SeeParty} }

func raw(s string) json.RawMessage { return json.RawMessage(s) }

func rule(ev, part, timing string, then ...Do) Rule {
	return Rule{When: When{Event: ev, Part: part}, Timing: timing, Then: then}
}

func TestValidateDocLimits(t *testing.T) {
	reg := testRegistry(t)
	manyParts := make([]Part, MaxParts+1)
	for i := range manyParts {
		manyParts[i] = w(fmt.Sprintf("p%d", i))
	}
	okParts := manyParts[:MaxParts]
	manyRules := make([]Rule, MaxRules+1)
	for i := range manyRules {
		manyRules[i] = rule("moon", "", "", Do{Action: "ping"})
	}
	thens := make([]Do, MaxThen+1)
	for i := range thens {
		thens[i] = Do{Action: "ping"}
	}
	tests := []struct {
		name string
		in   Doc
		want int // 0 = ok
	}{
		{"max parts ok", Doc{Parts: okParts}, 0},
		{"too many parts", Doc{Parts: manyParts}, 422},
		{"max rules ok", Doc{Rules: manyRules[:MaxRules]}, 0},
		{"too many rules", Doc{Rules: manyRules}, 422},
		{"max then ok", Doc{Rules: []Rule{rule("moon", "", "", thens[:MaxThen]...)}}, 0},
		{"too many then", Doc{Rules: []Rule{rule("moon", "", "", thens...)}}, 422},
		{"no then", Doc{Rules: []Rule{rule("moon", "", "")}}, 422},
		{"data at limit", Doc{Parts: []Part{{Key: "a", Kind: "widget", Label: "A", Data: raw(`"` + strings.Repeat("x", MaxDataBytes-2) + `"`)}}}, 0},
		{"data over limit", Doc{Parts: []Part{{Key: "a", Kind: "widget", Label: "A", Data: raw(`"` + strings.Repeat("x", MaxDataBytes) + `"`)}}}, 422},
		{"args at limit", Doc{Rules: []Rule{{When: When{Event: "moon", Args: raw(`"` + strings.Repeat("x", MaxArgsBytes-2) + `"`)}, Then: []Do{{Action: "ping"}}}}}, 0},
		{"when args over limit", Doc{Rules: []Rule{{When: When{Event: "moon", Args: raw(`"` + strings.Repeat("x", MaxArgsBytes) + `"`)}, Then: []Do{{Action: "ping"}}}}}, 422},
		{"do args over limit", Doc{Rules: []Rule{rule("moon", "", "", Do{Action: "ping", Args: raw(`"` + strings.Repeat("x", MaxArgsBytes) + `"`)})}}, 422},
		{"data not json", Doc{Parts: []Part{{Key: "a", Kind: "widget", Label: "A", Data: raw(`{nope`)}}}, 422},
		{"args not json", Doc{Rules: []Rule{{When: When{Event: "moon", Args: raw(`{nope`)}, Then: []Do{{Action: "ping"}}}}}, 422},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := validateDoc(context.Background(), reg, "c1", tt.in)
			if got := codeOf(err); got != tt.want {
				t.Errorf("code = %d (err %v), want %d", got, err, tt.want)
			}
		})
	}
}

func TestValidateDocKeys(t *testing.T) {
	reg := testRegistry(t)
	tests := []struct {
		name  string
		parts []Part
		want  int
	}{
		{"simple", []Part{w("a")}, 0},
		{"digits dash underscore", []Part{w("a1_b-c")}, 0},
		{"leading digit ok", []Part{w("1a")}, 0},
		{"40 chars ok", []Part{w(strings.Repeat("a", 40))}, 0},
		{"41 chars", []Part{w(strings.Repeat("a", 41))}, 422},
		{"empty", []Part{w("")}, 422},
		{"upper case", []Part{w("Abc")}, 422},
		{"leading dash", []Part{w("-a")}, 422},
		{"leading underscore", []Part{w("_a")}, 422},
		{"space", []Part{w("a b")}, 422},
		{"dot", []Part{w("a.b")}, 422},
		{"duplicate", []Part{w("a"), w("b"), w("a")}, 422},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := validateDoc(context.Background(), reg, "c1", Doc{Parts: tt.parts})
			if got := codeOf(err); got != tt.want {
				t.Errorf("code = %d (err %v), want %d", got, err, tt.want)
			}
		})
	}
}

func TestValidateDocUnknowns(t *testing.T) {
	reg := testRegistry(t)
	tests := []struct {
		name string
		in   Doc
	}{
		{"unknown kind", Doc{Parts: []Part{{Key: "a", Kind: "nope", Label: "A"}}}},
		{"empty kind", Doc{Parts: []Part{{Key: "a", Label: "A"}}}},
		{"unknown event", Doc{Rules: []Rule{rule("nope", "", "", Do{Action: "ping"})}}},
		{"empty event", Doc{Rules: []Rule{rule("", "", "", Do{Action: "ping"})}}},
		{"unknown action", Doc{Rules: []Rule{rule("moon", "", "", Do{Action: "nope"})}}},
		{"empty action", Doc{Rules: []Rule{rule("moon", "", "", Do{})}}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := validateDoc(context.Background(), reg, "c1", tt.in)
			if codeOf(err) != http.StatusUnprocessableEntity {
				t.Errorf("err = %v, want 422", err)
			}
		})
	}
}

func TestValidateDocSee(t *testing.T) {
	reg := testRegistry(t)
	tests := []struct {
		name    string
		kind    string
		see     string
		want    string
		wantErr bool
	}{
		{"default is director", "widget", "", SeeDirector, false},
		{"party kept", "widget", SeeParty, SeeParty, false},
		{"name kept", "widget", SeeName, SeeName, false},
		{"director kept", "widget", SeeDirector, SeeDirector, false},
		{"invalid", "widget", "everyone", "", true},
		{"invalid case", "widget", "Party", "", true},
		{"forced director overrides party", "secret", SeeParty, SeeDirector, false},
		{"forced director fills default", "secret", "", SeeDirector, false},
		{"forced party overrides director", "party-only", SeeDirector, SeeParty, false},
		{"invalid still refused when forced", "secret", "bogus", "", true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			out, err := validateDoc(context.Background(), reg, "c1", Doc{Parts: []Part{{Key: "a", Kind: tt.kind, Label: "A", See: tt.see}}})
			if tt.wantErr {
				if codeOf(err) != 422 {
					t.Fatalf("err = %v, want 422", err)
				}
				return
			}
			if err != nil {
				t.Fatal(err)
			}
			if out.Parts[0].See != tt.want {
				t.Errorf("see = %q, want %q", out.Parts[0].See, tt.want)
			}
		})
	}
}

func TestValidateDocLabel(t *testing.T) {
	reg := testRegistry(t)
	tests := []struct {
		name  string
		label string
		want  string
		bad   bool
	}{
		{"trimmed", "  Heat  ", "Heat", false},
		{"80 chars", strings.Repeat("é", 80), strings.Repeat("é", 80), false},
		{"81 chars", strings.Repeat("é", 81), "", true},
		{"empty", "", "", true},
		{"whitespace only", "   ", "", true},
		{"newline", "a\nb", "", true},
		{"tab", "a\tb", "", true},
		{"invalid utf8", "a\xffb", "", true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			out, err := validateDoc(context.Background(), reg, "c1", Doc{Parts: []Part{{Key: "a", Kind: "widget", Label: tt.label}}})
			if tt.bad {
				if codeOf(err) != 422 {
					t.Fatalf("err = %v, want 422", err)
				}
				return
			}
			if err != nil {
				t.Fatal(err)
			}
			if out.Parts[0].Label != tt.want {
				t.Errorf("label = %q, want %q", out.Parts[0].Label, tt.want)
			}
		})
	}
}

func TestValidateDocDataNormalising(t *testing.T) {
	reg := testRegistry(t)
	tests := []struct {
		name string
		kind string
		data json.RawMessage
		want string
	}{
		{"absent becomes object", "widget", nil, `{}`},
		{"null becomes object", "widget", raw(`null`), `{}`},
		{"whitespace becomes object", "widget", raw("  \n "), `{}`},
		{"padded data trimmed", "widget", raw(` {"a":1} `), `{"a":1}`},
		{"data kept", "widget", raw(`{"a":1}`), `{"a":1}`},
		{"owner rewrite wins", "rewriter", raw(`{"a":1}`), `{"rewritten":true}`},
		{"empty owner output falls back to input", "silent", raw(`{"a":1}`), `{"a":1}`},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			out, err := validateDoc(context.Background(), reg, "c1", Doc{Parts: []Part{{Key: "a", Kind: tt.kind, Label: "A", Data: tt.data}}})
			if err != nil {
				t.Fatal(err)
			}
			if string(out.Parts[0].Data) != tt.want {
				t.Errorf("data = %s, want %s", out.Parts[0].Data, tt.want)
			}
		})
	}
}

func TestValidateDocOwnerErrors(t *testing.T) {
	reg := testRegistry(t)
	t.Run("part validation is prefixed with the label and is 422", func(t *testing.T) {
		_, err := validateDoc(context.Background(), reg, "c1", Doc{Parts: []Part{{Key: "a", Kind: "picky", Label: "My Thing"}}})
		if codeOf(err) != 422 {
			t.Fatalf("code = %d, want 422", codeOf(err))
		}
		if got := msgOf(err); got != "My Thing: bad thing" {
			t.Errorf("message = %q", got)
		}
	})
	t.Run("owner 500 passes through unchanged", func(t *testing.T) {
		_, err := validateDoc(context.Background(), reg, "c1", Doc{Parts: []Part{{Key: "a", Kind: "broken", Label: "B"}}})
		if codeOf(err) != 500 {
			t.Fatalf("code = %d, want 500", codeOf(err))
		}
		if !errors.Is(err, errDB) {
			t.Error("the owner's internal error was replaced")
		}
		if strings.Contains(msgOf(err), "secret") {
			t.Errorf("client message leaks detail: %q", msgOf(err))
		}
	})
	t.Run("event check error names the rule", func(t *testing.T) {
		_, err := validateDoc(context.Background(), reg, "c1", Doc{Rules: []Rule{rule("picky.event", "", "", Do{Action: "ping"})}})
		if got := msgOf(err); codeOf(err) != 422 || got != "rule 1's when: bad thing" {
			t.Errorf("err = %v", err)
		}
	})
	t.Run("action check error names the step", func(t *testing.T) {
		ok := rule("moon", "", "", Do{Action: "ping"})
		bad := rule("moon", "", "", Do{Action: "ping"}, Do{Action: "picky.action"})
		_, err := validateDoc(context.Background(), reg, "c1", Doc{Rules: []Rule{ok, bad}})
		if got := msgOf(err); codeOf(err) != 422 || got != "rule 2, step 2: bad thing" {
			t.Errorf("err = %v", err)
		}
	})
}

func TestValidateDocRuleParts(t *testing.T) {
	reg := testRegistry(t)
	parts := []Part{w("a"), {Key: "o", Kind: "other", Label: "O"}}
	tests := []struct {
		name string
		rule Rule
		want int
	}{
		{"event with right part", rule("widget.hit", "a", "", Do{Action: "ping"}), 0},
		{"event missing part", rule("widget.hit", "", "", Do{Action: "ping"}), 422},
		{"event unknown part key", rule("widget.hit", "zzz", "", Do{Action: "ping"}), 422},
		{"event wrong kind", rule("widget.hit", "o", "", Do{Action: "ping"}), 422},
		{"free event given a part", rule("moon", "a", "", Do{Action: "ping"}), 422},
		{"free event no part", rule("moon", "", "", Do{Action: "ping"}), 0},
		{"action with right part", rule("moon", "", "", Do{Action: "bump", Part: "a"}), 0},
		{"action missing part", rule("moon", "", "", Do{Action: "bump"}), 422},
		{"action unknown part key", rule("moon", "", "", Do{Action: "bump", Part: "zzz"}), 422},
		{"action wrong kind", rule("moon", "", "", Do{Action: "bump", Part: "o"}), 422},
		{"free action given a part", rule("moon", "", "", Do{Action: "ping", Part: "a"}), 422},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := validateDoc(context.Background(), reg, "c1", Doc{Parts: parts, Rules: []Rule{tt.rule}})
			if got := codeOf(err); got != tt.want {
				t.Errorf("code = %d (err %v), want %d", got, err, tt.want)
			}
		})
	}
	t.Run("a rule cannot reference a part declared in another doc", func(t *testing.T) {
		_, err := validateDoc(context.Background(), reg, "c1", Doc{Rules: []Rule{rule("widget.hit", "a", "", Do{Action: "ping"})}})
		if codeOf(err) != 422 {
			t.Errorf("err = %v, want 422", err)
		}
	})
}

func TestValidateDocTiming(t *testing.T) {
	reg := testRegistry(t)
	tests := []struct {
		name   string
		timing string
		want   string
		bad    bool
	}{
		{"default is now", "", TimingNow, false},
		{"now", TimingNow, TimingNow, false},
		{"encounter end", TimingEncounterEnd, TimingEncounterEnd, false},
		{"held", TimingHeld, TimingHeld, false},
		{"invalid", "later", "", true},
		{"wrong case", "Now", "", true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			out, err := validateDoc(context.Background(), reg, "c1", Doc{Rules: []Rule{rule("moon", "", tt.timing, Do{Action: "ping"})}})
			if tt.bad {
				if codeOf(err) != 422 {
					t.Fatalf("err = %v, want 422", err)
				}
				return
			}
			if err != nil {
				t.Fatal(err)
			}
			if out.Rules[0].Timing != tt.want {
				t.Errorf("timing = %q, want %q", out.Rules[0].Timing, tt.want)
			}
		})
	}
}

func TestValidateDocTestText(t *testing.T) {
	reg := testRegistry(t)
	tests := []struct {
		name string
		text string
		want string
		bad  bool
	}{
		{"empty ok", "", "", false},
		{"trimmed", "  hp < 5 ", "hp < 5", false},
		{"200 chars", strings.Repeat("x", MaxTestChars), strings.Repeat("x", MaxTestChars), false},
		{"201 chars", strings.Repeat("x", MaxTestChars+1), "", true},
		{"newline", "a\nb", "", true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			r := Rule{When: When{Event: "moon", Test: tt.text}, Then: []Do{{Action: "ping"}}}
			out, err := validateDoc(context.Background(), reg, "c1", Doc{Rules: []Rule{r}})
			if tt.bad {
				if codeOf(err) != 422 {
					t.Fatalf("err = %v, want 422", err)
				}
				return
			}
			if err != nil {
				t.Fatal(err)
			}
			if out.Rules[0].When.Test != tt.want {
				t.Errorf("test = %q, want %q", out.Rules[0].When.Test, tt.want)
			}
		})
	}
}

func TestValidateDocOutputShape(t *testing.T) {
	reg := testRegistry(t)
	t.Run("empty doc has non-nil slices", func(t *testing.T) {
		out, err := validateDoc(context.Background(), reg, "c1", Doc{})
		if err != nil {
			t.Fatal(err)
		}
		b, _ := json.Marshal(out)
		if string(b) != `{"parts":[],"rules":[]}` {
			t.Errorf("json = %s", b)
		}
	})
	t.Run("args default to an empty object on events and actions", func(t *testing.T) {
		out, err := validateDoc(context.Background(), reg, "c1", Doc{Rules: []Rule{rule("moon", "", "", Do{Action: "ping"}, Do{Action: "rewrite"})}})
		if err != nil {
			t.Fatal(err)
		}
		r := out.Rules[0]
		if string(r.When.Args) != `{}` || string(r.Then[0].Args) != `{}` || string(r.Then[1].Args) != `{"rewritten":true}` {
			t.Errorf("args = %s / %s / %s", r.When.Args, r.Then[0].Args, r.Then[1].Args)
		}
	})
}
