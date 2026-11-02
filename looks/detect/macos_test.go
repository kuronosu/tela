package detect

import (
	"testing"

	"github.com/kuronosu/tela/looks/aqua"
)

func TestMacOSDesign(t *testing.T) {
	for _, c := range []struct {
		version string
		want    aqua.Design
	}{
		{"26.0", aqua.Tahoe},
		{"26.1.2", aqua.Tahoe},
		{"27.0", aqua.Tahoe},
		{"15.7.1", aqua.Sequoia},
		{"15", aqua.Sequoia},
		{"14.5", aqua.Sequoia},
		{"13.0", aqua.Sequoia},
		{"11.7", aqua.Sequoia},
		{"10.16", aqua.Sequoia},
		{" 15.0\n", aqua.Sequoia},
		{"", aqua.Tahoe},
		{"unknown", aqua.Tahoe},
	} {
		if got := macOSDesign(c.version); got != c.want {
			t.Errorf("macOSDesign(%q) = %v, want %v", c.version, got, c.want)
		}
	}
}
