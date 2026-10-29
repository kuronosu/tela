// Package chromium is the shared part of the harnesses that regenerate
// Chromium references (TELA_CHROMIUM=1): compiling a page's Tailwind CSS
// with the Tailwind CLI, taking a screenshot and dumping a result computed
// by the page's script. It is used by tests only, and runs external
// programs, so nothing here is exercised unless a harness regenerates.
//
// The Chromium binary is chromium in PATH, or TELA_CHROMIUM_BIN. The
// Tailwind CLI is TELA_TAILWIND, or tailwindcss in PATH; either must be
// installed in a node_modules directory.
package chromium

import (
	"encoding/json"
	"fmt"
	"html"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

// Bin is the Chromium binary: TELA_CHROMIUM_BIN, else "chromium".
func Bin() string {
	if b := os.Getenv("TELA_CHROMIUM_BIN"); b != "" {
		return b
	}
	return "chromium"
}

// Tailwind compiles the Tailwind CSS the page at htmlPath uses into outCSS.
// css is appended to the input stylesheet after the import of Tailwind and
// the source of the classes: the @theme and @font-face rules of the page.
func Tailwind(t testing.TB, htmlPath, outCSS, css string) {
	t.Helper()
	tw := os.Getenv("TELA_TAILWIND")
	if tw == "" {
		var err error
		if tw, err = exec.LookPath("tailwindcss"); err != nil {
			t.Fatal("the Tailwind CLI is needed: set TELA_TAILWIND to node_modules/.bin/tailwindcss")
		}
	}
	// @import "tailwindcss" resolves from the input file's directory; name
	// the package's entry point in the CLI's node_modules instead.
	bin, err := filepath.EvalSymlinks(tw)
	if err != nil {
		t.Fatal(err)
	}
	modules := bin
	for filepath.Base(modules) != "node_modules" {
		next := filepath.Dir(modules)
		if next == modules {
			t.Fatalf("%s is not inside a node_modules directory", tw)
		}
		modules = next
	}
	input := filepath.Join(t.TempDir(), "input.css")
	src := fmt.Sprintf("@import %q;\n@source %q;\n%s", filepath.Join(modules, "tailwindcss", "index.css"), htmlPath, css)
	if err := os.WriteFile(input, []byte(src), 0o644); err != nil {
		t.Fatal(err)
	}
	if out, err := exec.Command(tw, "-i", input, "-o", outCSS).CombinedOutput(); err != nil {
		t.Fatalf("tailwindcss: %v\n%s", err, out)
	}
}

// Screenshot takes a headless screenshot of the page at url, a window of w
// by h CSS pixels at the device scale factor scale, into the PNG shotPath
// (an absolute path). flags are added to the standard ones, for a page that
// loads files (--allow-file-access-from-files) or waits for its fonts
// (--virtual-time-budget=...).
func Screenshot(t testing.TB, url, shotPath string, w, h, scale int, flags ...string) {
	t.Helper()
	args := []string{"--headless", "--disable-gpu", "--hide-scrollbars", "--force-color-profile=srgb",
		fmt.Sprintf("--force-device-scale-factor=%d", scale), fmt.Sprintf("--window-size=%d,%d", w, h)}
	args = append(args, flags...)
	args = append(args, "--screenshot="+shotPath, url)
	if out, err := exec.Command(Bin(), args...).CombinedOutput(); err != nil {
		t.Fatalf("%s: %v\n%s", Bin(), err, out)
	}
}

// DumpJSON loads the page at url in a window of w by h CSS pixels at the
// device scale factor scale, waits for its script, and unmarshals into v
// the JSON that the script wrote between BEGIN and END in the text of
// <pre id="out">.
func DumpJSON(t testing.TB, url string, w, h, scale int, v any) {
	t.Helper()
	cmd := exec.Command(Bin(), "--headless", "--disable-gpu", "--allow-file-access-from-files", "--virtual-time-budget=10000",
		fmt.Sprintf("--force-device-scale-factor=%d", scale), fmt.Sprintf("--window-size=%d,%d", w, h), "--dump-dom", url)
	outb, err := cmd.Output()
	if err != nil {
		t.Fatalf("%s: %v", Bin(), err)
	}
	s := string(outb)
	i, j := strings.Index(s, "BEGIN"), strings.Index(s, "END</pre>")
	if i < 0 || j < 0 {
		t.Fatalf("chromium output has no result:\n%s", s[:min(len(s), 2000)])
	}
	if err := json.Unmarshal([]byte(html.UnescapeString(s[i+5:j])), v); err != nil {
		t.Fatal(err)
	}
}
