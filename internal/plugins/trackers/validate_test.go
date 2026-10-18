package trackers

import (
	"errors"
	"fmt"
	"strings"
	"testing"

	"github.com/keyxmakerx/chronicle/internal/apperror"
)

// isKind reports whether err is an apperror with the given HTTP code.
func isKind(err error, code int) bool {
	var ae *apperror.AppError
	return errors.As(err, &ae) && ae.Code == code
}

func steps(n int) []Step {
	out := make([]Step, n)
	for i := range out {
		out[i] = Step{Name: fmt.Sprintf("s%d", i+1)}
	}
	return out
}

func TestValidateKind(t *testing.T) {
	long80 := strings.Repeat("x", 80)
	tests := []struct {
		name    string
		in      KindInput
		wantErr bool
	}{
		{"count unbounded", KindInput{Name: "Gold", Shape: ShapeCount}, false},
		{"count bounded", KindInput{Name: "Heat", Shape: ShapeCount, Min: ip(0), Max: ip(10)}, false},
		{"count min equals max", KindInput{Name: "Heat", Shape: ShapeCount, Min: ip(3), Max: ip(3)}, true},
		{"count min above max", KindInput{Name: "Heat", Shape: ShapeCount, Min: ip(5), Max: ip(3)}, true},
		{"count min out of range", KindInput{Name: "Heat", Shape: ShapeCount, Min: ip(-MaxAbsValue - 1)}, true},
		{"count max out of range", KindInput{Name: "Heat", Shape: ShapeCount, Max: ip(MaxAbsValue + 1)}, true},
		{"steps good", KindInput{Name: "Mood", Shape: ShapeSteps, Steps: steps(3)}, false},
		{"steps minimum", KindInput{Name: "Mood", Shape: ShapeSteps, Steps: steps(MinSteps)}, false},
		{"steps maximum", KindInput{Name: "Mood", Shape: ShapeSteps, Steps: steps(MaxSteps)}, false},
		{"steps too few", KindInput{Name: "Mood", Shape: ShapeSteps, Steps: steps(1)}, true},
		{"steps none", KindInput{Name: "Mood", Shape: ShapeSteps}, true},
		{"steps too many", KindInput{Name: "Mood", Shape: ShapeSteps, Steps: steps(MaxSteps + 1)}, true},
		{"step name empty", KindInput{Name: "Mood", Shape: ShapeSteps, Steps: []Step{{Name: "a"}, {Name: " "}}}, true},
		{"clock good", KindInput{Name: "Doom", Shape: ShapeClock, Max: ip(6)}, false},
		{"clock lowest", KindInput{Name: "Doom", Shape: ShapeClock, Max: ip(2)}, false},
		{"clock highest", KindInput{Name: "Doom", Shape: ShapeClock, Max: ip(24)}, false},
		{"clock one segment", KindInput{Name: "Doom", Shape: ShapeClock, Max: ip(1)}, true},
		{"clock 25 segments", KindInput{Name: "Doom", Shape: ShapeClock, Max: ip(25)}, true},
		{"clock no max", KindInput{Name: "Doom", Shape: ShapeClock}, true},
		{"unknown shape", KindInput{Name: "X", Shape: "dial"}, true},
		{"empty shape", KindInput{Name: "X"}, true},
		{"name empty", KindInput{Name: "  ", Shape: ShapeCount}, true},
		{"name 80 ok", KindInput{Name: long80, Shape: ShapeCount}, false},
		{"name 81 too long", KindInput{Name: long80 + "x", Shape: ShapeCount}, true},
		{"name newline", KindInput{Name: "a\nb", Shape: ShapeCount}, true},
		{"name tab", KindInput{Name: "a\tb", Shape: ShapeCount}, true},
		{"name NUL", KindInput{Name: "a\x00b", Shape: ShapeCount}, true},
		{"name invalid utf8", KindInput{Name: "a\xffb", Shape: ShapeCount}, true},
		{"step name control char", KindInput{Name: "M", Shape: ShapeSteps, Steps: []Step{{Name: "a"}, {Name: "b\x07"}}}, true},
		{"step text too long", KindInput{Name: "M", Shape: ShapeSteps, Steps: []Step{{Name: "a", Text: strings.Repeat("x", MaxTextChars+1)}, {Name: "b"}}}, true},
		{"bad visibility", KindInput{Name: "X", Shape: ShapeCount, Visibility: "dm"}, true},
		{"everyone visibility", KindInput{Name: "X", Shape: ShapeCount, Visibility: SeeEveryone}, false},
		{"negative category", KindInput{Name: "X", Shape: ShapeCount, EveryTypeID: -1}, true},
		{"band text 80+ but under 300 is ok", KindInput{Name: "X", Shape: ShapeCount, Bands: []Band{{At: 0, Text: strings.Repeat("y", 250)}}}, false},
		{"band text exactly 300 is ok", KindInput{Name: "X", Shape: ShapeCount, Bands: []Band{{At: 0, Text: strings.Repeat("y", MaxTextChars)}}}, false},
		{"band text over 300", KindInput{Name: "X", Shape: ShapeCount, Bands: []Band{{At: 0, Text: strings.Repeat("y", MaxTextChars+1)}}}, true},
		{"band text empty", KindInput{Name: "X", Shape: ShapeCount, Bands: []Band{{At: 0, Text: "  "}}}, true},
		{"band duplicate at", KindInput{Name: "X", Shape: ShapeCount, Bands: []Band{{At: 2, Text: "a"}, {At: 2, Text: "b"}}}, true},
		{"band at out of range", KindInput{Name: "X", Shape: ShapeCount, Bands: []Band{{At: MaxAbsValue + 1, Text: "a"}}}, true},
		{"band control char", KindInput{Name: "X", Shape: ShapeCount, Bands: []Band{{At: 0, Text: "a\nb"}}}, true},
		{"too many bands", KindInput{Name: "X", Shape: ShapeCount, Bands: make([]Band, MaxBands+1)}, true},
		{"clock with bands", KindInput{Name: "X", Shape: ShapeClock, Max: ip(4), Bands: []Band{{At: 2, Text: "half"}}}, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := validateKind(tt.in)
			if (err != nil) != tt.wantErr {
				t.Fatalf("err = %v, wantErr %v", err, tt.wantErr)
			}
			if err != nil && !isKind(err, 422) {
				t.Fatalf("want a validation error, got %v", err)
			}
		})
	}
}

func TestValidateKindNormalises(t *testing.T) {
	t.Run("defaults visibility to directors and trims", func(t *testing.T) {
		k, err := validateKind(KindInput{Name: "  Gold  ", Shape: ShapeCount})
		if err != nil {
			t.Fatal(err)
		}
		if k.Name != "Gold" || k.Visibility != SeeDirectors {
			t.Fatalf("got %+v", k)
		}
	})
	t.Run("steps clears min max and bands", func(t *testing.T) {
		k, err := validateKind(KindInput{Name: "M", Shape: ShapeSteps, Steps: steps(2),
			Min: ip(1), Max: ip(9), Bands: []Band{{At: 1, Text: "x"}}})
		if err != nil {
			t.Fatal(err)
		}
		if k.Min != nil || k.Max != nil || len(k.Bands) != 0 || len(k.Steps) != 2 {
			t.Fatalf("stale fields kept: %+v", k)
		}
	})
	t.Run("clock clears min and steps, keeps max", func(t *testing.T) {
		k, err := validateKind(KindInput{Name: "D", Shape: ShapeClock, Max: ip(4), Min: ip(2), Steps: steps(3)})
		if err != nil {
			t.Fatal(err)
		}
		if k.Min != nil || k.Max == nil || *k.Max != 4 || len(k.Steps) != 0 {
			t.Fatalf("got %+v", k)
		}
	})
	t.Run("count clears steps", func(t *testing.T) {
		k, err := validateKind(KindInput{Name: "G", Shape: ShapeCount, Steps: steps(3)})
		if err != nil {
			t.Fatal(err)
		}
		if len(k.Steps) != 0 {
			t.Fatalf("steps kept: %+v", k.Steps)
		}
	})
	t.Run("slices are non-nil for JSON", func(t *testing.T) {
		k, _ := validateKind(KindInput{Name: "G", Shape: ShapeCount})
		if k.Steps == nil || k.Bands == nil {
			t.Fatal("steps and bands must be empty slices, not nil")
		}
	})
	t.Run("does not alias the input bounds", func(t *testing.T) {
		in := KindInput{Name: "G", Shape: ShapeCount, Max: ip(5)}
		k, _ := validateKind(in)
		*in.Max = 99
		if *k.Max != 5 {
			t.Fatal("kind shares the input's pointer")
		}
	})
}

func TestValidateLog(t *testing.T) {
	tests := []struct {
		name     string
		in       LogInput
		wantErr  bool
		wantNote string
	}{
		{"add one", LogInput{Op: OpAdd, Amount: 1}, false, ""},
		{"add negative", LogInput{Op: OpAdd, Amount: -4}, false, ""},
		{"add zero refused", LogInput{Op: OpAdd, Amount: 0}, true, ""},
		{"set zero allowed", LogInput{Op: OpSet, Amount: 0}, false, ""},
		{"bad op", LogInput{Op: "mul", Amount: 2}, true, ""},
		{"empty op", LogInput{Amount: 2}, true, ""},
		{"amount too big", LogInput{Op: OpAdd, Amount: MaxAbsValue + 1}, true, ""},
		{"amount too small", LogInput{Op: OpSet, Amount: -MaxAbsValue - 1}, true, ""},
		{"note trimmed", LogInput{Op: OpAdd, Amount: 1, Note: "  paid the guard "}, false, "paid the guard"},
		{"note at limit", LogInput{Op: OpAdd, Amount: 1, Note: strings.Repeat("n", MaxNoteChars)}, false, strings.Repeat("n", MaxNoteChars)},
		{"note too long", LogInput{Op: OpAdd, Amount: 1, Note: strings.Repeat("n", MaxNoteChars+1)}, true, ""},
		{"note control char", LogInput{Op: OpAdd, Amount: 1, Note: "a\nb"}, true, ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := validateLog(tt.in)
			if (err != nil) != tt.wantErr {
				t.Fatalf("err = %v, wantErr %v", err, tt.wantErr)
			}
			if err != nil {
				if !isKind(err, 422) {
					t.Fatalf("want validation error, got %v", err)
				}
				return
			}
			if got.Note != tt.wantNote {
				t.Fatalf("note = %q want %q", got.Note, tt.wantNote)
			}
		})
	}
}

func TestValidToken(t *testing.T) {
	tests := []struct {
		in   string
		want bool
	}{
		{"abc-123_X", true},
		{"", false},
		{"a b", false},
		{"a/b", false},
		{"a'b", false},
		{strings.Repeat("a", MaxIDChars), true},
		{strings.Repeat("a", MaxIDChars+1), false},
	}
	for _, tt := range tests {
		if got := validToken(tt.in); got != tt.want {
			t.Errorf("validToken(%q) = %v want %v", tt.in, got, tt.want)
		}
	}
}
