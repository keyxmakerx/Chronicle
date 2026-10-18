package trackers

import (
	"reflect"
	"strings"
	"testing"
)

func TestCheckShape(t *testing.T) {
	tests := []struct {
		name    string
		in      Shape
		want    Shape
		wantErr bool
	}{
		{"count unbounded", Shape{Shape: ShapeCount}, Shape{Shape: ShapeCount}, false},
		{"count bounded", Shape{Shape: ShapeCount, Min: ip(0), Max: ip(10)}, Shape{Shape: ShapeCount, Min: ip(0), Max: ip(10)}, false},
		{"count min equals max", Shape{Shape: ShapeCount, Min: ip(3), Max: ip(3)}, Shape{}, true},
		{"count out of range", Shape{Shape: ShapeCount, Max: ip(MaxAbsValue + 1)}, Shape{}, true},
		{"count bands normalised", Shape{Shape: ShapeCount, Bands: []Band{{At: 5, Text: "  hot "}}}, Shape{Shape: ShapeCount, Bands: []Band{{At: 5, Text: "hot"}}}, false},
		{"count band without text", Shape{Shape: ShapeCount, Bands: []Band{{At: 5}}}, Shape{}, true},
		{"count duplicate bands", Shape{Shape: ShapeCount, Bands: []Band{{At: 5, Text: "a"}, {At: 5, Text: "b"}}}, Shape{}, true},
		{"count too many bands", Shape{Shape: ShapeCount, Bands: manyBands(MaxBands + 1)}, Shape{}, true},
		{"empty shape refused", Shape{}, Shape{}, true},
		{"unknown shape refused", Shape{Shape: "dial"}, Shape{}, true},
		{"clock", Shape{Shape: ShapeClock, Max: ip(6)}, Shape{Shape: ShapeClock, Max: ip(6)}, false},
		{"clock minimum", Shape{Shape: ShapeClock, Max: ip(MinSegments)}, Shape{Shape: ShapeClock, Max: ip(MinSegments)}, false},
		{"clock maximum", Shape{Shape: ShapeClock, Max: ip(MaxSegments)}, Shape{Shape: ShapeClock, Max: ip(MaxSegments)}, false},
		{"clock too few segments", Shape{Shape: ShapeClock, Max: ip(MinSegments - 1)}, Shape{}, true},
		{"clock too many segments", Shape{Shape: ShapeClock, Max: ip(MaxSegments + 1)}, Shape{}, true},
		{"clock without segments", Shape{Shape: ShapeClock}, Shape{}, true},
		{"clock drops min", Shape{Shape: ShapeClock, Max: ip(4), Min: ip(2)}, Shape{Shape: ShapeClock, Max: ip(4)}, false},
		{"steps", Shape{Shape: ShapeSteps, Steps: []Step{{Name: " Calm "}, {Name: "Angry", Text: " rawr "}}},
			Shape{Shape: ShapeSteps, Steps: []Step{{Name: "Calm"}, {Name: "Angry", Text: "rawr"}}}, false},
		{"steps drop bounds and bands", Shape{Shape: ShapeSteps, Steps: steps(3), Min: ip(0), Max: ip(9), Bands: []Band{{At: 1, Text: "x"}}},
			Shape{Shape: ShapeSteps, Steps: steps(3)}, false},
		{"steps minimum", Shape{Shape: ShapeSteps, Steps: steps(MinSteps)}, Shape{Shape: ShapeSteps, Steps: steps(MinSteps)}, false},
		{"steps maximum", Shape{Shape: ShapeSteps, Steps: steps(MaxSteps)}, Shape{Shape: ShapeSteps, Steps: steps(MaxSteps)}, false},
		{"too few steps", Shape{Shape: ShapeSteps, Steps: steps(MinSteps - 1)}, Shape{}, true},
		{"too many steps", Shape{Shape: ShapeSteps, Steps: steps(MaxSteps + 1)}, Shape{}, true},
		{"step without a name", Shape{Shape: ShapeSteps, Steps: []Step{{Name: "a"}, {Name: " "}}}, Shape{}, true},
		{"step name too long", Shape{Shape: ShapeSteps, Steps: []Step{{Name: "a"}, {Name: strings.Repeat("x", MaxNameChars+1)}}}, Shape{}, true},
		{"step text too long", Shape{Shape: ShapeSteps, Steps: []Step{{Name: "a"}, {Name: "b", Text: strings.Repeat("x", MaxTextChars+1)}}}, Shape{}, true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := CheckShape(tt.in)
			if tt.wantErr {
				if !isKind(err, 422) {
					t.Fatalf("err = %v, want a 422", err)
				}
				return
			}
			if err != nil {
				t.Fatal(err)
			}
			if !reflect.DeepEqual(got, tt.want) {
				t.Errorf("got %+v, want %+v", got, tt.want)
			}
		})
	}
}

// A shape CheckShape accepts must be one validateKind accepts with the same
// fields, so a recipe can never hold a tracker the block would refuse.
func TestCheckShapeAgreesWithValidateKind(t *testing.T) {
	shapes := []Shape{
		{Shape: ShapeCount, Min: ip(1), Max: ip(2)},
		{Shape: ShapeClock, Max: ip(8)},
		{Shape: ShapeSteps, Steps: steps(4)},
		{Shape: ShapeClock, Max: ip(1)},
		{Shape: "x"},
	}
	for _, s := range shapes {
		_, shapeErr := CheckShape(s)
		_, kindErr := validateKind(KindInput{Name: "shape", Shape: s.Shape, Min: s.Min, Max: s.Max, Steps: s.Steps, Bands: s.Bands})
		if (shapeErr == nil) != (kindErr == nil) {
			t.Errorf("%+v: CheckShape err %v, validateKind err %v", s, shapeErr, kindErr)
		}
	}
}

func manyBands(n int) []Band {
	out := make([]Band, n)
	for i := range out {
		out[i] = Band{At: i, Text: "b"}
	}
	return out
}
