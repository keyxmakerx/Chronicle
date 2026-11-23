package cutscenes

import (
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
)

// Every kind the server accepts must be one the engine draws, or a scene the
// editor saved would play as a blank layer. The engine names each kind as a
// quoted string, so the test reads its source for each.
func TestEngineDrawsEveryAcceptedKind(t *testing.T) {
	path := filepath.Join("..", "..", "..", "static", "js", "cutscene_engine.js")
	src, err := os.ReadFile(path)
	if os.IsNotExist(err) {
		t.Skipf("%s does not exist yet", path)
	}
	if err != nil {
		t.Fatal(err)
	}
	body := string(src)
	for _, list := range [][]Kind{BackgroundKinds, OverlayKinds, TextStyles, EndingKinds} {
		for _, k := range list {
			if !strings.Contains(body, "'"+k.Key+"'") && !strings.Contains(body, `"`+k.Key+`"`) {
				t.Errorf("the engine does not name %q", k.Key)
			}
		}
	}
}

// The looks the server keeps and names must be the engine's own, in its order
// (the first is the engine's default), or the editor would offer a look the
// engine draws as something else. The test reads the LOOKS table out of the
// engine's source: each look is written as { id: '…', name: '…', … }.
func TestEngineLooksMatchTheServer(t *testing.T) {
	path := filepath.Join("..", "..", "..", "static", "js", "cutscene_engine.js")
	src, err := os.ReadFile(path)
	if os.IsNotExist(err) {
		t.Skipf("%s does not exist yet", path)
	}
	if err != nil {
		t.Fatal(err)
	}
	body := string(src)
	start := strings.Index(body, "var LOOKS = {")
	if start < 0 {
		t.Fatal("the engine has no LOOKS table")
	}
	end := strings.Index(body[start:], "\n  };")
	if end < 0 {
		t.Fatal("the engine's LOOKS table does not end where expected")
	}
	table := body[start : start+end]
	kindRe := regexp.MustCompile(`'([a-z-]+)'\s*:\s*\[|\b([a-z]+)\s*:\s*\[`)
	lookRe := regexp.MustCompile(`\{\s*id:\s*'([^']+)',\s*name:\s*'([^']+)'`)
	engine := map[string][]Look{}
	kinds := kindRe.FindAllStringSubmatchIndex(table, -1)
	for i, m := range kinds {
		var kind string
		if m[2] >= 0 {
			kind = table[m[2]:m[3]]
		} else {
			kind = table[m[4]:m[5]]
		}
		stop := len(table)
		if i+1 < len(kinds) {
			stop = kinds[i+1][0]
		}
		for _, l := range lookRe.FindAllStringSubmatch(table[m[1]:stop], -1) {
			engine[kind] = append(engine[kind], Look{Key: l[1], Name: l[2]})
		}
	}
	if len(engine) != len(Looks) {
		t.Errorf("the engine has looks for %v, the server for %d kinds", engine, len(Looks))
	}
	for kind, want := range Looks {
		got := engine[kind]
		if len(got) != len(want) {
			t.Errorf("%s: the engine has %v, the server %v", kind, got, want)
			continue
		}
		for i := range want {
			if got[i].Key != want[i].Key || got[i].Name != want[i].Name {
				t.Errorf("%s look %d: the engine has %s %q, the server %s %q", kind, i, got[i].Key, got[i].Name, want[i].Key, want[i].Name)
			}
		}
	}
	for kind, want := range Looks {
		if !strings.Contains(body, kind+"': '"+want[0].Key+"'") && !strings.Contains(body, kind+": '"+want[0].Key+"'") {
			t.Errorf("the engine's default look for %s is not %q, the server's first", kind, want[0].Key)
		}
	}
}
