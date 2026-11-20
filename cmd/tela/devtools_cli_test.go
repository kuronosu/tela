package main

import (
	"bytes"
	"path/filepath"
	"strings"
	"testing"
)

func TestDevtoolsUsage(t *testing.T) {
	for _, args := range [][]string{{"help", "devtools"}, {"devtools", "-h"}} {
		var out, errb bytes.Buffer
		if code := run(args, &out, &errb); code != 0 || !strings.Contains(out.String(), "tela devtools [-addr ADDRESS]") {
			t.Errorf("tela %v: exit %d, stdout %q, stderr %q", args, code, out.String(), errb.String())
		}
	}
	var out bytes.Buffer
	if code := run([]string{"help"}, &out, &out); code != 0 || !strings.Contains(out.String(), "devtools    open the DevTools") {
		t.Errorf("the command list does not name devtools:\n%s", out.String())
	}
}

func TestDevtoolsErrors(t *testing.T) {
	var out, errb bytes.Buffer
	if code := run([]string{"devtools", "extra"}, &out, &errb); code != 2 || !strings.Contains(errb.String(), `unexpected argument "extra"`) {
		t.Errorf("an argument: exit %d, stderr %q", code, errb.String())
	}
	errb.Reset()
	if code := run([]string{"devtools", "-nope"}, &out, &errb); code != 2 {
		t.Errorf("an unknown flag: exit %d, stderr %q", code, errb.String())
	}
	// A session that is not there: the error says where it tried.
	errb.Reset()
	missing := filepath.Join(t.TempDir(), "tools.sock")
	if code := run([]string{"devtools", "-addr", missing}, &out, &errb); code != 1 || !strings.Contains(errb.String(), "cannot reach the tela dev session at "+missing) {
		t.Errorf("a missing session: exit %d, stderr %q", code, errb.String())
	}
}

func TestDevArgsDevtools(t *testing.T) {
	o, err := parseDevArgs([]string{"--devtools", "./app"})
	if err != nil || !o.DevTools || o.Pkg != "./app" {
		t.Errorf("--devtools: %+v, %v", o, err)
	}
	if o, err := parseDevArgs(nil); err != nil || o.DevTools {
		t.Errorf("by default: %+v, %v", o, err)
	}
	if !strings.Contains(devUsage, "--devtools") {
		t.Error("the usage of tela dev does not name --devtools")
	}
}
