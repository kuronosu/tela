// Package appver holds the application's version (ADR-294): the one tela
// build and tela package write into the binary with the linker's -X flag,
// else the main module's version as the go command stamped it.
//
// It imports nothing from Tela, so every layer may import it.
package appver

import "runtime/debug"

// version is set by tela build and tela package:
//
//	-ldflags "-X github.com/kuronosu/tela/internal/appver.version=1.2.0"
var version string

// Version returns the application's version without a leading "v", or ""
// when nothing stamped one (go run, a build outside a tagged checkout,
// "(devel)").
func Version() string {
	if version != "" {
		return trimV(version)
	}
	if bi, ok := debug.ReadBuildInfo(); ok {
		if v := bi.Main.Version; v != "" && v != "(devel)" {
			return trimV(v)
		}
	}
	return ""
}

func trimV(v string) string {
	if len(v) > 1 && v[0] == 'v' && v[1] >= '0' && v[1] <= '9' {
		return v[1:]
	}
	return v
}
