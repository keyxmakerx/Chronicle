package packages

import (
	"database/sql"
	"strings"
	"testing"
)

// scanProbe stands in for *sql.Row: it records how many destinations
// scanPackage asks for and fills the retention one.
type scanProbe struct {
	n         int
	retention *sql.NullInt64
	set       bool
	setValue  int64
}

func (p *scanProbe) Scan(dest ...any) error {
	p.n = len(dest)
	for _, d := range dest {
		if nv, ok := d.(*sql.NullInt64); ok {
			p.retention = nv
			nv.Valid, nv.Int64 = p.set, p.setValue
		}
	}
	return nil
}

// columnNames splits packageColumns on top-level commas (the commas inside
// COALESCE(...) are not separators).
func columnNames() []string {
	var names []string
	depth, cur := 0, ""
	for _, r := range packageColumns {
		switch r {
		case '(':
			depth++
		case ')':
			depth--
		}
		if r == ',' && depth == 0 {
			names = append(names, strings.TrimSpace(cur))
			cur = ""
			continue
		}
		cur += string(r)
	}
	return append(names, strings.TrimSpace(cur))
}

// TestScanPackageReadsRetentionColumn pins that the SELECT list and the Scan
// destinations stay in step and that retention_keep_newest is actually read.
func TestScanPackageReadsRetentionColumn(t *testing.T) {
	names := columnNames()
	tests := []struct {
		name string
		set  bool
		val  int64
		want *int
	}{
		{"NULL means use the site rule", false, 0, nil},
		{"value is read", true, 3, intp(3)},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			p := &scanProbe{set: tc.set, setValue: tc.val}
			pkg, err := scanPackage(p)
			if err != nil {
				t.Fatal(err)
			}
			if p.n != len(names) {
				t.Fatalf("scanPackage asks for %d columns, packageColumns lists %d", p.n, len(names))
			}
			if p.retention == nil {
				t.Fatal("retention_keep_newest is not scanned")
			}
			if names[len(names)-3] != "retention_keep_newest" {
				t.Errorf("retention_keep_newest must sit just before created_at, got %q", names[len(names)-3])
			}
			switch {
			case tc.want == nil && pkg.RetentionKeepNewest != nil:
				t.Errorf("NULL must give nil, got %d", *pkg.RetentionKeepNewest)
			case tc.want != nil && (pkg.RetentionKeepNewest == nil || *pkg.RetentionKeepNewest != *tc.want):
				t.Errorf("got %v, want %d", pkg.RetentionKeepNewest, *tc.want)
			}
		})
	}
}
