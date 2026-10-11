// Command wlgen generates Go bindings from Wayland protocol XML files.
//
// Usage:
//
//	wlgen [-package name] -out dir <protocol.xml | dir>...
//
// Directories contribute their *.xml files. One Go file is written per
// protocol, all in one package so protocols can reference each other's
// interfaces and enums. Every generated file carries a "//go:build linux"
// constraint because the wire runtime needs Linux system calls; the generator
// itself is portable.
//
// See package protocols for the //go:generate line that drives it.
package main

import (
	"flag"
	"fmt"
	"os"
	"path/filepath"
)

func main() {
	if err := run(os.Args[1:]); err != nil {
		fmt.Fprintln(os.Stderr, "wlgen:", err)
		os.Exit(1)
	}
}

func run(args []string) error {
	fs := flag.NewFlagSet("wlgen", flag.ContinueOnError)
	out := fs.String("out", "", "directory to write the generated Go files to (required)")
	pkg := fs.String("package", "protocols", "Go package name of the generated files")
	if err := fs.Parse(args); err != nil {
		return err
	}
	if *out == "" || fs.NArg() == 0 {
		return errUsage
	}
	protos, err := loadProtocols(fs.Args())
	if err != nil {
		return err
	}
	files, err := generate(protos, *pkg)
	if err != nil {
		return err
	}
	if err := os.MkdirAll(*out, 0o755); err != nil {
		return err
	}
	for name, src := range files {
		if err := os.WriteFile(filepath.Join(*out, name), src, 0o644); err != nil {
			return err
		}
	}
	return nil
}
