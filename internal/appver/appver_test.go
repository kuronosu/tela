package appver

import "testing"

func TestVersionStamped(t *testing.T) {
	old := version
	t.Cleanup(func() { version = old })
	for in, want := range map[string]string{"v1.2.0": "1.2.0", "1.2.0": "1.2.0", "v": "v", "vnext": "vnext"} {
		version = in
		if got := Version(); got != want {
			t.Errorf("version %q: Version() = %q, want %q", in, got, want)
		}
	}
}
