package core

import (
	"fmt"
	"os"
	"runtime/debug"
	"strings"
	"sync"
)

// Dev mode is on by default unless the binary is a release build (ADR-085):
// an explicit choice (tela.App.Dev, telatest.NoDev) overrides the default,
// and TELA_DEV=1 or TELA_DEV=0 overrides both.

// ReleaseBuild reports whether the running binary is a release build, the
// one kind of binary whose dev mode is off by default (ADR-085). It is
// decided once from the build information the go command embeds
// (runtime/debug.ReadBuildInfo):
//
//   - a test binary (go test, and the runner of tela snapshot) is never a
//     release build;
//   - a binary built with -trimpath is one: the flag is what release
//     builds pass (reproducible paths), and nobody passes it to go run;
//   - a binary whose main module has a version and no version-control
//     stamp is one: go install example.com/app@v1.2.0 builds from the
//     module cache, which cannot hold local edits;
//   - everything else is not: go run . has the version "(devel)", and go
//     build in a checkout stamps a pseudo-version together with vcs.*
//     settings (Go 1.24 and later), with "+dirty" for local edits.
//
// A binary without build information counts as a development build.
func ReleaseBuild() bool { return releaseBuild() }

var releaseBuild = sync.OnceValue(func() bool {
	bi, ok := debug.ReadBuildInfo()
	return releaseFrom(bi, ok)
})

// releaseFrom is the rule of ReleaseBuild applied to bi.
func releaseFrom(bi *debug.BuildInfo, ok bool) bool {
	if !ok || bi == nil {
		return false
	}
	if strings.HasSuffix(bi.Path, ".test") {
		return false
	}
	vcs := false
	for _, s := range bi.Settings {
		if s.Key == "-trimpath" && s.Value == "true" {
			return true
		}
		if s.Key == "vcs" || strings.HasPrefix(s.Key, "vcs.") {
			vcs = true
		}
	}
	v := bi.Main.Version
	return v != "" && v != "(devel)" && !vcs
}

// ResolveDev returns whether dev mode is on for a tree or an app:
// TELA_DEV=1 or TELA_DEV=0 decides when set; otherwise the explicit
// choice, when explicit is not nil; otherwise whether the binary is not
// a release build (ReleaseBuild). Other values of TELA_DEV are ignored.
func ResolveDev(explicit *bool) bool {
	switch os.Getenv(EnvDev) {
	case "1":
		return true
	case "0":
		return false
	}
	if explicit != nil {
		return *explicit
	}
	return !ReleaseBuild()
}

// configDev applies TELA_DEV to the Dev field of a Config, which callers
// resolve with ResolveDev: the environment overrides it.
func configDev(dev bool) bool {
	switch os.Getenv(EnvDev) {
	case "1":
		return true
	case "0":
		return false
	}
	return dev
}

// ---------------------------------------------------------------------
// Warnings.

// warnf sends a dev warning: code that works as written but is likely a
// mistake (Config.Warn; Config.Log when it is nil). Safe from any
// goroutine when the log functions are.
func (t *Tree) warnf(format string, args ...any) {
	if t.warn != nil {
		t.warn(fmt.Sprintf(format, args...))
	}
}

// once reports whether key is given for the first time in this tree,
// recording it: the one deduplication of the tree's messages given once
// per call site or subject (warnings, unloadable images, invalid animation
// specs), whose keys carry a prefix naming their kind ("dep:", "anon:",
// "bg-url:"...). Class errors have their own set (Tree.classReported),
// which a new theme clears; this one lives as long as the tree. Safe from
// any goroutine.
func (t *Tree) once(key string) bool {
	t.onceMu.Lock()
	defer t.onceMu.Unlock()
	if t.onceSeen[key] {
		return false
	}
	if t.onceSeen == nil {
		t.onceSeen = map[string]bool{}
	}
	t.onceSeen[key] = true
	return true
}

// warnOnce sends the warning msg() the first time key is warned about in
// this tree (once). Safe from any goroutine.
func (t *Tree) warnOnce(key string, msg func() string) {
	if t.warn != nil && t.once(key) {
		t.warn(msg())
	}
}

// reportOnce logs msg, an error, the first time key is reported in this
// tree (once).
func (t *Tree) reportOnce(key, msg string) {
	if t.once(key) {
		t.logf("%s", msg)
	}
}
