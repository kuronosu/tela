//go:build darwin

package assoc

import (
	"errors"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/kuronosu/tela/internal/darwin"
	"github.com/kuronosu/tela/internal/platform"
)

// TestDarwinSmoke logs the bindings' answers for what every Mac has, and
// how long Launch Services takes to list a bundle in the user cache (where
// `tela run` builds). An earlier, longer version showed that Launch
// Services never lists a bundle in a temporary folder (/var/folders),
// whatever its executable, while it lists one in the user cache or
// ~/Applications within milliseconds.
func TestDarwinSmoke(t *testing.T) {
	watchdog(t, time.Minute)
	if err := darwin.LoadLaunchServices(); err != nil {
		t.Fatalf("LoadLaunchServices: %v", err)
	}
	t.Logf("LS bundles of com.apple.TextEdit: %q", darwin.LSApplicationPaths("com.apple.TextEdit"))
	t.Logf("LS mailto: handlers %q, default %q", darwin.LSSchemeHandlers("mailto"), darwin.LSDefaultSchemeHandler("mailto"))

	app, _ := lsBundle(t)
	dir := filepath.Dir(filepath.Dir(filepath.Dir(app.Exec)))
	st := darwin.LSRegisterURL(dir, true)
	t.Logf("LSRegisterURL = %d", st)
	if !poll(t, "a bundle in the user cache is listed", 15*time.Second, func() bool {
		return len(darwin.LSApplicationPaths(app.ID)) > 0
	}) {
		lsDump(t, app.ID)
		t.Errorf("Launch Services did not list %s", dir)
	}
}

// TestDarwinTempDir: a bundle in t.TempDir() is refused by the calls that
// register, and reported not registered.
func TestDarwinTempDir(t *testing.T) {
	watchdog(t, time.Minute)
	app, rnd := lsBundleWith(t, tempRoot(t), exeBuilt)
	a := Native()
	as := platform.Association{Scheme: "telatest-" + rnd}
	_, err := a.Register(app, as, platform.RegisterOptions{})
	t.Logf("Register in a temporary folder: %v", err)
	if !errors.Is(err, platform.ErrUnsupported) || !strings.Contains(err.Error(), "temporary folder") {
		t.Errorf("Register in a temporary folder = %v, want ErrUnsupported", err)
	}
	if _, err := a.RegisterApp(app, platform.RegisterOptions{}); !errors.Is(err, platform.ErrUnsupported) {
		t.Errorf("RegisterApp in a temporary folder = %v, want ErrUnsupported", err)
	}
	if s, err := a.Status(app, as); s.Registered || s.Default || err != nil {
		t.Errorf("Status in a temporary folder = %+v, %v", s, err)
	}
	if st, err := a.AppStatus(app); st.Registered || err != nil {
		t.Errorf("AppStatus in a temporary folder = %+v, %v", st, err)
	}
}
