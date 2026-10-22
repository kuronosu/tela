//go:build !teladev

package core

import (
	"os/exec"
	"slices"
	"strings"
	"testing"
)

// TestDevStateOff checks that a build without the teladev tag has no
// dev-loop state (ADR-111, ADR-113): the hook points are off, and neither
// this package nor its dependencies import encoding/gob, which only the
// tagged files use.
func TestDevStateOff(t *testing.T) {
	if devStateEnabled {
		t.Fatal("devStateEnabled is true without the teladev tag")
	}
	gobin, err := exec.LookPath("go")
	if err != nil {
		t.Skipf("no go command: %v", err)
	}
	deps := func(tags ...string) []string {
		t.Helper()
		args := append([]string{"list", "-deps"}, tags...)
		out, err := exec.Command(gobin, append(args, ".")...).Output()
		if err != nil {
			t.Skipf("go list: %v", err)
		}
		return strings.Fields(string(out))
	}
	if d := deps(); slices.Contains(d, "encoding/gob") {
		t.Error("internal/core depends on encoding/gob without the teladev tag")
	}
	// The check sees the tag's dependencies.
	if d := deps("-tags", "teladev"); !slices.Contains(d, "encoding/gob") {
		t.Error("internal/core does not depend on encoding/gob with the teladev tag: the check above proves nothing")
	}
}
