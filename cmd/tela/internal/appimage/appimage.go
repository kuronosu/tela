// Package appimage writes AppImages (ADR-295): Tela's own runtime (the
// runtime folder, a main package built from source embedded in tela
// for each target) with the AppImage type 2 layout (package layout) and
// the payload as a squashfs image (package squashfs) appended.
//
// The runtime is built in a temporary module of its own, from the copy of
// its source embedded in the tela command, so the runtime is the one of
// the tela that packages, whatever version of tela the app's module
// requires, and a vendored module needs nothing more. It uses only the
// standard library. The build is reproducible: -trimpath, no VCS stamp,
// an empty build ID, the baseline instruction set (GOAMD64=v1,
// GOARM64=v8.0), and GOFLAGS cleared, so the same go toolchain gives the
// same bytes.
package appimage

import (
	"bytes"
	"crypto/sha256"
	"embed"
	"fmt"
	"io"
	"io/fs"
	"os"
	"os/exec"
	"path"
	"path/filepath"
	"strings"
	"time"

	"github.com/kuronosu/tela/cmd/tela/internal/appimage/layout"
	"github.com/kuronosu/tela/cmd/tela/internal/appimage/squashfs"
)

// sources is the runtime's source and the packages it imports.
//
//go:embed runtime/*.go squashfs/*.go layout/*.go
var sources embed.FS

// GoDirective is the go line of the runtime's module: the tela module's.
const GoDirective = "1.27"

// modulePath is the runtime module's path, the tela module's, so the
// embedded packages keep their import paths.
const modulePath = "github.com/kuronosu/tela"

// srcDir is where the embedded files go in that module.
const srcDir = "cmd/tela/internal/appimage"

// Arches are the GOARCH values the runtime is built for: the 64-bit
// little-endian Linux targets.
var Arches = []string{"amd64", "arm64", "riscv64", "loong64", "ppc64le"}

// BuildRuntime builds the runtime for linux/goarch with the go command at
// goBin and the environment env (os.Environ() and the like; the target
// variables are set over it). version is tela's, printed by
// --appimage-version.
func BuildRuntime(goBin, goarch, version string, env []string, stderr io.Writer) ([]byte, error) {
	supported := false
	for _, a := range Arches {
		supported = supported || a == goarch
	}
	if !supported {
		return nil, fmt.Errorf("an AppImage needs a 64-bit little-endian target (%s), not linux/%s", strings.Join(Arches, ", "), goarch)
	}
	dir, err := os.MkdirTemp("", "tela-appimage-runtime-")
	if err != nil {
		return nil, err
	}
	defer os.RemoveAll(dir)
	if err := os.WriteFile(filepath.Join(dir, "go.mod"), []byte("module "+modulePath+"\n\ngo "+GoDirective+"\n"), 0o644); err != nil {
		return nil, err
	}
	err = fs.WalkDir(sources, ".", func(p string, d fs.DirEntry, err error) error {
		if err != nil || d.IsDir() || strings.HasSuffix(p, "_test.go") {
			return err
		}
		data, err := sources.ReadFile(p)
		if err != nil {
			return err
		}
		dst := filepath.Join(dir, filepath.FromSlash(path.Join(srcDir, p)))
		if err := os.MkdirAll(filepath.Dir(dst), 0o755); err != nil {
			return err
		}
		return os.WriteFile(dst, data, 0o644)
	})
	if err != nil {
		return nil, err
	}
	version = strings.Map(func(r rune) rune {
		if r <= ' ' || r == '\'' || r == '"' || r == '\\' || r > '~' {
			return '_'
		}
		return r
	}, version)
	out := filepath.Join(dir, "runtime")
	cmd := exec.Command(goBin, "build", "-trimpath", "-buildvcs=false",
		"-ldflags=-s -w -buildid= -X main.version="+version,
		"-o", out, "./"+srcDir+"/runtime")
	cmd.Dir = dir
	cmd.Env = append(append([]string(nil), env...),
		"CGO_ENABLED=0", "GOOS=linux", "GOARCH="+goarch, "GOFLAGS=", "GOWORK=off",
		"GO111MODULE=on", "GOAMD64=v1", "GOARM64=v8.0", "GORISCV64=rva20u64", "GOPPC64=power8")
	var msg bytes.Buffer
	cmd.Stdout, cmd.Stderr = &msg, &msg
	if err := cmd.Run(); err != nil {
		stderr.Write(msg.Bytes())
		return nil, fmt.Errorf("building the AppImage runtime for linux/%s: %v", goarch, err)
	}
	return os.ReadFile(out)
}

// Spec is what goes into an AppImage.
type Spec struct {
	// AppID and Version name the folder the runtime extracts into.
	AppID, Version string
	// Entries are the payload: AppRun at least.
	Entries []squashfs.Entry
	// ModTime is the time of every file.
	ModTime time.Time
	// UpdateInfo is the update information (AppImageUpdate's), empty for
	// none.
	UpdateInfo string
}

// Assemble returns the AppImage of spec with the runtime rt, an ELF file
// from BuildRuntime.
func Assemble(rt []byte, spec Spec) ([]byte, error) {
	if len(spec.UpdateInfo) >= layout.UpdateInfoSize {
		return nil, fmt.Errorf("the update information is longer than %d bytes", layout.UpdateInfoSize-1)
	}
	img, err := squashfs.Build(spec.Entries, squashfs.Options{ModTime: spec.ModTime})
	if err != nil {
		return nil, err
	}
	info := layout.Info{Size: uint64(len(img)), SHA256: sha256.Sum256(img), AppID: spec.AppID, Version: spec.Version}
	placeholder, err := info.Marshal()
	if err != nil {
		return nil, err
	}
	upd := make([]byte, layout.UpdateInfoSize)
	copy(upd, spec.UpdateInfo)
	out, offs, err := layout.AddSections(rt, []layout.Section{
		{Name: layout.UpdateInfo, Data: upd, Align: 1},
		{Name: layout.Signature, Data: make([]byte, layout.SignatureSize), Align: 1},
		{Name: layout.SigKey, Data: make([]byte, layout.SigKeySize), Align: 1},
		{Name: layout.Payload, Data: placeholder, Align: 8},
	})
	if err != nil {
		return nil, fmt.Errorf("the AppImage runtime: %v", err)
	}
	lf, err := layout.Read(bytes.NewReader(out), int64(len(out)))
	if err != nil {
		return nil, err
	}
	info.Offset = lf.End
	b, err := info.Marshal()
	if err != nil {
		return nil, err
	}
	copy(out[offs[3]:], b)
	if uint64(len(out)) != lf.End {
		return nil, fmt.Errorf("the AppImage runtime ends at %d but is %d bytes", lf.End, len(out))
	}
	return append(out, img...), nil
}
