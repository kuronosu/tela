package main

import (
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"slices"
	"strings"
	"testing"
)

// TestReleaseBuildHasNoDevLoop checks the owner's condition for tela dev:
// release builds are unchanged (ADR-111, ADR-114). An app built without
// the teladev tag has no dev-loop package among its dependencies (nor
// encoding/gob, which only the tagged state code uses) and no dev-loop
// symbol in its binary; with the tag they are all there, so the checks
// prove something.
func TestReleaseBuildHasNoDevLoop(t *testing.T) {
	skipShort(t)
	goBin, err := exec.LookPath("go")
	if err != nil {
		t.Skip("the go command is not in PATH")
	}
	root := repoRoot(t)
	const app = "./examples/counter" // imports neither encoding/gob nor a dev-loop package
	exe := ""
	if runtime.GOOS == "windows" {
		exe = ".exe"
	}
	run := func(args ...string) string {
		t.Helper()
		cmd := exec.Command(goBin, args...)
		cmd.Dir = root
		cmd.Env = append(os.Environ(), "CGO_ENABLED=0")
		out, err := cmd.Output()
		if err != nil {
			var stderr []byte
			if ee, ok := err.(*exec.ExitError); ok {
				stderr = ee.Stderr
			}
			t.Fatalf("go %s: %v\n%s", strings.Join(args, " "), err, stderr)
		}
		return string(out)
	}

	packages := []string{
		"github.com/kuronosu/tela/internal/devloop",
		"github.com/kuronosu/tela/internal/platform/remote",
		"encoding/gob",
	}
	symbols := []string{
		"github.com/kuronosu/tela/internal/devloop.",
		"github.com/kuronosu/tela/internal/platform/remote.",
		"encoding/gob.",
		"core.(*Tree).CaptureDevState",
		"core.RegisterDevState",
		"core.SetDevSeed",
		"core.devRun",
		"core.devRecover",
		"core.devBackend",
		"core.devAfterFrame",
	}

	check := func(tags []string, want bool) {
		t.Helper()
		name := "without the teladev tag"
		if want {
			name = "with the teladev tag"
		}
		listArgs := append(append([]string{"list", "-deps"}, tags...), app)
		deps := strings.Fields(run(listArgs...))
		for _, p := range packages {
			if got := slices.Contains(deps, p); got != want {
				t.Errorf("%s: %s in the dependencies of %s is %v, want %v", name, p, app, got, want)
			}
		}
		bin := filepath.Join(t.TempDir(), "app"+exe)
		buildArgs := append(append([]string{"build"}, tags...), "-o", bin, app)
		run(buildArgs...)
		nm := run("tool", "nm", bin)
		for _, sym := range symbols {
			if got := strings.Contains(nm, sym); got != want {
				t.Errorf("%s: a symbol containing %q in the binary is %v, want %v", name, sym, got, want)
			}
		}
	}
	check(nil, false)
	check([]string{"-tags", "teladev"}, true)
}
