package gfxtest

import "testing"

// TestScenes checks that every scene has a unique name and a size, and that
// it records at least one command.
func TestScenes(t *testing.T) {
	seen := map[string]bool{}
	scenes := Scenes()
	if want := len(Scenes1) + len(Scenes2) + len(Scenes3) + len(Scenes4) + len(TextScenes); len(scenes) != want {
		t.Fatalf("Scenes() has %d scenes, want %d", len(scenes), want)
	}
	for _, s := range append(scenes, Scenes5...) {
		if seen[s.Name] {
			t.Errorf("duplicate scene name %q", s.Name)
		}
		seen[s.Name] = true
		if s.W <= 0 || s.H <= 0 {
			t.Errorf("scene %q has size %dx%d", s.Name, s.W, s.H)
		}
		if Build(s.Draw).Len() == 0 {
			t.Errorf("scene %q is empty", s.Name)
		}
	}
}
