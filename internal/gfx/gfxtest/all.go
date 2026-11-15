package gfxtest

// Scenes returns every shared scene: Scenes1, Scenes2, Scenes3 and Scenes4,
// in that order. The result is a new slice.
func Scenes() []Scene {
	var out []Scene
	for _, g := range [][]Scene{Scenes1, Scenes2, Scenes3, Scenes4} {
		out = append(out, g...)
	}
	return out
}
