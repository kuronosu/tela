package pkgfmt

import (
	"fmt"
	"io/fs"
	"slices"

	"github.com/kuronosu/tela/cmd/tela/internal/appimage"
	"github.com/kuronosu/tela/cmd/tela/internal/appimage/squashfs"
)

// appImageFormat writes <package>-<version>-linux-<arch>.AppImage: Tela's
// runtime and the payload (package appimage). The payload:
//
//	AppRun                       -> usr/bin/<package>
//	<ID>.desktop                 the desktop entry (Exec=<package>)
//	<ID>.png, .DirIcon           the 256 px icon (.DirIcon links to it)
//	usr/bin/<package>            the program
//	usr/share/applications/<ID>.desktop
//	usr/share/metainfo/<ID>.metainfo.xml
//	usr/share/mime/packages/<ID>.xml   (with file types)
//	usr/share/icons/hicolor/...
//
// It updates itself (package update swaps the file at $APPIMAGE).
type appImageFormat struct{}

func (appImageFormat) Name() string { return "appimage" }
func (appImageFormat) OS() string   { return "linux" }

func (appImageFormat) Check(app App, t Target) error {
	if !slices.Contains(appimage.Arches, t.Arch) {
		return fmt.Errorf("an AppImage needs a 64-bit little-endian target (linux/amd64, arm64, riscv64, loong64 or ppc64le), not %s", t)
	}
	return nil
}

func (f appImageFormat) Write(job Job) ([]Output, error) {
	app := job.App
	prog, err := readProgram(job)
	if err != nil {
		return nil, err
	}
	usr, err := usrFiles(app, prog, job.Time)
	if err != nil {
		return nil, err
	}
	entries := []squashfs.Entry{
		{Path: ".", Mode: fs.ModeDir | 0o755},
		{Path: "AppRun", Mode: fs.ModeSymlink | 0o777, Target: "usr/bin/" + app.Package},
		{Path: app.ID + ".desktop", Mode: 0o644, Data: DesktopEntry(app, app.Package)},
	}
	if app.Icon != nil {
		png, err := app.Icon.PNG(256)
		if err != nil {
			return nil, err
		}
		entries = append(entries,
			squashfs.Entry{Path: app.ID + ".png", Mode: 0o644, Data: png},
			squashfs.Entry{Path: ".DirIcon", Mode: fs.ModeSymlink | 0o777, Target: app.ID + ".png"})
	}
	for _, u := range usr {
		entries = append(entries, squashfs.Entry{Path: u.path, Mode: u.mode, Data: u.data, Target: u.target})
	}
	rt, err := appimage.BuildRuntime(job.GoBin, job.Target.Arch, job.Tool, job.GoEnv, job.Log)
	if err != nil {
		return nil, err
	}
	b, err := appimage.Assemble(rt, appimage.Spec{AppID: app.ID, Version: app.Version, Entries: entries, ModTime: job.Time})
	if err != nil {
		return nil, err
	}
	p, err := writeOut(job.OutDir, FileName(app, job.Target, ".AppImage"), b, 0o755)
	if err != nil {
		return nil, err
	}
	out, err := sign(job, p)
	return []Output{out}, err
}
