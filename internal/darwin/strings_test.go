package darwin

import "testing"

func TestCString(t *testing.T) {
	for _, c := range []struct{ in, want string }{
		{"", ""},
		{"Tela ✓", "Tela ✓"},
		{"a\x00b", "a�b"},
		{"bad \xff byte", "bad � byte"},
	} {
		if got := CString(c.in); got != c.want {
			t.Errorf("CString(%q) = %q, want %q", c.in, got, c.want)
		}
	}
}
