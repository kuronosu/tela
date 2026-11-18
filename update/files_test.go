package update

import (
	"io/fs"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

// extractTo unpacks a zip of files into a new stage folder inside a
// temporary folder, and returns the error and the temporary folder.
func extractTo(t *testing.T, files map[string]string) (string, error) {
	t.Helper()
	parent := t.TempDir()
	zf := filepath.Join(parent, "a.zip")
	if err := os.WriteFile(zf, zipOfFiles(t, files), 0o600); err != nil {
		t.Fatal(err)
	}
	stage := filepath.Join(parent, "stage")
	if err := os.Mkdir(stage, 0o700); err != nil {
		t.Fatal(err)
	}
	return parent, extractZip(zf, stage)
}

// A chain of links whose targets each stay inside lexically, but which
// resolve outside on disk, is refused, and nothing is written through it.
// On Windows every link is refused (extractZip).
func TestExtractZipLinkChain(t *testing.T) {
	for _, tc := range []struct {
		name  string
		files map[string]string
		want  string
	}{
		{"a file through a link chain", map[string]string{
			"a/":   "",
			"a/l":  "->..",
			"b":    "->a/l/..",
			"b/x":  "outside",
			"keep": "k",
		}, "inside a link"},
		{"a link resolving outside", map[string]string{
			"a/":  "",
			"a/l": "->..",
			"b":   "->a/l/..",
		}, "points outside its folder"},
		{"a link made before the link it goes through", map[string]string{
			"a/":  "",
			"a/m": "->l/..",
			"a/l": "->..",
			"b":   "->a/m/../..",
		}, "points outside its folder"},
		{"a link inside a link", map[string]string{
			"a/":    "",
			"a/l":   "->..",
			"a/l/m": "->x",
		}, "inside a link"},
		{"a loop", map[string]string{
			"p": "->q",
			"q": "->p",
		}, "loop"},
	} {
		want := tc.want
		if runtime.GOOS == "windows" {
			want = "a Windows install has none"
		}
		parent, err := extractTo(t, tc.files)
		if err == nil || !strings.Contains(err.Error(), want) {
			t.Errorf("%s: %v, want an error with %q", tc.name, err, want)
		}
		if _, err := os.Lstat(filepath.Join(parent, "x")); !os.IsNotExist(err) {
			t.Errorf("%s: a file was written outside the stage folder", tc.name)
		}
	}
}

// The links of a macOS framework go through other links and stay inside;
// Windows refuses them, as every link.
func TestExtractZipFrameworkLinks(t *testing.T) {
	parent, err := extractTo(t, map[string]string{
		"F.framework/F":                     "->Versions/Current/F",
		"F.framework/Resources":             "->Versions/Current/Resources",
		"F.framework/Headers":               "->Versions/Current/Headers", // stripped: dangling
		"F.framework/Versions/A/F":          "lib",
		"F.framework/Versions/A/Resources/": "",
		"F.framework/Versions/Current":      "->A",
	})
	if runtime.GOOS == "windows" {
		if err == nil || !strings.Contains(err.Error(), "a Windows install has none") {
			t.Errorf("extractZip = %v, want the refusal of links", err)
		}
		return
	}
	if err != nil {
		t.Fatal(err)
	}
	stage := filepath.Join(parent, "stage")
	if got := readFile(t, filepath.Join(stage, "F.framework", "F")); got != "lib" {
		t.Errorf("F.framework/F reads %q", got)
	}
	if st, err := os.Lstat(filepath.Join(stage, "F.framework", "Headers")); err != nil || st.Mode()&fs.ModeSymlink == 0 {
		t.Errorf("the dangling Headers link: %v", err)
	}
}
