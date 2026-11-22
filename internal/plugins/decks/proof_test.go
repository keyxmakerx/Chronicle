package decks

// Sample data for the card tests: a look with every palette colour as a kind
// and a back, and a face wearing one of them.

// proofKinds paints the ten palette colours across the eight signs, so every
// colour and every sign is drawn.
func proofKinds() []KindDef {
	out := make([]KindDef, 0, len(Palette))
	for i, p := range Palette {
		out = append(out, KindDef{Key: "k-" + p, Name: "Kind " + p, Colour: p, Sign: signValues[i%len(signValues)]})
	}
	return out
}

// proofLook is the default look with the sample kinds and a back, changed by f.
func proofLook(f func(*Look)) Look {
	l := defaultLook()
	l.Kinds = proofKinds()
	l.Back = Back{Pattern: "lattice", Colour: "indigo", Words: "Sample deck", Sign: "star"}
	if f != nil {
		f(&l)
	}
	return l
}

func proofFace(kind, corner string, rare bool) Face {
	return Face{
		CardID: "sample", Name: "Sample card", KindKey: kind, Corner: corner, Rare: rare,
		Words: "Words go here. A few short lines show how they sit on the card.",
	}
}

func proofUp(kind, corner string, rare bool) CardState {
	return FaceUp(proofFace(kind, corner, rare), "")
}
