package pkgfmt

import (
	"archive/zip"
	"bytes"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"slices"
	"time"

	"github.com/kuronosu/tela/cmd/tela/internal/appbundle"
)

// MacArches are the architectures of the macOS formats: the two of macOS,
// and universal, one program holding both (appbundle.Universal).
var MacArches = []string{"amd64", "arm64", "universal"}

func checkMacArch(t Target) error {
	if !slices.Contains(MacArches, t.Arch) {
		return fmt.Errorf("macOS runs amd64 and arm64 programs; give -arch amd64, arm64 or universal (both in one program), not %s", t.Arch)
	}
	return nil
}

// appFormat writes <package>-<version>-darwin-<arch>.zip: the bundle
// <Name>.app at the top level of the zip, the layout package update's
// bundle swap reads (update.Apply), and the one Finder's Archive Utility
// and ditto -x -k unpack:
//
//	<Name>.app/Contents/Info.plist
//	<Name>.app/Contents/MacOS/<Name>
//	<Name>.app/Contents/Resources/<package>.icns   (with an icon)
//	<Name>.app/Contents/_CodeSignature/CodeResources   (signed)
//
// It also writes <package>.entitlements next to the zip, the entitlements
// codesign signed with (Entitlements). With -sign on macOS the program and
// the bundle are signed, the bundle is notarized (a zip of it is uploaded)
// and stapled, and the zip is made after the stapling.
type appFormat struct{}

func (appFormat) Name() string                  { return "app" }
func (appFormat) OS() string                    { return "darwin" }
func (appFormat) Check(app App, t Target) error { return checkMacArch(t) }

func (appFormat) Write(job Job) ([]Output, error) {
	ent, err := writeEntitlements(job)
	if err != nil {
		return nil, err
	}
	bundle, signed, err := stageBundle(job)
	if err != nil {
		return []Output{ent}, err
	}
	if signed {
		// The service takes a zip of the bundle, and the ticket is stapled
		// to the bundle itself.
		up := filepath.Join(job.Work, "notarize.zip")
		if err := writeZipFile(up, bundle, job.Time); err != nil {
			return []Output{ent}, err
		}
		if _, err := notarize(job, up, bundle); err != nil {
			return []Output{ent}, fmt.Errorf("notarizing %s: %v", filepath.Base(bundle), err)
		}
	}
	var buf bytes.Buffer
	if err := ZipBundle(&buf, bundle, job.Time); err != nil {
		return []Output{ent}, err
	}
	p, err := writeOut(job.OutDir, FileName(job.App, job.Target, ".zip"), buf.Bytes(), 0o644)
	if err != nil {
		return []Output{ent}, err
	}
	out, err := sign(job, p)
	out.Signed = out.Signed || signed
	return []Output{ent, out}, err
}

// dmgFormat writes <package>-<version>-darwin-<arch>.dmg with hdiutil, on
// macOS only: a compressed (UDZO) disk image named after the app, holding
// the bundle and a link to /Applications to drag it to. hdiutil's images
// are not byte for byte reproducible. With -sign, the bundle is signed,
// then the image, which is notarized and stapled as a whole.
type dmgFormat struct{}

func (dmgFormat) Name() string { return "dmg" }
func (dmgFormat) OS() string   { return "darwin" }

// hostOS is runtime.GOOS; tests replace it.
var hostOS = runtime.GOOS

func (dmgFormat) Check(app App, t Target) error {
	if hostOS != "darwin" {
		return errors.New("a dmg is made by macOS's hdiutil, so it needs a Mac; the app format (-format app, a zip of the bundle) is written on any system")
	}
	if _, err := exec.LookPath("hdiutil"); err != nil {
		return fmt.Errorf("a dmg is made by hdiutil, which was not found: %v", err)
	}
	return checkMacArch(t)
}

func (dmgFormat) Write(job Job) ([]Output, error) {
	ent, err := writeEntitlements(job)
	if err != nil {
		return nil, err
	}
	bundle, _, err := stageBundle(job)
	if err != nil {
		return []Output{ent}, err
	}
	src := filepath.Join(job.Work, "dmg")
	if err := os.MkdirAll(src, 0o755); err != nil {
		return []Output{ent}, err
	}
	if err := os.Rename(bundle, filepath.Join(src, filepath.Base(bundle))); err != nil {
		return []Output{ent}, err
	}
	if err := os.Symlink("/Applications", filepath.Join(src, "Applications")); err != nil {
		return []Output{ent}, err
	}
	if err := os.MkdirAll(job.OutDir, 0o755); err != nil {
		return []Output{ent}, err
	}
	tmp := filepath.Join(job.Work, "image.dmg")
	cmd := exec.Command("hdiutil", "create", "-volname", job.App.Name, "-srcfolder", src, "-ov", "-format", "UDZO", tmp)
	if out, err := cmd.CombinedOutput(); err != nil {
		return []Output{ent}, fmt.Errorf("hdiutil create: %v\n%s", err, out)
	}
	p := filepath.Join(job.OutDir, FileName(job.App, job.Target, ".dmg"))
	if err := os.Rename(tmp, p); err != nil {
		return []Output{ent}, err
	}
	out, err := sign(job, p)
	if err != nil {
		return []Output{ent, out}, err
	}
	if out.Signed {
		if _, err := notarize(job, p, p); err != nil {
			return []Output{ent, out}, fmt.Errorf("notarizing %s: %v", filepath.Base(p), err)
		}
	}
	return []Output{ent, out}, nil
}

// writeEntitlements writes <package>.entitlements into the output folder.
func writeEntitlements(job Job) (Output, error) {
	p, err := writeOut(job.OutDir, filepath.Base(EntitlementsPath(job.OutDir, job.App)), Entitlements(job.App), 0o644)
	return Output{Path: p}, err
}

// EntitlementsPath is where the formats write the entitlements of app.
func EntitlementsPath(outDir string, app App) string {
	return filepath.Join(outDir, app.Package+".entitlements")
}

// stageBundle writes the bundle of job into job.Work and signs it: the
// program (SignProgram) before it goes in, then the bundle (SignBundle).
// signed says the bundle was signed. Unsigned, the program is the linker's
// file unchanged, so the linker's ad-hoc signature of an arm64 program
// (each slice's, in a universal one) stays valid.
func stageBundle(job Job) (string, bool, error) {
	prog := job.Program
	if job.Sign != nil {
		// A copy: the program is shared by the formats of the target.
		prog = filepath.Join(job.Work, appbundle.FileName(job.App.Name))
		b, err := os.ReadFile(job.Program)
		if err != nil {
			return "", false, err
		}
		if err := os.WriteFile(prog, b, 0o755); err != nil {
			return "", false, err
		}
		if _, err := job.Sign.Sign(prog, SignProgram, job.Target); err != nil {
			return "", false, fmt.Errorf("signing the program: %v", err)
		}
	}
	icons := filepath.Join(job.Work, "icons")
	if err := os.MkdirAll(icons, 0o755); err != nil {
		return "", false, err
	}
	dir := filepath.Join(job.Work, appbundle.FileName(job.App.Name)+".app")
	if _, err := WriteBundle(dir, job.App, prog, icons); err != nil {
		return "", false, err
	}
	if job.Sign == nil {
		return dir, false, nil
	}
	signed, err := job.Sign.Sign(dir, SignBundle, job.Target)
	if err != nil {
		return "", false, fmt.Errorf("signing %s: %v", filepath.Base(dir), err)
	}
	return dir, signed, nil
}

func writeZipFile(path, bundle string, t time.Time) error {
	f, err := os.Create(path)
	if err != nil {
		return err
	}
	if err := ZipBundle(f, bundle, t); err != nil {
		f.Close()
		return err
	}
	return f.Close()
}

// ZipBundle writes a zip of the folder bundle (a <Name>.app) to w, with the
// folder at its top level, as ditto -c -k --keepParent makes it, the form
// Apple documents for shipping a bundle in a zip (and for uploading one to
// the notary service):
//
//   - Every entry records the Unix host (version made by 3) and its mode in
//     the high 16 bits of the external attributes: 0755 folders (with the
//     MS-DOS folder bit), 0644 files, 0755 files with an executable bit,
//     and symbolic links with S_IFLNK and the link's target as their data.
//     That is what Archive Utility, ditto -x -k and Go's archive/zip
//     (update's unpacking) read to restore the executable bits and the
//     links; a zip without them (made on Windows, or by a tool that writes
//     the MS-DOS host) unpacks a bundle whose program does not run. The
//     low byte of the version is the zip version, 2.0 here (deflate);
//     readers look only at the host byte for the mode.
//   - Folders come before their contents (the entries are in lexical
//     order of a walk), each with its own entry, so empty folders are kept.
//   - No AppleDouble (__MACOSX/._*) entries: ditto adds them for extended
//     attributes, which a bundle does not need; its signature lives in
//     files (Contents/_CodeSignature, the stapled ticket in
//     Contents/CodeResources).
//   - Files and links are deflated, their sizes and CRC-32 in a data
//     descriptor after the data (and in the central directory), as ditto
//     streams them; folders are stored, without one. Links are deflated
//     too, so that no stored entry has a data descriptor, which streaming
//     readers cannot delimit.
//   - Deterministic: every time is t (MS-DOS fields and the extended
//     timestamp field), and nothing else of the machine is recorded (no
//     owner, no access time).
func ZipBundle(w io.Writer, bundle string, t time.Time) error {
	zw := zip.NewWriter(w)
	parent := filepath.Dir(bundle)
	err := filepath.WalkDir(bundle, func(p string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		rel, err := filepath.Rel(parent, p)
		if err != nil {
			return err
		}
		name := filepath.ToSlash(rel)
		var mode fs.FileMode
		var data []byte
		switch {
		case d.IsDir():
			name += "/"
			mode = fs.ModeDir | 0o755
		case d.Type()&fs.ModeSymlink != 0:
			target, err := os.Readlink(p)
			if err != nil {
				return err
			}
			mode, data = fs.ModeSymlink|0o755, []byte(filepath.ToSlash(target))
		case d.Type().IsRegular():
			info, err := d.Info()
			if err != nil {
				return err
			}
			mode = 0o644
			if info.Mode()&0o111 != 0 {
				mode = 0o755
			}
			if data, err = os.ReadFile(p); err != nil {
				return err
			}
		default:
			return fmt.Errorf("%s is not a file, folder or link, which a zip cannot hold", p)
		}
		return addZipEntry(zw, name, mode, data, t)
	})
	if err != nil {
		return err
	}
	return zw.Close()
}

// addZipEntry writes one entry: data deflated (nothing for a folder).
func addZipEntry(zw *zip.Writer, name string, mode fs.FileMode, data []byte, t time.Time) error {
	// UTC, so the MS-DOS fields are the same on every machine; the writer
	// adds the extended timestamp (0x5455), and marks a UTF-8 name.
	h := &zip.FileHeader{Name: name, Method: zip.Deflate, Modified: t.UTC()}
	if mode.IsDir() {
		h.Method = zip.Store
	}
	h.SetMode(mode) // the Unix host, and the mode in the external attributes
	w, err := zw.CreateHeader(h)
	if err != nil {
		return err
	}
	_, err = w.Write(data)
	return err
}
