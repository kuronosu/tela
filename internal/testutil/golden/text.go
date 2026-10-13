package golden

import (
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// Text compares got against the golden text file testdata/<name> (name
// includes its extension, such as "counter.aria.yml") and reports a test
// failure if they differ. With TELA_UPDATE=1 the golden is written
// instead. On a mismatch, got is written to <base>_actual<ext> next to the
// golden (the .yml form is gitignored); it is removed again when the test
// passes.
func Text(t testing.TB, name, got string, opts ...Option) {
	t.Helper()
	cfg := config{dir: "testdata"}
	for _, o := range opts {
		o(&cfg)
	}
	path := filepath.Join(cfg.dir, filepath.FromSlash(name))
	ext := filepath.Ext(path)
	actual := strings.TrimSuffix(path, ext) + "_actual" + ext
	if os.Getenv(UpdateEnv) == "1" {
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, []byte(got), 0o644); err != nil {
			t.Fatalf("golden: cannot write %s: %v", path, err)
		}
		removeStale(actual)
		t.Logf("golden: wrote %s; review it before committing", path)
		return
	}
	want, err := os.ReadFile(path)
	if err != nil {
		_ = os.WriteFile(actual, []byte(got), 0o644)
		if errors.Is(err, fs.ErrNotExist) {
			t.Fatalf("golden: %s does not exist; create it with %s=1 go test -run '%s' and review it (the output is in %s)",
				path, UpdateEnv, t.Name(), actual)
		}
		t.Fatalf("golden: cannot read %s: %v", path, err)
	}
	if string(want) != got {
		_ = os.WriteFile(actual, []byte(got), 0o644)
		t.Errorf("golden: output differs from %s (actual in %s); if the change is intended, regenerate with %s=1 and review it.\ngot:\n%s\nwant:\n%s",
			path, actual, UpdateEnv, got, want)
		return
	}
	removeStale(actual)
}
