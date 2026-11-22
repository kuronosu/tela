package main

import (
	"os/exec"
	"strings"
	"testing"
)

// TestMCPCheckAndVerifyTools runs the check and verify tools of the server
// on the test screens: the same findings as the commands, as text with the
// source lines.
func TestMCPCheckAndVerifyTools(t *testing.T) {
	skipShort(t)
	goBin, err := exec.LookPath("go")
	if err != nil {
		t.Fatal(err)
	}
	c := startMCP(t, goBin, mcpOptions{Pkg: "./testdata/checkscreens", W: 320, H: 160, Scale: 1, Preview: "Clean"})
	c.request("initialize", map[string]any{"protocolVersion": "2025-06-18", "capabilities": map[string]any{}, "clientInfo": map[string]any{"name": "test", "version": "1"}})

	r := c.tool("check", map[string]any{"preview": "Broken"})
	if r.isError {
		t.Fatalf("check:\n%s", r.text)
	}
	for _, want := range []string{"checkscreens.go:30: color-contrast (serious)", `element: text "Quarterly revenue"`, "fix: ", "in: Broken / light, dark", "tela check: 4 findings"} {
		if !strings.Contains(r.text, want) {
			t.Errorf("check lacks %q:\n%s", want, r.text)
		}
	}
	r = c.tool("check", map[string]any{"preview": "Clean"})
	if r.isError || !strings.Contains(r.text, "nothing found") {
		t.Errorf("check of the clean screen:\n%s", r.text)
	}

	r = c.tool("verify", map[string]any{"preview": "Narrow", "widths": "360"})
	if r.isError || !strings.Contains(r.text, "in: Narrow / width 360") || !strings.Contains(r.text, "text-clipped") {
		t.Errorf("verify:\n%s", r.text)
	}
	r = c.tool("verify", map[string]any{"rules": "nope"})
	if !r.isError || !strings.Contains(r.text, `unknown rule "nope"`) {
		t.Errorf("an unknown rule: error %v:\n%s", r.isError, r.text)
	}
}
