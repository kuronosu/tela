package main

import (
	"regexp"
	"strings"
	"testing"
)

// srcToken is the source line `tela inspect` and `tela mcp` print at the
// end of an element's key.
var srcToken = regexp.MustCompile(` \[src=[^\]\s]+\]`)

// stripSrc removes the [src=file:line] tokens of an aria snapshot, to
// compare it with one made without sources.
func stripSrc(s string) string { return srcToken.ReplaceAllString(s, "") }

// TestInspectSources: `tela inspect` prints the line that made each
// element, in the aria snapshot and in the layout JSON, with the path
// relative to the package's directory.
func TestInspectSources(t *testing.T) {
	skipShort(t)
	out, _ := inspectRun(t, 0, "--size", "400x300", "./testdata/preview")
	if want := `- heading "Hello & welcome" [level=1] [src=preview.go:`; !strings.HasPrefix(out, want) {
		t.Errorf("inspect printed %q, want it to start with %q", out, want)
	}
	layout, _ := inspectRun(t, 0, "--layout", "--size", "400x300", "./testdata/preview")
	if !regexp.MustCompile(`"src": "preview\.go:\d+"`).MatchString(layout) {
		t.Errorf("the layout JSON has no src:\n%s", layout)
	}
}
