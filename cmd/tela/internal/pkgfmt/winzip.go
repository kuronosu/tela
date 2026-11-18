package pkgfmt

import (
	"archive/zip"
	"bytes"
	"encoding/json"
	"fmt"
	"io/fs"
	"sort"
	"time"

	"github.com/kuronosu/tela/update"
)

// zipFormat writes <package>-<version>-windows-<arch>.zip, the portable
// program that updates itself (package update, ADR-296):
//
//	<package>/<package>.exe        the program
//	<package>/tela-install.json    the update.Marker of the folder
//
// The marker (update.MarkerName) makes the folder an install: update
// finds it next to the executable, and replaces only the entries its
// Files lists, so files the user puts in the folder stay. The executable
// is the one tela build makes (GUI subsystem, icon and version
// information), signed first when a signer signs programs.
type zipFormat struct{}

func (zipFormat) Name() string { return "zip" }
func (zipFormat) OS() string   { return "windows" }

func (zipFormat) Check(app App, t Target) error { return nil }

// InstallMarker returns the update.Marker of the zip folder of app, whose
// program is exe, in the JSON tela package writes.
func InstallMarker(app App, exe string) ([]byte, error) {
	files := []string{exe, update.MarkerName}
	sort.Strings(files)
	m := update.Marker{Schema: 1, Format: "zip", AppID: app.ID, Version: app.Version, Exe: exe, Files: files}
	b, err := json.MarshalIndent(m, "", "  ")
	if err != nil {
		return nil, err
	}
	return append(b, '\n'), nil
}

func (zipFormat) Write(job Job) ([]Output, error) {
	app := job.App
	prog, err := readProgram(job)
	if err != nil {
		return nil, err
	}
	exe := app.Package + ".exe"
	marker, err := InstallMarker(app, exe)
	if err != nil {
		return nil, err
	}
	dir := app.Package + "/"
	b, err := zipFiles(job.Time, []file{
		{path: dir, mode: fs.ModeDir | 0o755},
		{path: dir + exe, mode: 0o755, data: prog},
		{path: dir + update.MarkerName, mode: 0o644, data: marker},
	})
	if err != nil {
		return nil, err
	}
	p, err := writeOut(job.OutDir, FileName(app, job.Target, ".zip"), b, 0o644)
	if err != nil {
		return nil, err
	}
	out, err := sign(job, p)
	if job.ProgramSigned && !out.Signed {
		out.Note = "the program in it is signed"
	}
	return []Output{out}, err
}

// zipFiles returns a zip of files in their order (folders end in '/'),
// deflated, every entry with the time when and its Unix mode, so the same
// files give the same bytes.
func zipFiles(when time.Time, files []file) ([]byte, error) {
	var buf bytes.Buffer
	zw := zip.NewWriter(&buf)
	for _, f := range files {
		h := &zip.FileHeader{Name: f.path, Method: zip.Deflate, Modified: when.UTC()}
		h.SetMode(f.mode)
		if f.mode.IsDir() {
			h.Method = zip.Store
		}
		w, err := zw.CreateHeader(h)
		if err != nil {
			return nil, err
		}
		if _, err := w.Write(f.data); err != nil {
			return nil, err
		}
	}
	if err := zw.Close(); err != nil {
		return nil, fmt.Errorf("writing the zip: %v", err)
	}
	return buf.Bytes(), nil
}
