package main

import (
	"bytes"
	"go/format"
	"image"
	"image/color"
	"image/png"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

// sameCanvas reports whether the canvas of an animated PNG frame is the
// picture img.
func sameCanvas(canvas *image.NRGBA, img image.Image) bool {
	b := img.Bounds()
	if b.Dx() != canvas.Bounds().Dx() || b.Dy() != canvas.Bounds().Dy() {
		return false
	}
	for y := 0; y < b.Dy(); y++ {
		for x := 0; x < b.Dx(); x++ {
			if color.NRGBAModel.Convert(img.At(b.Min.X+x, b.Min.Y+y)) != canvas.NRGBAAt(x, y) {
				return false
			}
		}
	}
	return true
}

// filmOf calls film and returns its result, the animated PNG it wrote and
// the sheet.
func filmOf(t *testing.T, c *mcpClient, apng string, args map[string]any) (mcpToolResult, *apngFile) {
	t.Helper()
	args["apng"] = apng
	r := c.tool("film", args)
	if r.isError {
		t.Fatalf("film %v:\n%s", args, r.text)
	}
	if len(r.images) != 1 {
		t.Fatalf("film returned %d images, want the contact sheet", len(r.images))
	}
	sheet, err := png.Decode(bytes.NewReader(r.images[0]))
	if err != nil {
		t.Fatalf("the contact sheet is not a PNG: %v", err)
	}
	if b := sheet.Bounds(); b.Dx() > sheetMaxSide || b.Dy() > sheetMaxSide {
		t.Errorf("the contact sheet is %v: over %d", b, sheetMaxSide)
	}
	data, err := os.ReadFile(apng)
	if err != nil {
		t.Fatal(err)
	}
	a, err := parseAPNG(data)
	if err != nil {
		t.Fatalf("the animated PNG breaks a rule of the format: %v", err)
	}
	// The first frame is a valid PNG of its own.
	if _, err := png.Decode(bytes.NewReader(data)); err != nil {
		t.Fatalf("the animated PNG does not decode as a PNG: %v", err)
	}
	return r, a
}

// settledScreenshot is the screenshot tool's picture.
func settledScreenshot(t *testing.T, c *mcpClient) image.Image {
	t.Helper()
	r := c.tool("screenshot", nil)
	if r.isError || len(r.images) != 1 {
		t.Fatalf("screenshot:\n%s", r.text)
	}
	img, err := png.Decode(bytes.NewReader(r.images[0]))
	if err != nil {
		t.Fatal(err)
	}
	return img
}

// TestMCPFilmEndToEnd films the motions of testdata/motionapp: frames that
// differ over time and, when the span covers the motion, a last frame that
// is the settled state.
func TestMCPFilmEndToEnd(t *testing.T) {
	skipShort(t)
	goBin, err := exec.LookPath("go")
	if err != nil {
		t.Fatal(err)
	}
	dir := fixtureModule(t, "motionapp")
	t.Chdir(dir)
	c := startMCP(t, goBin, mcpOptions{Pkg: "./app", W: 320, H: 160, Scale: 1, Preview: "Slide"})
	c.request("initialize", map[string]any{"protocolVersion": "2025-06-18", "capabilities": map[string]any{}, "clientInfo": map[string]any{"name": "test", "version": "1"}})

	// A transition: the click starts it, the film shows it move.
	r := c.tool("snapshot", nil)
	slide := refOf(t, r.text, `button "Slide"`)
	r, a := filmOf(t, c, filepath.Join(dir, "slide.png"), map[string]any{"span": 450, "every": 150, "action": map[string]any{"click": slide}})
	if !strings.Contains(r.text, "Filmed 4 frames, 0 to 450 ms every 150 ms") || !strings.Contains(r.text, "### Snapshot") || !strings.Contains(r.text, "slide.png") {
		t.Errorf("the film's text:\n%s", r.text)
	}
	if len(a.frames) != 4 {
		t.Fatalf("%d frames, want 4 (0, 150, 300, 450 ms)", len(a.frames))
	}
	for i := 1; i < 3; i++ {
		if bytes.Equal(a.frames[i-1].canvas.Pix, a.frames[i].canvas.Pix) {
			t.Errorf("frames %d and %d are the same picture: the box did not move", i-1, i)
		}
	}
	if !sameCanvas(a.frames[3].canvas, settledScreenshot(t, c)) {
		t.Error("the last frame, after the 300 ms transition, is not the settled state")
	}
	if sameCanvas(a.frames[0].canvas, settledScreenshot(t, c)) {
		t.Error("the first frame already is the settled state")
	}

	// A spring, filmed from the action; the span ends before it comes to rest.
	r = c.tool("open", map[string]any{"preview": "Spring"})
	if r.isError {
		t.Fatal(r.text)
	}
	bounce := refOf(t, r.text, `button "Bounce"`)
	_, a = filmOf(t, c, filepath.Join(dir, "spring.png"), map[string]any{"span": 300, "every": 100, "action": map[string]any{"click": bounce}})
	if len(a.frames) != 4 {
		t.Fatalf("%d frames of the spring, want 4", len(a.frames))
	}
	for i := 1; i < 4; i++ {
		if bytes.Equal(a.frames[i-1].canvas.Pix, a.frames[i].canvas.Pix) {
			t.Errorf("spring frames %d and %d are the same picture", i-1, i)
		}
	}
	if !sameCanvas(a.frames[0].canvas, a.frames[0].img) {
		t.Error("the first frame is not its own picture")
	}

	// A Presence exit: the toast fades out and is gone at the end.
	r = c.tool("open", map[string]any{"preview": "Toast"})
	if r.isError {
		t.Fatal(r.text)
	}
	toggle := refOf(t, r.text, `button "Toggle"`)
	r = c.tool("click", map[string]any{"ref": toggle})
	if r.isError || !strings.Contains(r.text, `status`) {
		t.Fatalf("the toast did not show:\n%s", r.text)
	}
	r, a = filmOf(t, c, filepath.Join(dir, "toast.png"), map[string]any{"span": 400, "every": 100, "action": map[string]any{"click": toggle}})
	if strings.Contains(r.text, `status "Saved"`) || strings.Contains(r.text, "- status") {
		t.Errorf("the toast is still in the settled snapshot:\n%s", r.text)
	}
	if bytes.Equal(a.frames[0].canvas.Pix, a.frames[1].canvas.Pix) || bytes.Equal(a.frames[1].canvas.Pix, a.frames[2].canvas.Pix) {
		t.Error("the toast does not fade over the first frames")
	}
	if !sameCanvas(a.frames[len(a.frames)-1].canvas, settledScreenshot(t, c)) {
		t.Error("the last frame is not the settled state without the toast")
	}

	// Without an action it films the app as it is; mistakes are errors.
	if r, _ := filmOf(t, c, filepath.Join(dir, "still.png"), map[string]any{"span": 100, "every": 50}); !strings.Contains(r.text, "the app as it is") {
		t.Errorf("a film without an action:\n%s", r.text)
	}
	for _, tc := range []struct {
		args map[string]any
		want string
	}{
		{map[string]any{}, "film needs span"},
		{map[string]any{"span": 5000, "every": 10}, "use an interval of at least"},
		{map[string]any{"span": 100, "action": map[string]any{"resize": map[string]any{"width": 3}}}, "a film starts with one of"},
		{map[string]any{"span": 100, "action": map[string]any{"click": "e999"}}, "unknown ref"},
		{map[string]any{"span": 100, "apng": "../outside.png"}, "inside the module"},
	} {
		if r := c.tool("film", tc.args); !r.isError || !strings.Contains(r.text, tc.want) {
			t.Errorf("film %v: error %v, %s; want %q", tc.args, r.isError, r.text, tc.want)
		}
	}
	// The instance still answers.
	if r := c.tool("snapshot", nil); r.isError {
		t.Errorf("after the failed films: %s", r.text)
	}
}

// TestMCPTestToolEndToEnd drives testdata/mcpapp through a few actions,
// writes the session as a test and runs it.
func TestMCPTestToolEndToEnd(t *testing.T) {
	skipShort(t)
	goBin, err := exec.LookPath("go")
	if err != nil {
		t.Fatal(err)
	}
	dir := mcpAppModule(t)
	t.Chdir(dir)
	// A test file of a person's, in the package from the start (adding one
	// later would rebuild the screen).
	other := filepath.Join(dir, "app", "mine_test.go")
	if err := os.WriteFile(other, []byte("package mcpapp\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	c := startMCP(t, goBin, mcpOptions{Pkg: "./app", W: 480, H: 320, Scale: 1})
	c.request("initialize", map[string]any{"protocolVersion": "2025-06-18", "capabilities": map[string]any{}, "clientInfo": map[string]any{"name": "test", "version": "1"}})

	// Nothing to write before an action.
	if r := c.tool("test", map[string]any{"name": "greeting flow"}); !r.isError || !strings.Contains(r.text, "nothing to write") {
		t.Errorf("test before any action: %v\n%s", r.isError, r.text)
	}
	r := c.tool("snapshot", nil)
	count, name := refOf(t, r.text, `button "Count: 0"`), refOf(t, r.text, `textbox "Name"`)
	c.tool("click", map[string]any{"ref": count})
	c.tool("hover", map[string]any{"role": "button", "name": "Locked"})
	c.tool("fill", map[string]any{"ref": name, "text": "Ana"})
	c.tool("press", map[string]any{"key": "Enter"})
	c.tool("click", map[string]any{"x": 100, "y": 40, "double": true})
	c.tool("wait", map[string]any{"ms": 100})
	// An action that fails is not in the test.
	if r := c.tool("click", map[string]any{"ref": "e999"}); !r.isError {
		t.Fatal("a click on an unknown ref worked")
	}

	r = c.tool("test", map[string]any{"name": "greeting flow"})
	if r.isError || !strings.Contains(r.text, "TestGreetingFlow passes") {
		t.Fatalf("test:\n%s", r.text)
	}
	if strings.Contains(r.text, "rebuilt") {
		t.Errorf("the test file rebuilt the instance:\n%s", r.text)
	}
	path := filepath.Join(dir, "app", "greeting_flow_test.go")
	b, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	src := string(b)
	for _, want := range []string{
		`app := telatest.Mount(t, Previews["Form"], telatest.Size(480, 320))`,
		`app.Click(telatest.ByRole("button", "Count: 0"))`,
		`app.ExpectRole("button", "Count: 1")`,
		`app.Hover(telatest.ByRole("button", "Locked"))`,
		`app.Fill(telatest.ByRole("textbox", "Name"), "Ana")`,
		`app.ExpectValue(telatest.ByRole("textbox", "Name"), "Ana")`,
		`app.Press("Enter")`,
		`app.ExpectText("Hello, Ana!")`,
		`app.ClickAt(100, 40)`,
		`app.ExpectText("The title was double-clicked")`,
		`app.Advance(100 * time.Millisecond)`,
		"func TestGreetingFlow(t *testing.T) {",
	} {
		if !strings.Contains(src, want) {
			t.Errorf("the test lacks %q:\n%s", want, src)
		}
	}
	if strings.Contains(src, "getByNodeId") || strings.Contains(src, "e999") || strings.Contains(src, "[ref=") || strings.Contains(src, "[src=") {
		t.Errorf("the test carries something of the session:\n%s", src)
	}
	if out, err := format.Source(b); err != nil || !bytes.Equal(out, b) {
		t.Errorf("the test is not gofmt'd (%v)", err)
	}
	// The test passes on its own, as `go test` runs it.
	cmd := exec.Command(goBin, "test", "-count=1", "-run", "^TestGreetingFlow$", "./app")
	cmd.Dir = dir
	cmd.Env = append(os.Environ(), "CGO_ENABLED=0", "GOFLAGS=-mod=mod", "GOPROXY=off")
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Errorf("go test of the written file: %v\n%s", err, out)
	}

	// The tool replaces its own file, and refuses one it did not write.
	if r := c.tool("test", map[string]any{"name": "TestGreetingFlow"}); r.isError {
		t.Errorf("writing the same test again:\n%s", r.text)
	}
	if r := c.tool("test", map[string]any{"name": "mine", "path": "mine_test.go"}); !r.isError || !strings.Contains(r.text, "was not written by this tool") {
		t.Errorf("overwriting a person's file: %v\n%s", r.isError, r.text)
	}
	for _, tc := range []struct {
		args map[string]any
		want string
	}{
		{map[string]any{"name": "x", "path": "x.go"}, "ends with _test.go"},
		{map[string]any{"name": "x", "path": "../x_test.go"}, "must be in the package's directory"},
		{map[string]any{"name": "a/b"}, `has '/' in it`},
		{map[string]any{"name": ""}, "needs a name"},
	} {
		if r := c.tool("test", tc.args); !r.isError || !strings.Contains(r.text, tc.want) {
			t.Errorf("test %v: %v %s; want %q", tc.args, r.isError, r.text, tc.want)
		}
	}

	// Other previews and a change of the source: open starts a new mount
	// in the test; an edit starts the recording over.
	c.tool("open", map[string]any{"preview": "Other", "dark": true})
	c.tool("hover", map[string]any{"role": "heading", "name": "Other screen"})
	if r := c.tool("test", map[string]any{"name": "two screens"}); r.isError {
		t.Fatalf("two screens:\n%s", r.text)
	}
	b2, _ := os.ReadFile(filepath.Join(dir, "app", "two_screens_test.go"))
	if !strings.Contains(string(b2), `app = telatest.Mount(t, Previews["Other"],`) || !strings.Contains(string(b2), "telatest.Dark()") || !strings.Contains(string(b2), `Previews["Form"]`) {
		t.Errorf("the second screen is not a second mount:\n%s", b2)
	}
	srcFile := filepath.Join(dir, "app", "mcpapp.go")
	b3, _ := os.ReadFile(srcFile)
	if err := os.WriteFile(srcFile, bytes.Replace(b3, []byte(`"Version 1"`), []byte(`"Version 2"`), 1), 0o644); err != nil {
		t.Fatal(err)
	}
	if r := c.tool("snapshot", nil); r.isError || !strings.Contains(r.text, "rebuilt") {
		t.Fatalf("after the edit:\n%s", r.text)
	}
	if r := c.tool("test", map[string]any{"name": "after the edit"}); !r.isError || !strings.Contains(r.text, "nothing to write") {
		t.Errorf("the recording should start over after an edit: %v\n%s", r.isError, r.text)
	}
}
