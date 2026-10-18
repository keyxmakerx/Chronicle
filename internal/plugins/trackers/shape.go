package trackers

// Shape is a kind's rules without a name, a home or a default audience: what
// a Homebrew Workshop recipe part carries for a tracker or a clock.
type Shape struct {
	Shape string `json:"shape"`
	Min   *int   `json:"min,omitempty"`
	Max   *int   `json:"max,omitempty"`
	Steps []Step `json:"steps,omitempty"`
	Bands []Band `json:"bands,omitempty"`
}

// CheckShape validates a shape by the same rules as a saved kind and returns
// it normalised, so a recipe can never hold a tracker the block would refuse.
func CheckShape(s Shape) (Shape, error) {
	k, err := validateKind(KindInput{Name: "shape", Shape: s.Shape, Min: s.Min, Max: s.Max, Steps: s.Steps, Bands: s.Bands})
	if err != nil {
		return Shape{}, err
	}
	out := Shape{Shape: k.Shape, Min: k.Min, Max: k.Max}
	if len(k.Steps) > 0 {
		out.Steps = k.Steps
	}
	if len(k.Bands) > 0 {
		out.Bands = k.Bands
	}
	return out, nil
}
