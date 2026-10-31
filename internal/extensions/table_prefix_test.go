package extensions

import (
	"context"
	"testing"
)

// listOnlyRepo answers List with a fixed set of installed extensions; any
// other repository call panics through the nil embedded interface.
type listOnlyRepo struct {
	ExtensionRepository
	installed []Extension
}

func (r *listOnlyRepo) List(context.Context) ([]Extension, error) { return r.installed, nil }

// TestRequireOwnTablePrefix pins that no two installed extensions can share
// table names: "foo" owns ext_foo_*, which includes every ext_foo_bar_*
// table "foo-bar" would create, so either one is refused while the other is
// installed. Names that only look alike ("foo" and "foobar") are fine.
func TestRequireOwnTablePrefix(t *testing.T) {
	tests := []struct {
		installed, candidate string
		refused              bool
	}{
		{"foo", "foo-bar", true},
		{"foo-bar", "foo", true},
		{"foo", "foobar", false},
		{"dice-roller", "harptos-calendar", false},
	}
	for _, tt := range tests {
		t.Run(tt.installed+"+"+tt.candidate, func(t *testing.T) {
			svc := &extensionService{repo: &listOnlyRepo{installed: []Extension{{ExtID: tt.installed}}}}
			err := svc.requireOwnTablePrefix(context.Background(), tt.candidate)
			if (err != nil) != tt.refused {
				t.Fatalf("err = %v, want refused %v", err, tt.refused)
			}
		})
	}
}

// TestOwnedByOther pins the uninstall safety net: dropping "foo" leaves the
// tables of an installed "foo-bar" alone.
func TestOwnedByOther(t *testing.T) {
	prefix := ExtensionTablePrefix("foo")
	if !ownedByOther("ext_foo_bar_items", prefix, []string{"foo_bar"}) {
		t.Fatal("foo-bar's table would be dropped with foo")
	}
	if ownedByOther("ext_foo_items", prefix, []string{"foo_bar"}) {
		t.Fatal("foo's own table was kept")
	}
}
