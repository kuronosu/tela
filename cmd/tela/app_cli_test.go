package main

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/kuronosu/tela/internal/testutil/golden"
)

// The integration tests of the seeded previews, the package's App and the
// forwarded dev messages (ADR-092) run `go test` in child processes, on the
// packages of testdata.

// boxRect returns the layout rectangle of the box with the key, from
// `tela inspect --layout`.
func boxRect(t *testing.T, stdout, key string) (x, y, w, h int) {
	t.Helper()
	var root box
	if err := json.Unmarshal([]byte(stdout), &root); err != nil {
		t.Fatalf("output is not JSON: %v\n%s", err, stdout)
	}
	n := root.find(key)
	if n == nil {
		t.Fatalf("no box with key %q in\n%s", key, stdout)
	}
	return int(n.Rect.X), int(n.Rect.Y), int(n.Rect.Width), int(n.Rect.Height)
}

// TestSeededPreviews: a struct component root seeds its local state, a Wrap
// seeds a context, and the package's App sets the window.
func TestSeededPreviews(t *testing.T) {
	skipShort(t)
	// Local state seeded through a field of the struct component.
	out, _ := inspectRun(t, 0, "--preview", "1 Open", "./testdata/seeded")
	if !strings.Contains(out, "tab=open user=guest") {
		t.Errorf("1 Open:\n%s\nwant tab=open and the context's default user", out)
	}
	// Context state seeded by Wrap.
	out, _ = inspectRun(t, 0, "--preview", "2 Mocked", "./testdata/seeded")
	if !strings.Contains(out, "tab=closed user=Ana") {
		t.Errorf("2 Mocked:\n%s\nwant the field and the provided user", out)
	}

	// The image is the App's window: 320x140, a slate-800 root, dark.
	img, stdout, _ := snap(t, "--preview", "2 Mocked", "./testdata/seeded")
	if b := img.Bounds(); b.Dx() != 320 || b.Dy() != 140 {
		t.Errorf("image is %v, want the App's 320x140", b)
	}
	if !strings.Contains(stdout, `preview "2 Mocked"`) {
		t.Errorf("stdout %q", stdout)
	}
	layout, _ := inspectRun(t, 0, "--layout", "--preview", "2 Mocked", "./testdata/seeded")
	if got := pixel(img, 318, 138); got != [3]uint8{0x1d, 0x29, 0x3d} {
		t.Errorf("RootClass: corner %v, want slate-800", got)
	}
	// The theme token and its alias (the dark value, blue), and the dark:
	// variant of the App's color scheme, with no --dark.
	x, y, w, h := boxRect(t, layout, "token")
	if got := pixel(img, x+w/2, y+h/2); got != [3]uint8{0, 0, 0xff} {
		t.Errorf("Theme and ClassAliases: swatch %v, want the dark value of brand (blue)", got)
	}
	x, y, w, h = boxRect(t, layout, "scheme")
	if got := pixel(img, x+w/2, y+h/2); got != [3]uint8{0x00, 0xbc, 0x7d} {
		t.Errorf("ColorScheme: box %v, want emerald-500 (dark: variant)", got)
	}
	golden.Image(t, "seeded_mocked", img)

	// --size beats the App's size.
	img, _, _ = snap(t, "--size", "200x100", "--preview", "1 Open", "./testdata/seeded")
	if b := img.Bounds(); b.Dx() != 200 || b.Dy() != 100 {
		t.Errorf("--size: image is %v, want 200x100", b)
	}
	// The scale applies to the App's size.
	img, _, _ = snap(t, "--scale", "2", "./testdata/seeded")
	if b := img.Bounds(); b.Dx() != 640 || b.Dy() != 280 {
		t.Errorf("--scale 2: image is %v, want 640x280", b)
	}
}

// A preview that is a struct component, with no node around it.
func TestPreviewComponent(t *testing.T) {
	skipShort(t)
	out, _ := inspectRun(t, 0, "./testdata/component")
	if out != "- heading \"Card 3\" [level=1]\n" {
		t.Errorf("inspect: %q", out)
	}
	img, _, _ := snap(t, "--size", "60x40", "./testdata/component")
	if got := pixel(img, 58, 38); got != [3]uint8{0xcb, 0xfb, 0xf1} {
		t.Errorf("background %v, want teal-100", got)
	}
}

// A package with only an App: its Root is the screen.
func TestAppOnly(t *testing.T) {
	skipShort(t)
	img, stdout, _ := snap(t, "./testdata/apponly")
	if b := img.Bounds(); b.Dx() != 200 || b.Dy() != 100 {
		t.Errorf("image is %v, want the App's 200x100", b)
	}
	if got := pixel(img, 190, 90); got != [3]uint8{0xff, 0xd2, 0x30} {
		t.Errorf("background %v, want amber-300", got)
	}
	if strings.Contains(stdout, "preview") {
		t.Errorf("stdout %q: App alone is a single screen", stdout)
	}
	if out, _ := inspectRun(t, 0, "./testdata/apponly"); out != "- heading \"n=5\" [level=1]\n" {
		t.Errorf("inspect: %q", out)
	}
	if msg := snapFails(t, "--preview", "X", "./testdata/apponly"); !strings.Contains(msg, "defines App") {
		t.Errorf("stderr:\n%s", msg)
	}
	// The App's scheme beats --dark, as in a window, and the CLI says so.
	img, _, stderr := snap(t, "--dark", "./testdata/apponly")
	if got := pixel(img, 190, 90); got != [3]uint8{0xff, 0xd2, 0x30} {
		t.Errorf("--dark with App.ColorScheme Light: background %v, want amber-300", got)
	}
	if !strings.Contains(stderr, "--dark has no effect") {
		t.Errorf("stderr %q lacks the --dark warning", stderr)
	}
}

// The dev warnings and class errors of the screen reach stderr, and stdout
// keeps only the output of the command.
func TestDevMessagesForwarded(t *testing.T) {
	skipShort(t)
	stdout, stderr := inspectRun(t, 0, "./testdata/devwarn")
	if strings.Contains(stdout, "flex-colum") || strings.Contains(stdout, "key") {
		t.Errorf("stdout has dev messages:\n%s", stdout)
	}
	for _, want := range []string{"flex-colum", "key"} {
		if !strings.Contains(stderr, want) {
			t.Errorf("stderr lacks %q:\n%s", want, stderr)
		}
	}
	_, _, stderr = snap(t, "--size", "100x60", "./testdata/devwarn")
	if !strings.Contains(stderr, "flex-colum") {
		t.Errorf("snapshot stderr lacks the class error:\n%s", stderr)
	}
	// App.Dev = false turns them off.
	if _, stderr := inspectRun(t, 0, "./testdata/devoff"); stderr != "" {
		t.Errorf("stderr with Dev off: %q", stderr)
	}
}
