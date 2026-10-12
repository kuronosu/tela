package main

import (
	"bytes"
	"runtime/debug"
	"strings"
	"testing"
)

func TestVersionString(t *testing.T) {
	for _, tc := range []struct {
		info *debug.BuildInfo
		ok   bool
		want string
	}{
		{nil, false, "tela (unknown) "},
		{&debug.BuildInfo{Main: debug.Module{Version: "v0.3.0"}}, true, "tela v0.3.0 "},
		{&debug.BuildInfo{Main: debug.Module{Version: "(devel)"}, Settings: []debug.BuildSetting{
			{Key: "vcs.revision", Value: "fc44fd2a1b2c3d4e"}, {Key: "vcs.modified", Value: "true"}}}, true, "tela (devel) (fc44fd2-dirty) "},
	} {
		if got := versionString(tc.info, tc.ok); !strings.HasPrefix(got, tc.want) {
			t.Errorf("versionString = %q, want prefix %q", got, tc.want)
		}
	}
}

func TestRunVersion(t *testing.T) {
	var out, errOut bytes.Buffer
	if code := run([]string{"version"}, &out, &errOut); code != 0 || !strings.HasPrefix(out.String(), "tela ") {
		t.Errorf("tela version: code %d, stdout %q, stderr %q", code, out.String(), errOut.String())
	}
	if code := run([]string{"version", "x"}, &out, &errOut); code != 2 {
		t.Errorf("tela version x: code %d, want 2", code)
	}
}
