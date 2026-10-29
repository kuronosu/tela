package editdist

import "testing"

func TestDistance(t *testing.T) {
	for _, tc := range []struct {
		a, b        string
		bytes, runs int
	}{
		{"", "", 0, 0},
		{"abc", "", 3, 3},
		{"kitten", "sitting", 3, 3},
		{"ab", "ba", 2, 2}, // a transposition costs two (Levenshtein, not OSA)
		{"é", "e", 2, 1},   // two bytes against one, one rune against one
	} {
		if got := Distance(tc.a, tc.b); got != tc.bytes {
			t.Errorf("Distance(%q, %q) = %d, want %d", tc.a, tc.b, got, tc.bytes)
		}
		if got := RuneDistance(tc.a, tc.b); got != tc.runs {
			t.Errorf("RuneDistance(%q, %q) = %d, want %d", tc.a, tc.b, got, tc.runs)
		}
	}
}
