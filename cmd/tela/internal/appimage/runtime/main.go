//go:build linux

// Command runtime is Tela's AppImage runtime (ADR-295): the program at the
// start of an AppImage that tela package writes. The AppImage project's
// runtime mounts the payload with FUSE (and links libfuse3); this one
// extracts it instead, once per version, into the user's cache:
//
//	$XDG_CACHE_HOME/tela-appimage/<app ID>/<version>-<12 hex digits of its SHA-256>/
//
// and runs its AppRun from there with exec, so the app's process is the
// one the user started, with APPIMAGE (the AppImage's absolute path),
// APPDIR (the extracted folder), ARGV0 (the name it was started by) and
// OWD (the folder it was started in) set and the arguments forwarded.
//
// Concurrency: every start takes an exclusive flock on <app ID>/.lock
// while it checks, extracts and cleans, so a second instance started at
// the same time waits and then uses the first one's result. Extraction
// goes into a .tmp- sibling that is renamed into place when complete; a
// start finds the leftovers of a crashed one under the lock and removes
// them.
//
// Integrity: the squashfs image's SHA-256, recorded by tela package in
// the ELF section .tela_payload, is checked before extracting; a damaged
// AppImage is refused with a message and exit status 1. The extracted
// tree gets a sealed marker, .tela-extracted, written last: the hash, and
// each entry's type, mode, size, time and SHA-256 (a link's target). A
// later start checks the marker's seal and every entry's type, mode and
// size, and hashes a file only when its time changed; anything missing or
// different makes it extract again, so the warm start costs a few
// stat calls.
//
// Cleanup: each running instance holds a shared flock on its version's
// folder, through a descriptor its exec inherits, for the life of the
// app's process. After its own version is ready, a start removes the
// other folders of the app whose lock it can take exclusively without
// waiting, so a version that is running is never deleted under it.
// Descriptors of that kind inherited from a previous instance (an update
// that restarts the app by exec) are closed first.
//
// Options, as the first argument: --appimage-extract [pattern] (into
// ./squashfs-root), --appimage-offset, --appimage-help,
// --appimage-version, --appimage-updateinformation,
// --appimage-signature and --appimage-extract-and-run (what a plain start
// does). --appimage-mount is not available: there is no FUSE.
//
// It uses only the standard library and builds with CGO_ENABLED=0; tela
// package builds it from its embedded source for each target
// architecture.
package main

import (
	"bytes"
	"crypto/sha256"
	"errors"
	"fmt"
	"io"
	"os"
	"path"
	"path/filepath"
	"strings"
	"syscall"

	"github.com/kuronosu/tela/cmd/tela/internal/appimage/layout"
	"github.com/kuronosu/tela/cmd/tela/internal/appimage/squashfs"
)

// version is tela's version, set by tela package with -X.
var version = "devel"

func main() {
	os.Exit(run(os.Args))
}

// appImage is the running AppImage.
type appImage struct {
	path    string // absolute, symbolic links resolved
	f       *os.File
	size    int64
	info    layout.Info
	end     uint64
	updInfo []byte
	sig     []byte
}

func fail(format string, a ...any) int {
	fmt.Fprintf(os.Stderr, "AppImage: "+format+"\n", a...)
	return 1
}

func run(args []string) int {
	img, err := openSelf()
	if err != nil {
		return fail("%v", err)
	}
	defer img.f.Close()
	if len(args) > 1 && strings.HasPrefix(args[1], "--appimage-") {
		switch args[1] {
		case "--appimage-help":
			fmt.Print(helpText)
			return 0
		case "--appimage-version":
			fmt.Printf("Tela AppImage runtime %s\n", version)
			return 0
		case "--appimage-offset":
			fmt.Println(img.end)
			return 0
		case "--appimage-updateinformation":
			fmt.Println(cstring(img.updInfo))
			return 0
		case "--appimage-signature":
			fmt.Println(cstring(img.sig))
			return 0
		case "--appimage-extract":
			pattern := ""
			if len(args) > 2 {
				pattern = args[2]
			}
			if err := extractHere(img, pattern); err != nil {
				return fail("%v", err)
			}
			return 0
		case "--appimage-extract-and-run":
			args = append(args[:1:1], args[2:]...)
		case "--appimage-mount":
			return fail("--appimage-mount is not available: Tela's runtime does not use FUSE; use --appimage-extract")
		default:
			return fail("unknown option %s; see --appimage-help", args[1])
		}
	}
	dir, err := prepare(img)
	if err != nil {
		return fail("%v", err)
	}
	return execApp(img, dir, args)
}

const helpText = `This is an AppImage made by tela package; it runs on Tela's own runtime,
which extracts the app into the user's cache folder once per version
(no FUSE) and runs it from there.

Options (as the first argument):

  --appimage-extract [pattern]   extract the content into ./squashfs-root
                                 (only the paths that match pattern, as
                                 path.Match does, and what is under them)
  --appimage-extract-and-run     run the app (what a start without options does)
  --appimage-offset              print where the squashfs image starts
  --appimage-updateinformation   print the update information
  --appimage-signature           print the embedded signature (empty)
  --appimage-version             print the runtime's version
  --appimage-help                print this help

The cache folder is $XDG_CACHE_HOME/tela-appimage/<app ID> (else
~/.cache/tela-appimage/<app ID>); versions not in use are removed when
another version starts.
`

// openSelf opens the running AppImage and reads its layout.
func openSelf() (*appImage, error) {
	exe, err := os.Executable()
	if err != nil {
		return nil, fmt.Errorf("cannot find the AppImage file: %v", err)
	}
	if r, err := filepath.EvalSymlinks(exe); err == nil {
		exe = r
	}
	f, err := os.Open(exe)
	if err != nil {
		return nil, fmt.Errorf("cannot read the AppImage file: %v", err)
	}
	st, err := f.Stat()
	if err != nil {
		f.Close()
		return nil, err
	}
	lf, err := layout.Read(f, st.Size(), layout.Payload, layout.UpdateInfo, layout.Signature)
	if err != nil {
		f.Close()
		return nil, fmt.Errorf("%s is not a readable AppImage: %v", exe, err)
	}
	info, err := layout.UnmarshalInfo(lf.Sections[layout.Payload])
	if err != nil {
		f.Close()
		return nil, fmt.Errorf("%s is not an AppImage made by tela package: %v", exe, err)
	}
	img := &appImage{path: exe, f: f, size: st.Size(), info: info, end: lf.End,
		updInfo: lf.Sections[layout.UpdateInfo], sig: lf.Sections[layout.Signature]}
	if info.Offset != lf.End || info.Size > uint64(st.Size()) || info.Offset > uint64(st.Size())-info.Size {
		f.Close()
		return nil, fmt.Errorf("%s is damaged or incomplete: it is %d bytes, and its content should take %d bytes from offset %d; download it again", exe, st.Size(), info.Size, info.Offset)
	}
	if !validName(info.AppID) {
		f.Close()
		return nil, fmt.Errorf("%s has an invalid app ID %q", exe, info.AppID)
	}
	return img, nil
}

// validName reports whether s can name a folder of the cache.
func validName(s string) bool {
	return s != "" && len(s) <= 255 && !strings.HasPrefix(s, ".") && !strings.ContainsAny(s, "/\\\x00")
}

func cstring(b []byte) string {
	if i := bytes.IndexByte(b, 0); i >= 0 {
		b = b[:i]
	}
	return string(b)
}

// image returns a reader of the squashfs image.
func (img *appImage) image() *io.SectionReader {
	return io.NewSectionReader(img.f, int64(img.info.Offset), int64(img.info.Size))
}

// errDamaged is the error of an image whose hash does not match.
var errDamaged = errors.New("the AppImage is damaged: its content does not match the checksum recorded when it was made (an incomplete download, or a changed file); download it again")

// verify checks the image against the recorded SHA-256.
func (img *appImage) verify() error {
	h := sha256.New()
	if _, err := io.CopyBuffer(h, img.image(), make([]byte, 1<<20)); err != nil {
		return fmt.Errorf("reading the AppImage: %v", err)
	}
	if !bytes.Equal(h.Sum(nil), img.info.SHA256[:]) {
		return errDamaged
	}
	return nil
}

// reader opens the squashfs image.
func (img *appImage) reader() (*squashfs.Reader, error) {
	rd, err := squashfs.NewReader(img.image(), int64(img.info.Size))
	if err != nil {
		return nil, fmt.Errorf("the AppImage's content: %v", err)
	}
	return rd, nil
}

// execApp runs dir/AppRun in place of this process.
func execApp(img *appImage, dir string, args []string) int {
	apprun := filepath.Join(dir, "AppRun")
	owd, _ := os.Getwd()
	env := os.Environ()
	set := func(k, v string) {
		for i, kv := range env {
			if strings.HasPrefix(kv, k+"=") {
				env[i] = k + "=" + v
				return
			}
		}
		env = append(env, k+"="+v)
	}
	set("APPIMAGE", img.path)
	set("APPDIR", dir)
	set("ARGV0", args[0])
	set("OWD", owd)
	argv := append([]string{apprun}, args[1:]...)
	img.f.Close()
	err := syscall.Exec(apprun, argv, env)
	if errors.Is(err, syscall.EACCES) {
		return fail("cannot run %s: %v; the cache folder may be on a file system mounted noexec: set XDG_CACHE_HOME to another folder, or use --appimage-extract", apprun, err)
	}
	return fail("cannot run %s: %v", apprun, err)
}

// extractHere implements --appimage-extract: the entries that match
// pattern (all without one), and what is under them, into ./squashfs-root,
// replacing what is there.
func extractHere(img *appImage, pattern string) error {
	if pattern != "" {
		if _, err := path.Match(pattern, ""); err != nil {
			return fmt.Errorf("the pattern %q: %v", pattern, err)
		}
	}
	if err := img.verify(); err != nil {
		return err
	}
	rd, err := img.reader()
	if err != nil {
		return err
	}
	const out = "squashfs-root"
	if err := os.MkdirAll(out, 0o755); err != nil {
		return err
	}
	matched := map[string]bool{}
	var dirs []squashfs.File
	err = rd.Walk(func(f squashfs.File) error {
		parent := path.Dir(f.Path)
		ok := pattern == "" || matched[parent]
		if !ok {
			ok, _ = path.Match(pattern, f.Path)
		}
		if !ok {
			return nil
		}
		matched[f.Path] = true
		p := filepath.Join(out, filepath.FromSlash(f.Path))
		if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
			return err
		}
		fmt.Println(p)
		switch {
		case f.Mode.IsDir():
			if err := os.Mkdir(p, 0o700); err != nil && !errors.Is(err, os.ErrExist) {
				return err
			}
			os.Chmod(p, 0o700)
			dirs = append(dirs, f)
		case f.Mode&os.ModeSymlink != 0:
			os.Remove(p)
			return os.Symlink(f.Target, p)
		default:
			os.Remove(p)
			w, err := os.OpenFile(p, os.O_WRONLY|os.O_CREATE|os.O_EXCL|syscall.O_NOFOLLOW, 0o600)
			if err != nil {
				return err
			}
			if _, err := rd.WriteTo(w, f); err != nil {
				w.Close()
				return err
			}
			if err := w.Chmod(f.Mode.Perm()); err != nil {
				w.Close()
				return err
			}
			return w.Close()
		}
		return nil
	})
	if err != nil {
		return err
	}
	for i := len(dirs) - 1; i >= 0; i-- {
		os.Chmod(filepath.Join(out, filepath.FromSlash(dirs[i].Path)), dirs[i].Mode.Perm())
	}
	return nil
}
