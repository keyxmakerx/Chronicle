// Package patch provides three-state JSON fields for partial-update
// requests.
//
// The contract it encodes — ruled by the coordinator on 2026-08-07 for
// sweep R4, extending the C-SIDEBAR-REORDER-RESCUE PR1 step 1 booking and
// the C-CAL-NULL-PRESERVE precedent to the whole partial-write class:
//
//	an ABSENT key preserves the stored value;
//	an EXPLICIT null clears it;
//	a present value replaces it.
//
// A plain pointer field cannot express that, because encoding/json collapses
// "key missing" and "key: null" into the same nil. Field[T] keeps them apart
// by recording presence in UnmarshalJSON, which the decoder only calls for
// keys that are actually in the body.
//
// Usage on a request struct (handler layer) and on the service input it
// feeds (services never import Echo, and Field is Echo-free):
//
//	type req struct {
//	    Summary patch.Field[string] `json:"summary"`
//	}
//	stored.Summary = input.Summary.Ptr(stored.Summary) // nullable column
//	stored.Status  = input.Status.Val(stored.Status)   // NOT NULL column
package patch

import (
	"bytes"
	"encoding/json"
)

// Field is a three-state JSON value: absent, explicit null, or a value.
//
// The zero Field is the ABSENT state, which is what a JSON body that omits
// the key produces — so a struct bound from a partial body preserves every
// field the caller did not mention without any per-field bookkeeping.
type Field[T any] struct {
	present bool
	value   *T
}

// Of returns a present Field carrying v ("replace with v").
func Of[T any](v T) Field[T] { return Field[T]{present: true, value: &v} }

// Null returns a present Field carrying an explicit null ("clear").
func Null[T any]() Field[T] { return Field[T]{present: true} }

// FromPtr converts a nullable Go value into a PRESENT Field: a non-nil p
// becomes "replace with *p", a nil p becomes an explicit null ("clear").
// It is for callers that hold a complete record and mean to write all of
// it — a full restore from an export, say. It is NOT for translating a
// partially-bound request struct; there, nil means absent, and using
// FromPtr would resurrect exactly the clobber this package prevents.
func FromPtr[T any](p *T) Field[T] {
	if p == nil {
		return Null[T]()
	}
	return Of(*p)
}

// Absent returns the zero Field ("preserve"). It exists so test tables and
// call sites can say which of the three states they mean out loud.
func Absent[T any]() Field[T] { return Field[T]{} }

// Present reports whether the key was in the JSON body at all.
func (f Field[T]) Present() bool { return f.present }

// IsNull reports whether the key was present AND explicitly null.
func (f Field[T]) IsNull() bool { return f.present && f.value == nil }

// Get returns the carried value and whether one is carried. It is false for
// both the absent and the explicit-null states; callers that must tell them
// apart use Present/IsNull.
func (f Field[T]) Get() (T, bool) {
	if f.value == nil {
		var zero T
		return zero, false
	}
	return *f.value, true
}

// Ptr merges this field over the stored value of a NULLABLE model field:
// absent preserves cur, explicit null returns nil (clear), a value returns a
// pointer to it (replace). The returned pointer never aliases cur's pointee.
func (f Field[T]) Ptr(cur *T) *T {
	if !f.present {
		return cur
	}
	if f.value == nil {
		return nil
	}
	v := *f.value
	return &v
}

// Val merges this field over the stored value of a NON-NULLABLE model field:
// absent preserves cur, a value replaces it. An explicit null on a column
// that cannot hold NULL also preserves — "clear" has no representation
// there, and silently writing the zero value is exactly the whole-replace
// data loss this package exists to stop. Callers wanting "reset to default"
// on such a field send the default explicitly.
func (f Field[T]) Val(cur T) T {
	if !f.present || f.value == nil {
		return cur
	}
	return *f.value
}

// UnmarshalJSON records presence. encoding/json only calls it for keys the
// body actually carries, which is the entire mechanism.
func (f *Field[T]) UnmarshalJSON(b []byte) error {
	f.present = true
	if bytes.Equal(bytes.TrimSpace(b), []byte("null")) {
		f.value = nil
		return nil
	}
	var v T
	if err := json.Unmarshal(b, &v); err != nil {
		return err
	}
	f.value = &v
	return nil
}

// MarshalJSON renders the carried value, or null when the field is absent or
// explicitly null. Round-tripping an absent field through Marshal loses the
// absent/null distinction — Field is a REQUEST type; do not use it on a
// response struct.
func (f Field[T]) MarshalJSON() ([]byte, error) {
	if f.value == nil {
		return []byte("null"), nil
	}
	return json.Marshal(*f.value)
}
