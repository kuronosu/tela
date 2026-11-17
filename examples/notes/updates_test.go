package main

import (
	"errors"
	"testing"

	"github.com/kuronosu/tela/telatest"
	"github.com/kuronosu/tela/update"
)

// withUpdates makes the build one that checks for updates, whose check is
// at st, for the rest of the test; it returns the count of restarts.
func withUpdates(t *testing.T, st update.State) *int {
	t.Helper()
	url, key, use, rs := updateURL, updateKey, useUpdate, restart
	t.Cleanup(func() { updateURL, updateKey, useUpdate, restart = url, key, use, rs })
	updateURL, updateKey = "https://example.com/notes/manifest.json", "ed25519:OWsplNJQAWg09GzQZtNSskYuS1sHWDMT4iSqU3RFSbo="
	useUpdate = func(cfg update.Config) update.State {
		if cfg.URL != updateURL || len(cfg.PublicKeys) != 1 || cfg.PublicKeys[0] != update.PublicKey(updateKey) {
			t.Errorf("the update config: %+v", cfg)
		}
		return st
	}
	restarts := new(int)
	restart = func() error { *restarts++; return nil }
	return restarts
}

var restartBtn = telatest.ByRole("button", "Restart")

func readyState(version string) update.State {
	return update.State{Status: update.Ready, Update: &update.Update{Release: update.Release{Version: version}}}
}

// TestUpdateNotice: a ready update shows the notice at the foot of the
// rail, whose Restart starts the new version; nothing shows while the
// check runs, downloads, finds nothing or fails.
func TestUpdateNotice(t *testing.T) {
	sandbox(t)
	restarts := withUpdates(t, readyState("1.1.0"))
	a := mount(t)
	a.ExpectText("Notes 1.1.0 is ready")
	a.Click(restartBtn)
	if *restarts != 1 {
		t.Errorf("Restart restarted %d times", *restarts)
	}

	for _, st := range []update.State{
		{Status: update.Checking},
		{Status: update.UpToDate},
		{Status: update.Downloading, Update: &update.Update{Release: update.Release{Version: "1.1.0"}}, Done: 10, Total: 100},
		{Status: update.Failed, Err: errors.New("no network")},
		{Status: update.Failed, Err: update.ErrNotInstalled},
	} {
		withUpdates(t, st)
		a := mount(t)
		if n := a.Count(restartBtn); n != 0 {
			t.Errorf("status %v: %d Restart buttons", st.Status, n)
		}
		a.ExpectNoText("is ready")
	}
}

// TestUpdatesOff: a build without an update URL and key (go run, tela dev,
// the other tests) does not check.
func TestUpdatesOff(t *testing.T) {
	sandbox(t)
	if updatesOn() {
		t.Fatal("this build checks for updates")
	}
	use := useUpdate
	t.Cleanup(func() { useUpdate = use })
	useUpdate = func(update.Config) update.State {
		t.Error("an update check without an update URL")
		return update.State{}
	}
	mount(t).ExpectText("All notes")
}

// TestGoldenUpdateNotice is the notice of a ready update, light and dark
// (testdata/notes_update.png, notes_update_dark.png).
func TestGoldenUpdateNotice(t *testing.T) {
	sandbox(t)
	withUpdates(t, readyState("1.1.0"))
	a := mount(t)
	a.ExpectText("Notes 1.1.0 is ready")
	a.MouseMove(windowW-60, windowH-120)
	a.Golden("notes_update.png")

	d := mount(t, telatest.Dark())
	d.ExpectText("Notes 1.1.0 is ready")
	d.MouseMove(windowW-60, windowH-120)
	d.Golden("notes_update_dark.png")
}
