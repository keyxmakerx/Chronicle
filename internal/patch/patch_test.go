// patch_test.go — the three directions of the sweep-R4 partial-write
// contract, on the primitive every fixed endpoint is built from:
// absent preserves · present replaces · explicit null clears.
package patch

import (
	"encoding/json"
	"testing"
)

type probe struct {
	Summary patch3[string] `json:"summary"`
	Status  patch3[string] `json:"status"`
	Count   patch3[int]    `json:"count"`
	Flag    patch3[bool]   `json:"flag"`
}

// patch3 is an alias so the test body reads as the contract, not as Go.
type patch3[T any] = Field[T]

func TestField_ThreeStatesFromJSON(t *testing.T) {
	cases := []struct {
		name        string
		body        string
		wantPresent bool
		wantNull    bool
		wantValue   string
	}{
		{"absent key preserves", `{}`, false, false, ""},
		{"explicit null clears", `{"summary":null}`, true, true, ""},
		{"present value replaces", `{"summary":"recap"}`, true, false, "recap"},
		{"present empty string is a value, not a clear", `{"summary":""}`, true, false, ""},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			var p probe
			if err := json.Unmarshal([]byte(tc.body), &p); err != nil {
				t.Fatalf("unmarshal: %v", err)
			}
			if got := p.Summary.Present(); got != tc.wantPresent {
				t.Errorf("Present() = %v, want %v", got, tc.wantPresent)
			}
			if got := p.Summary.IsNull(); got != tc.wantNull {
				t.Errorf("IsNull() = %v, want %v", got, tc.wantNull)
			}
			if v, ok := p.Summary.Get(); ok && v != tc.wantValue {
				t.Errorf("Get() = %q, want %q", v, tc.wantValue)
			}
		})
	}
}

func TestField_PtrMergesNullableColumn(t *testing.T) {
	stored := "the stored summary"
	cases := []struct {
		name  string
		body  string
		want  *string
		isNil bool
	}{
		{"absent preserves", `{}`, &stored, false},
		{"explicit null clears", `{"summary":null}`, nil, true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			var p probe
			if err := json.Unmarshal([]byte(tc.body), &p); err != nil {
				t.Fatalf("unmarshal: %v", err)
			}
			got := p.Summary.Ptr(&stored)
			if tc.isNil {
				if got != nil {
					t.Fatalf("Ptr() = %q, want nil (explicit null must clear)", *got)
				}
				return
			}
			if got == nil || *got != stored {
				t.Fatalf("Ptr() = %v, want %q preserved", got, stored)
			}
		})
	}

	// present replaces, and the result does not alias the input pointer.
	var p probe
	if err := json.Unmarshal([]byte(`{"summary":"new"}`), &p); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	got := p.Summary.Ptr(&stored)
	if got == nil || *got != "new" {
		t.Fatalf("Ptr() = %v, want \"new\"", got)
	}
	*got = "mutated"
	if stored != "the stored summary" {
		t.Errorf("Ptr() aliased the stored value; stored = %q", stored)
	}
}

func TestField_ValMergesNonNullableColumn(t *testing.T) {
	cases := []struct {
		name string
		body string
		cur  string
		want string
	}{
		{"absent preserves", `{}`, "planned", "planned"},
		{"present replaces", `{"status":"completed"}`, "planned", "completed"},
		// A NOT NULL column has no "cleared" state; writing the zero value
		// silently IS the data loss this package exists to stop.
		{"explicit null preserves on a non-nullable field", `{"status":null}`, "planned", "planned"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			var p probe
			if err := json.Unmarshal([]byte(tc.body), &p); err != nil {
				t.Fatalf("unmarshal: %v", err)
			}
			if got := p.Status.Val(tc.cur); got != tc.want {
				t.Errorf("Val(%q) = %q, want %q", tc.cur, got, tc.want)
			}
		})
	}
}

// The zero Field must be the ABSENT state: a caller that constructs an input
// struct without mentioning a field must not overwrite it.
func TestField_ZeroValueIsAbsent(t *testing.T) {
	var f Field[bool]
	if f.Present() {
		t.Fatal("zero Field reports present; every struct-literal caller would clobber")
	}
	if got := f.Val(true); got != true {
		t.Errorf("Val(true) = %v on a zero Field, want true preserved", got)
	}
	if got := f.Ptr(nil); got != nil {
		t.Errorf("Ptr(nil) = %v, want nil", got)
	}
}

func TestField_FalseAndZeroAreValues(t *testing.T) {
	var p probe
	if err := json.Unmarshal([]byte(`{"flag":false,"count":0}`), &p); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if !p.Flag.Present() || p.Flag.Val(true) != false {
		t.Error("explicit false must replace, not read as absent")
	}
	if !p.Count.Present() || p.Count.Val(9) != 0 {
		t.Error("explicit 0 must replace, not read as absent")
	}
}

func TestField_Constructors(t *testing.T) {
	if !Of("x").Present() || Of("x").IsNull() {
		t.Error("Of must be present and non-null")
	}
	if !Null[string]().IsNull() {
		t.Error("Null must be present and null")
	}
	if Absent[string]().Present() {
		t.Error("Absent must not be present")
	}
}

func TestField_BadJSONErrors(t *testing.T) {
	var p probe
	if err := json.Unmarshal([]byte(`{"count":"not a number"}`), &p); err == nil {
		t.Fatal("want a decode error for a type mismatch inside Field")
	}
}
