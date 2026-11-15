package gfxtest

// Scenes returns every shared scene: Scenes1, Scenes2, Scenes3, Scenes4 and
// TextScenes, in that order. The result is a new slice.
func Scenes() []Scene {
	var out []Scene
	for _, g := range [][]Scene{Scenes1, Scenes2, Scenes3, Scenes4, TextScenes} {
		out = append(out, g...)
	}
	return out
}
