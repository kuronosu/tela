//go:build windows

package win32

import (
	"bytes"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strings"
	"sync"
	"testing"
	"unsafe"

	winreg "golang.org/x/sys/windows/registry"

	"github.com/kuronosu/tela/internal/platform"
	"github.com/kuronosu/tela/internal/platform/assoc"
)

// Live tests of the app-level services of phase 13 (appservices_windows.go,
// settings.go, assoc). They use the real hotkey table, Credential Manager,
// named mutex and the current user's registry, so they run on Windows only
// and with TELA_LIVE=1, under throwaway app IDs, schemes and extensions that
// they remove at the end.

func requireLiveServices(t *testing.T) {
	t.Helper()
	requireInteractiveDesktop(t)
	if inWine() {
		t.Skip("Wine does not implement Credential Manager, the hotkey table or the Default apps registration these tests check; run them on Windows")
	}
	if os.Getenv("TELA_LIVE") != "1" {
		t.Skip("set TELA_LIVE=1 to run against the real Windows services (hotkeys, Credential Manager, the registry)")
	}
}

var procKeybdEvent = user32.NewProc("keybd_event")

// pressKeys presses the keys in order and releases them in reverse, as a
// user holding the modifiers does.
func pressKeys(vks ...uint8) {
	const keyUp = 2 // KEYEVENTF_KEYUP
	for _, vk := range vks {
		procKeybdEvent.Call(uintptr(vk), 0, 0, 0)
	}
	for _, vk := range slices.Backward(vks) {
		procKeybdEvent.Call(uintptr(vk), 0, keyUp, 0)
	}
}

func TestLiveSettings(t *testing.T) {
	requireLiveServices(t)
	s := readSettings()
	t.Logf("settings: %+v", s)
	// Both accent sources, to compare with Settings > Personalization >
	// Colors on this machine.
	if v, ok := readDWORD(dwmKey, accentColorName); ok {
		t.Logf("DWM\\AccentColor = %#08x -> %v", v, accentFromABGR(v))
	} else {
		t.Logf("DWM\\AccentColor is not set")
	}
	if available(procDwmGetColorizationColor) {
		var c, opaque uint32
		if hr, _, _ := procDwmGetColorizationColor.Call(uintptr(unsafe.Pointer(&c)), uintptr(unsafe.Pointer(&opaque))); hr == 0 {
			t.Logf("DwmGetColorizationColor = %#08x -> %v", c, accentFromARGB(c))
		}
	}
	if p, ok := readDWORD(accessibilityKey, textScaleFactorName); ok {
		t.Logf("TextScaleFactor = %d%%", p)
	}
	if !s.HasAccentColor {
		t.Errorf("no accent color on a desktop Windows")
	}
	if s.TextScale < 1 || s.TextScale > 2.25 {
		t.Errorf("TextScale = %v", s.TextScale)
	}
	platform.CheckSystemSettings(s)
}

func TestLiveGlobalShortcut(t *testing.T) {
	requireLiveServices(t)
	b1, b2 := liveBackend(t), liveBackend(t)
	// Ctrl+Alt+Shift+F19 and F20: keys few keyboards have and no program
	// takes.
	acc := platform.Accelerator{Mods: platform.ModCtrl | platform.ModAlt | platform.ModShift, Key: "F19"}
	if err := b1.Desktop().RegisterShortcut(platform.Shortcut{ID: "live-a", Accelerator: acc, Description: "Live test"}); err != nil {
		t.Fatalf("RegisterShortcut: %v", err)
	}
	// Another registration of the combination is Taken, reported as an
	// event and not as an error.
	if err := b2.Desktop().RegisterShortcut(platform.Shortcut{ID: "live-b", Accelerator: acc, Description: "Live test"}); err != nil {
		t.Fatalf("RegisterShortcut of a taken combination returned %v; the outcome is an event", err)
	}
	var taken []platform.ShortcutStatusEvent
	runLive(t, b2, nil, func(_ platform.Window, e platform.Event) {
		if ev, ok := e.(platform.ShortcutStatusEvent); ok {
			taken = append(taken, ev)
			b2.Quit()
		}
	})
	if len(taken) != 1 || taken[0].ID != "live-b" || taken[0].Status != platform.ShortcutTaken || taken[0].Reason == "" {
		t.Errorf("status of the taken combination = %+v", taken)
	}

	// The first backend holds it: bound, pressed, replaced, released.
	d := b1.Desktop()
	stage := 0
	var seen []string
	defer func() { // also when runLive fails the test on its timeout
		if stage != 3 {
			t.Logf("stage %d; events: %q", stage, seen)
		}
	}()
	runLive(t, b1, nil, func(_ platform.Window, e platform.Event) {
		switch e := e.(type) {
		case platform.ShortcutStatusEvent:
			seen = append(seen, fmt.Sprintf("status %s %v %q", e.ID, e.Status, e.Trigger))
			switch stage {
			case 0:
				if e.Status != platform.ShortcutBound || e.Trigger != "Ctrl+Alt+Shift+F19" {
					t.Errorf("first status = %+v", e)
					b1.Quit()
					return
				}
				stage = 1
				pressKeys(vkControl, vkMenu, vkShift, 0x82) // VK_F19
			case 2: // the combination is free again
				if e.ID != "live-c" || e.Status != platform.ShortcutBound {
					t.Errorf("status after unregistering = %+v", e)
				}
				stage = 3
				b1.Quit()
			}
		case platform.ShortcutEvent:
			seen = append(seen, fmt.Sprintf("event %s %v", e.ID, e.Action))
			if stage == 1 && e.ID == "live-a" && e.Action == platform.ShortcutPressed {
				stage = 2
				_ = d.UnregisterShortcut("live-a")
				_ = d.UnregisterShortcut("unknown") // ignored
				if err := d.RegisterShortcut(platform.Shortcut{ID: "live-c", Accelerator: acc, Description: "Live test"}); err != nil {
					t.Errorf("RegisterShortcut after unregistering: %v", err)
				}
			}
		}
	})
	if stage != 3 {
		t.Errorf("stopped at stage %d; events: %q", stage, seen)
	}
}

func TestLiveSecrets(t *testing.T) {
	requireLiveServices(t)
	d := liveBackend(t).Desktop()
	appID := fmt.Sprintf("com.tela.livetest.%d", os.Getpid())
	t.Cleanup(func() {
		for _, n := range []string{"token", "empty", "big", "other/name"} {
			_ = d.DeleteSecret(appID, n)
		}
	})
	if _, err := d.Secret(appID, "token"); !errors.Is(err, platform.ErrSecretNotFound) || !errors.Is(err, fs.ErrNotExist) {
		t.Fatalf("Secret of a missing name = %v", err)
	}
	if err := d.DeleteSecret(appID, "token"); err != nil {
		t.Errorf("DeleteSecret of a missing name = %v", err)
	}
	value := []byte("s3cr\x00et \xff bytes")
	if err := d.SetSecret(appID, "token", value); err != nil {
		t.Fatalf("SetSecret: %v", err)
	}
	if got, err := d.Secret(appID, "token"); err != nil || !bytes.Equal(got, value) {
		t.Errorf("Secret = %q, %v", got, err)
	}
	// Replace, empty, the largest size, other names and apps.
	if err := d.SetSecret(appID, "token", []byte("v2")); err != nil {
		t.Fatal(err)
	}
	if got, _ := d.Secret(appID, "token"); string(got) != "v2" {
		t.Errorf("replaced secret = %q", got)
	}
	if err := d.SetSecret(appID, "empty", nil); err != nil {
		t.Errorf("SetSecret of an empty secret: %v", err)
	} else if got, err := d.Secret(appID, "empty"); err != nil || len(got) != 0 {
		t.Errorf("Secret of an empty secret = %q, %v", got, err)
	}
	big := bytes.Repeat([]byte{0xA5}, platform.MaxSecretSize)
	if err := d.SetSecret(appID, "big", big); err != nil {
		t.Errorf("SetSecret of MaxSecretSize bytes: %v", err)
	} else if got, _ := d.Secret(appID, "big"); !bytes.Equal(got, big) {
		t.Errorf("big secret came back with %d bytes", len(got))
	}
	if err := d.SetSecret(appID, "big", append(big, 0)); !errors.Is(err, platform.ErrSecretTooLarge) {
		t.Errorf("SetSecret above MaxSecretSize = %v", err)
	}
	_ = d.SetSecret(appID, "other/name", []byte("x"))
	if _, err := d.Secret(appID+"x", "token"); !errors.Is(err, platform.ErrSecretNotFound) {
		t.Errorf("another app sees the secret: %v", err)
	}
	// Safe from any goroutine.
	var wg sync.WaitGroup
	for i := range 4 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			if got, err := d.Secret(appID, "token"); err != nil || string(got) != "v2" {
				t.Errorf("goroutine %d: Secret = %q, %v", i, got, err)
			}
		}()
	}
	wg.Wait()
	if err := d.DeleteSecret(appID, "token"); err != nil {
		t.Fatal(err)
	}
	if _, err := d.Secret(appID, "token"); !errors.Is(err, platform.ErrSecretNotFound) {
		t.Errorf("Secret after DeleteSecret = %v", err)
	}
}

// TestLiveSingleInstance claims one app ID from three places: a second
// backend of this process, a second process (the test binary run again as
// TestLiveInstanceChild), and, after Run returned, a third backend.
func TestLiveSingleInstance(t *testing.T) {
	requireLiveServices(t)
	id := fmt.Sprintf("com.tela.livetest.%d", os.Getpid())
	b1, b2 := liveBackend(t), liveBackend(t)
	primary, err := b1.Desktop().ClaimInstance(id, platform.CurrentLaunch())
	if !primary || err != nil {
		t.Fatalf("first ClaimInstance = %v, %v", primary, err)
	}
	if again, err := b1.Desktop().ClaimInstance(id, platform.Launch{}); !again || err != nil {
		t.Errorf("claiming again in the primary = %v, %v", again, err)
	}
	if _, err := b1.Desktop().ClaimInstance(id+".other", platform.Launch{}); err == nil {
		t.Errorf("a second app ID in one backend was accepted")
	}

	dir := t.TempDir()
	want := []platform.Launch{
		{Args: []string{"notes:open?id=1", "a b", "ünï"}, Dir: dir, Token: "tok"},
		{Args: []string{"from-child"}, Dir: "", Token: ""},
	}
	second, err := b2.Desktop().ClaimInstance(id, want[0])
	if second || err != nil {
		t.Fatalf("ClaimInstance of the same ID from a second backend = %v, %v", second, err)
	}

	child := exec.Command(os.Args[0], "-test.run=^TestLiveInstanceChild$", "-test.v")
	child.Env = append(os.Environ(), "TELA_LIVE_INSTANCE_ID="+id)
	var out bytes.Buffer
	child.Stdout, child.Stderr = &out, &out
	if err := child.Start(); err != nil {
		t.Fatalf("starting the second process: %v", err)
	}
	var got []platform.Launch
	runLive(t, b1, nil, func(w platform.Window, e platform.Event) {
		if ev, ok := e.(platform.LaunchEvent); ok {
			if w != nil {
				t.Errorf("LaunchEvent for window %v; it is app-scoped", w)
			}
			got = append(got, ev.Launch)
			if len(got) == len(want) {
				b1.Quit()
			}
		}
	})
	if err := child.Wait(); err != nil {
		t.Errorf("the second process failed: %v\n%s", err, out.String())
	}
	if !strings.Contains(out.String(), "CHILD primary=false err=<nil>") {
		t.Errorf("the second process did not report primary=false:\n%s", out.String())
	}
	if len(got) != len(want) {
		t.Fatalf("got %d LaunchEvents, want %d: %+v", len(got), len(want), got)
	}
	if !launchEqual(got[0], want[0]) {
		t.Errorf("launch from the second backend = %+v, want %+v", got[0], want[0])
	}
	if !slices.Equal(got[1].Args, want[1].Args) || got[1].Dir == "" {
		t.Errorf("launch from the second process = %+v (want args %q and its working directory)", got[1], want[1].Args)
	}

	// Run returned: the claim is released.
	b3 := liveBackend(t)
	if p, err := b3.Desktop().ClaimInstance(id, platform.Launch{}); !p || err != nil {
		t.Errorf("ClaimInstance after the first instance ended = %v, %v", p, err)
	}
}

func launchEqual(a, b platform.Launch) bool {
	return a.Dir == b.Dir && a.Token == b.Token && slices.Equal(a.Args, b.Args)
}

// TestLiveInstanceChild is the second process of TestLiveSingleInstance.
func TestLiveInstanceChild(t *testing.T) {
	id := os.Getenv("TELA_LIVE_INSTANCE_ID")
	if id == "" {
		t.Skip("the second process of TestLiveSingleInstance")
	}
	b := liveBackend(t)
	dir, _ := os.Getwd()
	primary, err := b.Desktop().ClaimInstance(id, platform.Launch{Args: []string{"from-child"}, Dir: dir})
	fmt.Printf("CHILD primary=%v err=%v\n", primary, err)
}

// TestLiveAssociations registers a throwaway scheme and extension in the
// real HKEY_CURRENT_USER and removes them.
func TestLiveAssociations(t *testing.T) {
	requireLiveServices(t)
	exe, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	pid := os.Getpid()
	app := platform.HandlerApp{ID: fmt.Sprintf("com.tela.livetest.%d", pid), Name: "Tela live test", Exec: exe}
	sch := platform.Association{Scheme: fmt.Sprintf("telalive%d", pid)}
	file := platform.Association{Extension: fmt.Sprintf(".telalive%d", pid), MIMEType: "application/x-tela-live", Description: "Tela live test document"}
	taken := platform.Association{Scheme: fmt.Sprintf("telataken%d", pid)}
	a := assoc.Native()
	cleanup := func() {
		for _, as := range []platform.Association{sch, file, taken} {
			_, _ = a.Unregister(app, as)
		}
		for _, k := range []string{
			`Software\Classes\` + taken.Scheme,
			`Software\Classes\` + sch.Scheme,
			`Software\Classes\` + file.Extension,
			`Software\Classes\` + app.ID + "." + file.Extension[1:],
			`Software\` + app.ID,
		} {
			deleteTree(k)
		}
		if rk, err := winreg.OpenKey(winreg.CURRENT_USER, `Software\RegisteredApplications`, winreg.SET_VALUE); err == nil {
			_ = rk.DeleteValue(app.ID)
			rk.Close()
		}
	}
	cleanup()
	t.Cleanup(cleanup)

	for _, as := range []platform.Association{sch, file} {
		if s, err := a.Status(app, as); err != nil || s != (platform.AssociationStatus{}) {
			t.Fatalf("Status(%v) before = %+v, %v", as, s, err)
		}
		if changed, err := a.Register(app, as, platform.RegisterOptions{}); !changed || err != nil {
			t.Fatalf("Register(%v) = %v, %v", as, changed, err)
		}
		if changed, err := a.Register(app, as, platform.RegisterOptions{}); changed || err != nil {
			t.Errorf("second Register(%v) = %v, %v (nothing should change)", as, changed, err)
		}
		if s, err := a.Status(app, as); err != nil || !s.Registered || !s.Default {
			t.Errorf("Status(%v) after Register = %+v, %v", as, s, err)
		}
	}
	for key, want := range map[string]string{
		`Software\Classes\` + sch.Scheme + `\shell\open\command`:                        `"` + exe + `" "%1"`,
		`Software\Classes\` + app.ID + "." + file.Extension[1:] + `\shell\open\command`: `"` + exe + `" "%1"`,
		`Software\Classes\` + file.Extension:                                            app.ID + "." + file.Extension[1:],
	} {
		name := ""
		if k, err := winreg.OpenKey(winreg.CURRENT_USER, key, winreg.QUERY_VALUE); err != nil {
			t.Errorf("key %s: %v", key, err)
		} else {
			if v, _, err := k.GetStringValue(name); err != nil || v != want {
				t.Errorf("%s = %q, %v; want %q", key, v, err, want)
			}
			k.Close()
		}
	}
	// A scheme another application holds is refused, and Replace takes it.
	tk, _, err := winreg.CreateKey(winreg.CURRENT_USER, `Software\Classes\`+taken.Scheme+`\shell\open\command`, winreg.SET_VALUE)
	if err != nil {
		t.Fatal(err)
	}
	_ = tk.SetStringValue("", `"C:\Other\other.exe" "%1"`)
	tk.Close()
	if _, err := a.Register(app, taken, platform.RegisterOptions{}); !errors.Is(err, platform.ErrAssociationTaken) {
		t.Errorf("Register of a held scheme = %v", err)
	}
	if changed, err := a.Register(app, taken, platform.RegisterOptions{Replace: true}); !changed || err != nil {
		t.Errorf("Register with Replace = %v, %v", changed, err)
	}
	// A scheme without a UserChoice is the default as soon as it is
	// registered; so is an extension nothing else opens. Neither opens the
	// settings page.
	for _, as := range []platform.Association{sch, file} {
		if res, err := a.MakeDefault(app, as); res != platform.DefaultSet || err != nil {
			t.Errorf("MakeDefault(%v) = %v, %v", as, res, err)
		}
	}
	for _, as := range []platform.Association{sch, file, taken} {
		if changed, err := a.Unregister(app, as); !changed || err != nil {
			t.Errorf("Unregister(%v) = %v, %v", as, changed, err)
		}
		if s, err := a.Status(app, as); err != nil || s.Registered || s.Default {
			t.Errorf("Status(%v) after Unregister = %+v, %v", as, s, err)
		}
	}
	for _, key := range []string{`Software\Classes\` + sch.Scheme, `Software\Classes\` + file.Extension, `Software\` + app.ID} {
		if k, err := winreg.OpenKey(winreg.CURRENT_USER, key, winreg.QUERY_VALUE); err == nil {
			k.Close()
			t.Errorf("key %s is still there after Unregister", key)
		}
	}
}

// deleteTree removes a key of the current user and the keys below it.
func deleteTree(key string) {
	if k, err := winreg.OpenKey(winreg.CURRENT_USER, key, winreg.ENUMERATE_SUB_KEYS); err == nil {
		subs, _ := k.ReadSubKeyNames(-1)
		k.Close()
		for _, s := range subs {
			deleteTree(key + `\` + s)
		}
	}
	_ = winreg.DeleteKey(winreg.CURRENT_USER, key)
}

// TestLiveMoveToTrash recycles a temporary file through IFileOperation:
// it leaves its place, from a goroutine that is not the UI thread, and a
// missing file is fs.ErrNotExist. The file stays in the Recycle Bin.
func TestLiveMoveToTrash(t *testing.T) {
	requireLiveServices(t)
	d := liveBackend(t).Desktop()
	path := filepath.Join(t.TempDir(), fmt.Sprintf("tela-trash-%d.txt", os.Getpid()))
	if err := os.WriteFile(path, []byte("recycle me"), 0o644); err != nil {
		t.Fatal(err)
	}
	done := make(chan error, 1)
	go func() { done <- d.MoveToTrash(path) }()
	if err := <-done; err != nil {
		t.Fatalf("MoveToTrash: %v", err)
	}
	if _, err := os.Stat(path); !errors.Is(err, fs.ErrNotExist) {
		t.Errorf("the file is still at its place after MoveToTrash: %v", err)
	}
	if err := d.MoveToTrash(path); !errors.Is(err, fs.ErrNotExist) {
		t.Errorf("MoveToTrash of a missing file = %v, want fs.ErrNotExist", err)
	}
}
