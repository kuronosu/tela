package main

import (
	"debug/buildinfo"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"slices"
	"strings"

	"github.com/kuronosu/tela/cmd/tela/internal/appicon"
	"github.com/kuronosu/tela/cmd/tela/internal/winres"
)

// The parts of tela build that the app's declaration and icon (ADR-294)
// change: the version written into the program, the Windows GUI subsystem
// (ADR-127, ADR-295) and the resources of a Windows executable.

// appverVersion is the variable the linker's -X flag sets: the version
// internal/appver reports.
const appverVersion = telaImportPath + "/internal/appver.version"

// linkerFlags returns the -ldflags value to build with: the user's (the
// -ldflags flag, else the one in GOFLAGS, which the flag would replace),
// then tela's own -X flag for -version and -H=windowsgui for a GUI program.
// It returns the flag's own value untouched when tela adds nothing, so
// GOFLAGS applies as for any go command. The user's -H wins over tela's.
func linkerFlags(opts buildOptions, goflags string, gui bool) string {
	var extra []string
	if opts.Version != "" {
		extra = append(extra, "-X "+appverVersion+"="+opts.Version)
	}
	user := opts.Ldflags
	if user == "" {
		for _, f := range splitGoFlags(goflags) {
			if name, value, ok := goFlag(f); ok && name == "ldflags" {
				user = value
			}
		}
	}
	if gui && !slices.ContainsFunc(splitGoFlags(user), func(f string) bool {
		return f == "-H" || strings.HasPrefix(f, "-H=")
	}) {
		extra = append(extra, "-H=windowsgui")
	}
	if len(extra) == 0 {
		return opts.Ldflags
	}
	return strings.TrimSpace(user + " " + strings.Join(extra, " "))
}

// moduleVersionFull returns the main module's version of the Go program at
// path as the build recorded it, without a leading "v", or "" when it
// recorded none ("(devel)").
func moduleVersionFull(path string) string {
	info, err := buildinfo.ReadFile(path)
	if err != nil || info.Main.Version == "(devel)" {
		return ""
	}
	return strings.TrimPrefix(info.Main.Version, "v")
}

// winResources are the resources tela adds to a Windows executable: the
// icon and the version information. The go command takes resources from
// .syso files in the package's folder and opens them by their real path, so
// an overlay cannot bring one in without writing to the user's folder;
// tela adds the resource section to the linked executable instead
// (winres.Patch).
type winResources struct {
	info  winres.Info
	icons []appicon.Image
}

// newWinResources reads what the resources need, before the build starts so
// that a mistake is reported at once: the App (its Title, Publisher and
// Description), the icon (the -icon flag, else icon.svg or icon.png in the
// package's folder; none is fine) and the program's file name.
func newWinResources(opts buildOptions, t *buildTarget, exeName string, log io.Writer) (*winResources, error) {
	src, err := readAppSource(t.pkg, t.deps)
	if err != nil {
		return nil, err
	}
	if err := src.DeclarationErr(); err != nil {
		return nil, fmt.Errorf("%v\nmake it a constant: the executable's version information is written from it", err)
	}
	title := opts.Name
	if title == "" {
		if src.TitleErr != nil {
			return nil, fmt.Errorf("%v\npass the name with -name instead: tela build -name \"My App\" %s", src.TitleErr, opts.Pkg)
		}
		title = strings.TrimSpace(src.Title)
	}
	if title == "" {
		title = packageBaseName(t.pkg)
	}
	r := &winResources{info: winres.Info{
		ProductName:     title,
		CompanyName:     strings.TrimSpace(src.Publisher),
		FileDescription: strings.TrimSpace(src.Description),
		Version:         opts.Version,
		FileName:        exeName,
	}}
	iconPath := opts.Icon
	if iconPath == "" {
		iconPath = appicon.Find(t.pkg.Dir)
	}
	if iconPath != "" {
		s, err := appicon.Load(iconPath)
		if err != nil {
			return nil, err
		}
		if r.icons, err = s.ICOImages(); err != nil {
			return nil, err
		}
		if opts.Verbose {
			fmt.Fprintf(log, "icon %s\n", iconPath)
		}
	} else if opts.Verbose {
		fmt.Fprintf(log, "no icon: put an icon.svg or an icon.png (512 px or more) in %s, or pass -icon\n", t.pkg.Dir)
	}
	return r, nil
}

// apply adds the resources to the executable at path. Without -version the
// file version is the one the go command stamped into the program, which
// only the built program shows. A program that already has resources (the
// package has its own .syso) keeps them, and tela says so.
func (r *winResources) apply(path string, log io.Writer) error {
	info := r.info
	if info.Version == "" {
		info.Version = moduleVersionFull(path)
	}
	exe, err := os.ReadFile(path)
	if err != nil {
		return err
	}
	patched, err := winres.Patch(exe, winres.Resources(r.icons, info))
	if errors.Is(err, winres.ErrHasResources) {
		fmt.Fprintf(log, "tela build: the program already has resources (a .syso of the package): tela adds no icon or version information\n")
		return nil
	}
	if err != nil {
		return fmt.Errorf("adding the icon and version information to %s: %v", filepath.Base(path), err)
	}
	// Written beside the program and renamed over it, so an interrupted
	// build does not leave half of one.
	tmp, err := os.CreateTemp(filepath.Dir(path), ".tela-res-*")
	if err != nil {
		return err
	}
	defer os.Remove(tmp.Name())
	if _, err := tmp.Write(patched); err != nil {
		tmp.Close()
		return err
	}
	if err := tmp.Close(); err != nil {
		return err
	}
	if err := os.Chmod(tmp.Name(), 0o755); err != nil {
		return err
	}
	return os.Rename(tmp.Name(), path)
}
