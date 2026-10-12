package main

import (
	"bytes"
	"encoding/json"
	"image"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/kuronosu/tela/internal/testutil/golden"
)

// The integration tests of the Preview and Previews contracts run `go test`
// in child processes, on the packages of testdata.

func skipShort(t *testing.T) {
	t.Helper()
	if testing.Short() {
		t.Skip("runs go test on a package")
	}
}

// snap runs `tela snapshot` and returns the image, stdout and stderr; it
// fails the test on a nonzero exit code.
func snap(t *testing.T, args ...string) (img *image.NRGBA, stdout, stderr string) {
	t.Helper()
	outPath := filepath.Join(t.TempDir(), "out.png")
	var out, errOut bytes.Buffer
	args = append([]string{"snapshot", "-o", outPath}, args...)
	if code := run(args, &out, &errOut); code != 0 {
		t.Fatalf("tela %v: exit code %d\nstdout: %s\nstderr: %s", args, code, out.String(), errOut.String())
	}
	return readPNG(t, outPath), out.String(), errOut.String()
}

// snapFails runs `tela snapshot` expecting exit code 1 and returns stderr.
func snapFails(t *testing.T, args ...string) string {
	t.Helper()
	outPath := filepath.Join(t.TempDir(), "out.png")
	var out, errOut bytes.Buffer
	args = append([]string{"snapshot", "-o", outPath}, args...)
	if code := run(args, &out, &errOut); code != 1 {
		t.Fatalf("tela %v: exit code %d, want 1\nstdout: %s\nstderr: %s", args, code, out.String(), errOut.String())
	}
	if _, err := os.Stat(outPath); err == nil {
		t.Errorf("%s was written for a failed snapshot", outPath)
	}
	return errOut.String()
}

func pixel(img *image.NRGBA, x, y int) [3]uint8 {
	c := img.NRGBAAt(x, y)
	return [3]uint8{c.R, c.G, c.B}
}

func TestSnapshotPreview(t *testing.T) {
	skipShort(t)
	for _, scale := range []string{"1", "2"} {
		img, stdout, _ := snap(t, "--size", "300x160", "--scale", scale, "./testdata/preview")
		golden.Image(t, "preview_x"+scale, img)
		if !strings.HasPrefix(stdout, "wrote ") || strings.Contains(stdout, "preview \"") {
			t.Errorf("stdout %q: want a plain \"wrote\" line for a package with Preview()", stdout)
		}
	}
	// slate-100 background by default; --dark selects dark:bg-slate-900.
	light, _, _ := snap(t, "--size", "300x160", "./testdata/preview")
	dark, _, _ := snap(t, "--size", "300x160", "--dark", "./testdata/preview")
	if got := pixel(light, 2, 2); got != [3]uint8{0xf1, 0xf5, 0xf9} {
		t.Errorf("light background %v, want slate-100 (#f1f5f9)", got)
	}
	if got := pixel(dark, 2, 2); got != [3]uint8{0x0f, 0x17, 0x2b} {
		t.Errorf("dark background %v, want slate-900 (#0f172b)", got)
	}
}

func TestSnapshotPreviews(t *testing.T) {
	skipShort(t)
	// Without --preview, the first name in byte order: Alert (amber).
	img, stdout, _ := snap(t, "--size", "240x120", "./testdata/previews")
	if got := pixel(img, 2, 2); got != [3]uint8{0xfe, 0xf3, 0xc6} {
		t.Errorf("default preview background %v, want amber-100 (Alert)", got)
	}
	if !strings.Contains(stdout, `(preview "Alert"; others: Counter)`) {
		t.Errorf("stdout %q does not name the rendered preview and the others", stdout)
	}
	golden.Image(t, "previews_alert", img)

	// The stateful preview builds as a component, so its hook works.
	img, stdout, _ = snap(t, "--size", "240x120", "--preview", "Counter", "./testdata/previews")
	if got := pixel(img, 2, 2); got != [3]uint8{0xe0, 0xe7, 0xff} {
		t.Errorf("Counter background %v, want indigo-100", got)
	}
	if !strings.Contains(stdout, `(preview "Counter"; others: Alert)`) {
		t.Errorf("stdout %q", stdout)
	}
	golden.Image(t, "previews_counter", img)
}

func TestSnapshotUnknownPreview(t *testing.T) {
	skipShort(t)
	msg := snapFails(t, "--preview", "Nope", "./testdata/previews")
	for _, want := range []string{`unknown preview "Nope"`, "Alert, Counter"} {
		if !strings.Contains(msg, want) {
			t.Errorf("stderr lacks %q:\n%s", want, msg)
		}
	}
	// Case matters, like Go identifiers.
	if msg := snapFails(t, "--preview", "alert", "./testdata/previews"); !strings.Contains(msg, `unknown preview "alert"`) {
		t.Errorf("stderr:\n%s", msg)
	}
}

func TestSnapshotPreviewFlagOnSingleScreen(t *testing.T) {
	skipShort(t)
	msg := snapFails(t, "--preview", "X", "./testdata/preview")
	if !strings.Contains(msg, "defines Preview(), a single screen") || !strings.Contains(msg, "tela.Previews") {
		t.Errorf("stderr:\n%s", msg)
	}
}

func TestSnapshotContractErrors(t *testing.T) {
	skipShort(t)
	msg := snapFails(t, "./testdata/empty")
	for _, want := range []string{"cannot be rendered", "func Preview() tela.Node", "tela.Previews", "Scene"} {
		if !strings.Contains(msg, want) {
			t.Errorf("empty package: stderr lacks %q:\n%s", want, msg)
		}
	}
	msg = snapFails(t, "./testdata/wrongtype")
	if !strings.Contains(msg, "its Preview has the wrong type") || !strings.Contains(msg, "func Preview() tela.Node") {
		t.Errorf("mistyped Preview: stderr:\n%s", msg)
	}
}

func TestSnapshotSceneIgnoresDarkWithWarning(t *testing.T) {
	skipShort(t)
	_, _, stderr := snap(t, "--size", "100x80", "--dark", "./testdata/scene")
	if !strings.Contains(stderr, "--dark has no effect") {
		t.Errorf("stderr %q lacks the --dark warning", stderr)
	}
	msg := snapFails(t, "--preview", "A", "./testdata/scene")
	if !strings.Contains(msg, "single scene") {
		t.Errorf("stderr:\n%s", msg)
	}
}

// inspectLayout runs `tela inspect` and returns stdout and stderr.
func inspectRun(t *testing.T, code int, args ...string) (stdout, stderr string) {
	t.Helper()
	var out, errOut bytes.Buffer
	if got := run(append([]string{"inspect"}, args...), &out, &errOut); got != code {
		t.Fatalf("tela inspect %v: exit code %d, want %d\nstdout: %s\nstderr: %s", args, got, code, out.String(), errOut.String())
	}
	return out.String(), errOut.String()
}

// box is the part of an inspect node the tests check.
type box struct {
	Kind string `json:"kind"`
	Key  string `json:"key"`
	Rect struct {
		X, Y, Width, Height float32
	} `json:"rect"`
	Text     string `json:"text"`
	Lines    int    `json:"lines"`
	Children []box  `json:"children"`
}

func (b *box) find(key string) *box {
	if b.Key == key {
		return b
	}
	for i := range b.Children {
		if f := b.Children[i].find(key); f != nil {
			return f
		}
	}
	return nil
}

func TestInspectLayout(t *testing.T) {
	skipShort(t)
	stdout, stderr := inspectRun(t, 0, "--layout", "--size", "400x300", "./testdata/preview")
	if stderr != "" {
		t.Errorf("stderr %q, want none for a single screen", stderr)
	}
	goldenText(t, "inspect_preview.json", stdout)

	// The default mode is the layout tree.
	if def, _ := inspectRun(t, 0, "--size", "400x300", "./testdata/preview"); def != stdout {
		t.Errorf("inspect without --layout differs from inspect --layout")
	}

	var root box
	if err := json.Unmarshal([]byte(stdout), &root); err != nil {
		t.Fatalf("output is not JSON: %v\n%s", err, stdout)
	}
	// By hand: the page is 400x300 (min-h-screen) with p-4 and gap-2. The
	// title is 24px text with a 32px line (text-2xl), so it is 368x32 at
	// (16, 16); the row follows after the 8px gap, at y = 16 + 32 + 8 = 56,
	// with two 80x40 boxes 8px apart.
	for key, want := range map[string][4]float32{
		"title": {16, 16, 368, 32},
		"row":   {16, 56, 368, 40},
		"a":     {16, 56, 80, 40},
		"b":     {104, 56, 80, 40},
	} {
		n := root.find(key)
		if n == nil {
			t.Fatalf("no box with key %q in\n%s", key, stdout)
		}
		if got := [4]float32{n.Rect.X, n.Rect.Y, n.Rect.Width, n.Rect.Height}; got != want {
			t.Errorf("%s: rect %v, want %v (x, y, width, height)", key, got, want)
		}
	}
	if title := root.find("title"); title.Text != "Hello & welcome" {
		t.Errorf("text %q: HTML characters must not be escaped", title.Text)
	}
	if !strings.Contains(stdout, "\n  \"kind\": \"root\"") {
		t.Errorf("output is not indented with two spaces:\n%.200s", stdout)
	}
}

func TestInspectDark(t *testing.T) {
	skipShort(t)
	light, _ := inspectRun(t, 0, "--size", "400x300", "./testdata/preview")
	dark, _ := inspectRun(t, 0, "--size", "400x300", "--dark", "./testdata/preview")
	if !strings.Contains(light, `"background": "#f1f5f9"`) || !strings.Contains(dark, `"background": "#0f172b"`) {
		t.Errorf("--dark must switch dark: variants; light has slate-100: %v, dark has slate-900: %v",
			strings.Contains(light, "#f1f5f9"), strings.Contains(dark, "#0f172b"))
	}
}

func TestInspectPreviews(t *testing.T) {
	skipShort(t)
	stdout, stderr := inspectRun(t, 0, "--size", "300x200", "./testdata/previews")
	if !strings.Contains(stdout, `"key": "box"`) || !strings.Contains(stderr, `preview "Alert"`) || !strings.Contains(stderr, "Counter") {
		t.Errorf("default preview: stdout\n%s\nstderr %q", stdout, stderr)
	}
	stdout, stderr = inspectRun(t, 0, "--size", "300x200", "--preview", "Counter", "./testdata/previews")
	var root box
	if err := json.Unmarshal([]byte(stdout), &root); err != nil {
		t.Fatal(err)
	}
	if b := root.find("inc"); b == nil || b.Rect.X != 16 || b.Rect.Y != 16 {
		t.Errorf("inc button: %+v in\n%s", b, stdout)
	}
	if stderr != "" {
		t.Errorf("stderr %q, want none when --preview is given", stderr)
	}
	if msg := func() string { _, e := inspectRun(t, 1, "--preview", "x", "./testdata/previews"); return e }(); !strings.Contains(msg, "Alert, Counter") {
		t.Errorf("unknown preview: stderr %q", msg)
	}
}

func TestInspectSceneRejected(t *testing.T) {
	skipShort(t)
	_, stderr := inspectRun(t, 1, "./testdata/scene")
	if !strings.Contains(stderr, "only defines Scene") {
		t.Errorf("stderr %q", stderr)
	}
}

// goldenText compares got with testdata/<name>; TELA_UPDATE=1 rewrites it.
func goldenText(t *testing.T, name, got string) {
	t.Helper()
	path := filepath.Join("testdata", name)
	if os.Getenv(golden.UpdateEnv) == "1" {
		if err := os.WriteFile(path, []byte(got), 0o644); err != nil {
			t.Fatal(err)
		}
		return
	}
	want, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("%v; create it with %s=1 go test and review it", err, golden.UpdateEnv)
	}
	if string(want) != got {
		_ = os.WriteFile(strings.TrimSuffix(path, ".json")+"_actual.json", []byte(got), 0o644)
		t.Errorf("output differs from %s (actual in %s_actual.json); if the change is intended, regenerate with %s=1", path, strings.TrimSuffix(name, ".json"), golden.UpdateEnv)
	}
}

// A Preview declared in a _test.go file is found: the runner is compiled
// by go test, which sees it.
func TestSnapshotPreviewInTestFile(t *testing.T) {
	pkg, err := listPackage("go", "./testdata/testpreview")
	if err != nil {
		t.Fatal(err)
	}
	if c, err := detectContract(pkg.Dir, pkg.GoFiles); err != nil || c != contractPreview {
		t.Fatalf("detectContract(%v) = %v, %v; want Preview", pkg.GoFiles, c, err)
	}
	skipShort(t)
	img, _, _ := snap(t, "--size", "40x30", "./testdata/testpreview")
	if got := pixel(img, 20, 15); got != [3]uint8{0x00, 0xbc, 0x7d} {
		t.Errorf("pixel = %v, want emerald-500", got)
	}
}

// writeFileAtomic replaces the file and leaves no temporary file behind.
func TestWriteFileAtomic(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "sub", "out.png")
	for _, data := range []string{"first", "second"} {
		if err := writeFileAtomic(path, []byte(data)); err != nil {
			t.Fatal(err)
		}
		if got, err := os.ReadFile(path); err != nil || string(got) != data {
			t.Fatalf("read %q, %v; want %q", got, err, data)
		}
	}
	entries, _ := os.ReadDir(filepath.Dir(path))
	if len(entries) != 1 {
		t.Errorf("%d files in the directory, want only out.png", len(entries))
	}
	if err := writeFileAtomic(filepath.Join(path, "x"), []byte("y")); err == nil {
		t.Error("writing under a file succeeded")
	}
}
