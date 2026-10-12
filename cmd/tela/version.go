package main

import (
	"fmt"
	"io"
	"runtime"
	"runtime/debug"
)

const versionUsage = `Usage:

	tela version

Prints the version of the tela module this binary was built from, the VCS
revision when the build recorded one, and the Go version and platform.
`

// runVersion prints the module version from the build information: a
// release tag (v0.3.0) when installed with go install at a version, a
// pseudo-version or "(devel)" otherwise.
func runVersion(args []string, stdout, stderr io.Writer) int {
	if len(args) > 0 {
		fmt.Fprintf(stderr, "tela version: unexpected arguments %q\n\n%s", args, versionUsage)
		return 2
	}
	fmt.Fprintln(stdout, versionString(debug.ReadBuildInfo()))
	return 0
}

// versionString formats the build information as one line, such as
// "tela v0.3.0 (fc44fd2) go1.27.2 linux/amd64".
func versionString(info *debug.BuildInfo, ok bool) string {
	version, rev, dirty := "(unknown)", "", false
	if ok {
		version = info.Main.Version
		if version == "" {
			version = "(devel)"
		}
		for _, s := range info.Settings {
			switch s.Key {
			case "vcs.revision":
				rev = s.Value
			case "vcs.modified":
				dirty = s.Value == "true"
			}
		}
	}
	out := "tela " + version
	if rev != "" {
		if len(rev) > 7 {
			rev = rev[:7]
		}
		if dirty {
			rev += "-dirty"
		}
		out += " (" + rev + ")"
	}
	return out + " " + runtime.Version() + " " + runtime.GOOS + "/" + runtime.GOARCH
}
