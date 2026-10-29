package utf16x

import (
	"testing"
	"unicode/utf16"
)

func TestOffsets(t *testing.T) {
	s := "aé😀b" // 1 + 2 + 4 + 1 bytes; 1 + 1 + 2 + 1 units
	if got, want := Len(s), len(utf16.Encode([]rune(s))); got != want {
		t.Errorf("Len = %d, want %d", got, want)
	}
	for _, tc := range []struct{ u, b int }{{-1, 0}, {0, 0}, {1, 1}, {2, 3}, {3, 3}, {4, 7}, {5, 8}, {9, 8}} {
		if got := ToByte(s, tc.u); got != tc.b {
			t.Errorf("ToByte(%d) = %d, want %d", tc.u, got, tc.b)
		}
	}
	for _, tc := range []struct{ b, u int }{{-3, 0}, {0, 0}, {1, 1}, {2, 1}, {3, 2}, {5, 2}, {7, 4}, {8, 5}, {20, 5}} {
		if got := FromByte(s, tc.b); got != tc.u {
			t.Errorf("FromByte(%d) = %d, want %d", tc.b, got, tc.u)
		}
	}
	// A surrogate pair first, and a three-byte character last.
	s = "a😀é日"
	if n := Len(s); n != 5 {
		t.Errorf("Len = %d, want 5", n)
	}
	for _, tc := range []struct{ u, b int }{{0, 0}, {1, 1}, {2, 1}, {3, 5}, {4, 7}, {5, 10}, {9, 10}} {
		if got := ToByte(s, tc.u); got != tc.b {
			t.Errorf("ToByte(%q, %d) = %d, want %d", s, tc.u, got, tc.b)
		}
	}
	for _, tc := range []struct{ b, u int }{{0, 0}, {1, 1}, {3, 1}, {5, 3}, {7, 4}, {10, 5}} {
		if got := FromByte(s, tc.b); got != tc.u {
			t.Errorf("FromByte(%q, %d) = %d, want %d", s, tc.b, got, tc.u)
		}
	}
}
