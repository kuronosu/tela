package win32

import (
	"slices"
	"testing"
)

func TestStripToastFlags(t *testing.T) {
	for _, c := range []struct {
		in, want  []string
		activated bool
	}{
		{[]string{"app.exe"}, []string{"app.exe"}, false},
		{[]string{"app.exe", "-v", "notes.md"}, []string{"app.exe", "-v", "notes.md"}, false},
		// A COM server of the app's own keeps -Embedding.
		{[]string{"app.exe", "-Embedding"}, []string{"app.exe", "-Embedding"}, false},
		{[]string{"app.exe", "-ToastActivated"}, []string{"app.exe"}, true},
		{[]string{"app.exe", "-ToastActivated", "-Embedding"}, []string{"app.exe"}, true},
		{[]string{"app.exe", "-ToastActivated", "/embedding"}, []string{"app.exe"}, true},
		{[]string{"app.exe", "-v", "-ToastActivated", "-Embedding", "x"}, []string{"app.exe", "-v", "x"}, true},
		// Only the exact flag: another spelling is the app's own argument.
		{[]string{"app.exe", "-toastactivated"}, []string{"app.exe", "-toastactivated"}, false},
	} {
		got, activated := stripToastFlags(c.in)
		if !slices.Equal(got, c.want) || activated != c.activated {
			t.Errorf("stripToastFlags(%q) = %q, %v; want %q, %v", c.in, got, activated, c.want, c.activated)
		}
	}
}
