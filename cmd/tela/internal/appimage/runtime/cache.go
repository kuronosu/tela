//go:build linux

package main

import (
	"bytes"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"

	"github.com/kuronosu/tela/cmd/tela/internal/appimage/squashfs"
)

// The names in an app's cache folder.
const (
	lockName   = ".lock"
	tmpPrefix  = ".tmp-"
	oldPrefix  = ".old-"
	markerName = ".tela-extracted"
)

// cacheRoot is $XDG_CACHE_HOME (when absolute, as the XDG specification
// requires), else ~/.cache.
func cacheRoot() (string, error) {
	if d := os.Getenv("XDG_CACHE_HOME"); filepath.IsAbs(d) {
		return d, nil
	}
	if h := os.Getenv("HOME"); filepath.IsAbs(h) {
		return filepath.Join(h, ".cache"), nil
	}
	return "", errors.New("neither XDG_CACHE_HOME nor HOME is set, so there is no cache folder to extract the app into; set one, or use --appimage-extract")
}

// versionDir is the name of the image's folder: its version and the
// first 12 hex digits of its hash.
func versionDir(img *appImage) string {
	v := strings.Map(func(r rune) rune {
		if r >= 'a' && r <= 'z' || r >= 'A' && r <= 'Z' || r >= '0' && r <= '9' || strings.ContainsRune(".+~_-", r) {
			return r
		}
		return '_'
	}, img.info.Version)
	if v == "" || v[0] == '.' {
		v = "v" + v
	}
	return v + "-" + hex.EncodeToString(img.info.SHA256[:6])
}

// prepare makes the image's folder ready, takes the shared lock that its
// exec inherits, removes the versions not in use, and returns the folder.
func prepare(img *appImage) (string, error) {
	root, err := cacheRoot()
	if err != nil {
		return "", err
	}
	appRoot := filepath.Join(root, "tela-appimage", img.info.AppID)
	if err := os.MkdirAll(appRoot, 0o700); err != nil {
		return "", fmt.Errorf("cannot create the cache folder %s: %v; set XDG_CACHE_HOME to a folder you can write, or use --appimage-extract", appRoot, err)
	}
	// The real path: APPDIR is compared with the program's resolved path
	// (update's detection), and /proc/self/fd shows real paths.
	if r, err := filepath.EvalSymlinks(appRoot); err == nil {
		appRoot = r
	}
	closeInherited(appRoot)

	lock, err := os.OpenFile(filepath.Join(appRoot, lockName), os.O_RDWR|os.O_CREATE, 0o600)
	if err != nil {
		return "", fmt.Errorf("cannot open the cache's lock: %v", err)
	}
	defer lock.Close() // releases the lock; it is close-on-exec
	if err := flock(int(lock.Fd()), syscall.LOCK_EX); err != nil {
		return "", fmt.Errorf("cannot lock the cache folder %s: %v", appRoot, err)
	}

	// Under the lock nobody else extracts: temporary folders are what a
	// crashed start left.
	if ents, err := os.ReadDir(appRoot); err == nil {
		for _, e := range ents {
			if strings.HasPrefix(e.Name(), tmpPrefix) {
				removeTree(filepath.Join(appRoot, e.Name()))
			}
		}
	}
	name := versionDir(img)
	dir := filepath.Join(appRoot, name)
	if !checkTree(dir, img.info.SHA256) {
		if _, err := os.Lstat(dir); err == nil {
			// Altered or incomplete: set aside, removed by the cleanup
			// below unless an instance still runs from it.
			if err := os.Rename(dir, filepath.Join(appRoot, oldPrefix+randomName())); err != nil {
				return "", fmt.Errorf("cannot replace %s: %v", dir, err)
			}
		}
		if err := extractTo(img, appRoot, dir); err != nil {
			return "", err
		}
	}

	// The shared lock of this version, held by the app's process: the
	// descriptor is not close-on-exec.
	fd, err := syscall.Open(dir, syscall.O_RDONLY|syscall.O_DIRECTORY|syscall.O_NOFOLLOW, 0)
	if err != nil {
		return "", fmt.Errorf("cannot open %s: %v", dir, err)
	}
	if err := flock(fd, syscall.LOCK_SH); err != nil {
		syscall.Close(fd)
		return "", fmt.Errorf("cannot lock %s: %v", dir, err)
	}
	cleanup(appRoot, name)
	return dir, nil
}

// flock is flock(2), retried when a signal interrupts it (the Go
// runtime's preemption signals do).
func flock(fd, how int) error {
	for {
		err := syscall.Flock(fd, how)
		if err != syscall.EINTR {
			return err
		}
	}
}

// closeInherited closes the descriptors of version folders of this app
// that the process inherited: an app that restarts itself by exec (an
// update) passes its own version's lock on, which would keep that
// version from being removed while the new one runs.
func closeInherited(appRoot string) {
	ents, err := os.ReadDir("/proc/self/fd")
	if err != nil {
		return
	}
	for _, e := range ents {
		fd, err := strconv.Atoi(e.Name())
		if err != nil || fd < 3 {
			continue
		}
		target, err := os.Readlink("/proc/self/fd/" + e.Name())
		if err == nil && filepath.Dir(target) == appRoot {
			syscall.Close(fd)
		}
	}
}

// cleanup removes the folders of appRoot other than keep that no running
// instance holds.
func cleanup(appRoot, keep string) {
	ents, err := os.ReadDir(appRoot)
	if err != nil {
		return
	}
	for _, e := range ents {
		n := e.Name()
		if n == keep || !e.IsDir() {
			continue
		}
		p := filepath.Join(appRoot, n)
		fd, err := syscall.Open(p, syscall.O_RDONLY|syscall.O_DIRECTORY|syscall.O_NOFOLLOW|syscall.O_CLOEXEC, 0)
		if err != nil {
			continue
		}
		if syscall.Flock(fd, syscall.LOCK_EX|syscall.LOCK_NB) == nil {
			removeTree(p)
		}
		syscall.Close(fd)
	}
}

// removeTree removes p and everything in it, making its folders writable
// first (an app may ship read-only ones).
func removeTree(p string) {
	if os.RemoveAll(p) == nil {
		return
	}
	filepath.WalkDir(p, func(q string, d fs.DirEntry, err error) error {
		if err == nil && d.IsDir() {
			os.Chmod(q, 0o700)
		}
		return nil
	})
	os.RemoveAll(p)
}

func randomName() string {
	var b [8]byte
	rand.Read(b[:])
	return hex.EncodeToString(b[:])
}

// extractTo verifies the image and extracts it into dir, through a
// temporary sibling in appRoot renamed into place once complete.
func extractTo(img *appImage, appRoot, dir string) error {
	if err := img.verify(); err != nil {
		return err
	}
	rd, err := img.reader()
	if err != nil {
		return err
	}
	tmp, err := os.MkdirTemp(appRoot, tmpPrefix)
	if err != nil {
		return fmt.Errorf("cannot extract the app into %s: %v", appRoot, err)
	}
	ok := false
	defer func() {
		if !ok {
			removeTree(tmp)
		}
	}()
	m := &marker{sha: img.info.SHA256}
	type dirMode struct {
		path string
		perm fs.FileMode
	}
	var dirs []dirMode
	hasRun := false
	err = rd.Walk(func(f squashfs.File) error {
		if f.Path == markerName {
			return fmt.Errorf("the AppImage holds a file named %s, which the runtime writes", markerName)
		}
		p := filepath.Join(tmp, filepath.FromSlash(f.Path))
		e := entry{path: f.Path, perm: f.Mode.Perm()}
		switch {
		case f.Mode.IsDir():
			e.kind = 'd'
			if err := os.Mkdir(p, 0o700); err != nil {
				return err
			}
			dirs = append(dirs, dirMode{p, f.Mode.Perm()})
		case f.Mode&fs.ModeSymlink != 0:
			e.kind, e.target = 'l', f.Target
			if err := os.Symlink(f.Target, p); err != nil {
				return err
			}
		case f.Mode.IsRegular():
			e.kind, e.size, e.mtime = 'f', f.Size, f.ModTime.Unix()
			w, err := os.OpenFile(p, os.O_WRONLY|os.O_CREATE|os.O_EXCL|syscall.O_NOFOLLOW, 0o600)
			if err != nil {
				return err
			}
			h := sha256.New()
			if _, err := rd.WriteTo(io.MultiWriter(w, h), f); err != nil {
				w.Close()
				return err
			}
			copy(e.sha[:], h.Sum(nil))
			if err := w.Chmod(f.Mode.Perm()); err != nil {
				w.Close()
				return err
			}
			if err := w.Sync(); err != nil {
				w.Close()
				return err
			}
			if err := w.Close(); err != nil {
				return err
			}
			if err := os.Chtimes(p, f.ModTime, f.ModTime); err != nil {
				return err
			}
		default:
			return fmt.Errorf("%s has an unsupported type", f.Path)
		}
		if f.Path == "AppRun" {
			hasRun = true
		}
		m.entries = append(m.entries, e)
		return nil
	})
	if err != nil {
		if errors.Is(err, squashfs.ErrFormat) {
			return fmt.Errorf("the AppImage is damaged: %v", err)
		}
		return fmt.Errorf("cannot extract the app into %s: %v", appRoot, err)
	}
	if !hasRun {
		return errors.New("the AppImage has no AppRun to run")
	}
	// The seal, written last; then the folders' modes, deepest first.
	if err := writeSynced(filepath.Join(tmp, markerName), m.marshal()); err != nil {
		return fmt.Errorf("cannot extract the app into %s: %v", appRoot, err)
	}
	for i := len(dirs) - 1; i >= 0; i-- {
		syncDir(dirs[i].path)
		if err := os.Chmod(dirs[i].path, dirs[i].perm); err != nil {
			return err
		}
	}
	rootFile, err := rd.Root()
	if err != nil {
		return err
	}
	syncDir(tmp)
	if err := os.Chmod(tmp, rootFile.Mode.Perm()|0o700); err != nil {
		return err
	}
	if err := os.Rename(tmp, dir); err != nil {
		return fmt.Errorf("cannot move the extracted app into place: %v", err)
	}
	ok = true
	syncDir(appRoot)
	return nil
}

func writeSynced(p string, data []byte) error {
	f, err := os.OpenFile(p, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o444)
	if err != nil {
		return err
	}
	if _, err := f.Write(data); err != nil {
		f.Close()
		return err
	}
	if err := f.Sync(); err != nil {
		f.Close()
		return err
	}
	return f.Close()
}

func syncDir(p string) {
	if f, err := os.Open(p); err == nil {
		f.Sync()
		f.Close()
	}
}

// marker is the record of an extracted tree.
type marker struct {
	sha     [32]byte // the image's
	entries []entry
}

type entry struct {
	kind   byte // 'd', 'f' or 'l'
	path   string
	perm   fs.FileMode
	size   int64
	mtime  int64
	sha    [32]byte
	target string
}

const markerHeader = "tela-appimage-extracted 1\n"

// marshal returns the marker as text: a header, the image's hash, one
// line per entry, and a seal line with the SHA-256 of everything before
// it.
func (m *marker) marshal() []byte {
	var b bytes.Buffer
	b.WriteString(markerHeader)
	fmt.Fprintf(&b, "image %x\n", m.sha)
	for _, e := range m.entries {
		switch e.kind {
		case 'd':
			fmt.Fprintf(&b, "d %o %s\n", e.perm, strconv.Quote(e.path))
		case 'l':
			fmt.Fprintf(&b, "l %s %s\n", strconv.Quote(e.target), strconv.Quote(e.path))
		case 'f':
			fmt.Fprintf(&b, "f %o %d %d %x %s\n", e.perm, e.size, e.mtime, e.sha, strconv.Quote(e.path))
		}
	}
	seal := sha256.Sum256(b.Bytes())
	fmt.Fprintf(&b, "seal %x\n", seal)
	return b.Bytes()
}

// parseMarker reads marshal's text, checking its seal.
func parseMarker(data []byte) (*marker, error) {
	bad := errors.New("not a valid marker")
	i := bytes.LastIndex(data, []byte("seal "))
	if !bytes.HasPrefix(data, []byte(markerHeader)) || i < 0 {
		return nil, bad
	}
	seal := sha256.Sum256(data[:i])
	if string(data[i:]) != fmt.Sprintf("seal %x\n", seal) {
		return nil, bad
	}
	lines := strings.Split(string(data[len(markerHeader):i]), "\n")
	m := &marker{}
	if len(lines) < 1 {
		return nil, bad
	}
	h, ok := strings.CutPrefix(lines[0], "image ")
	if raw, err := hex.DecodeString(h); !ok || err != nil || len(raw) != 32 {
		return nil, bad
	} else {
		copy(m.sha[:], raw)
	}
	for _, l := range lines[1:] {
		if l == "" {
			continue
		}
		e := entry{kind: l[0]}
		rest := l[1:]
		field := func() string {
			rest = strings.TrimPrefix(rest, " ")
			j := strings.IndexByte(rest, ' ')
			if j < 0 {
				j = len(rest)
			}
			f := rest[:j]
			rest = rest[j:]
			return f
		}
		quoted := func() (string, bool) {
			rest = strings.TrimPrefix(rest, " ")
			q, err := strconv.QuotedPrefix(rest)
			if err != nil {
				return "", false
			}
			rest = rest[len(q):]
			s, err := strconv.Unquote(q)
			return s, err == nil
		}
		perm := func() bool {
			v, err := strconv.ParseUint(field(), 8, 32)
			e.perm = fs.FileMode(v)
			return err == nil
		}
		var okp bool
		switch e.kind {
		case 'd':
			if !perm() {
				return nil, bad
			}
			e.path, okp = quoted()
		case 'l':
			if e.target, okp = quoted(); okp {
				e.path, okp = quoted()
			}
		case 'f':
			if !perm() {
				return nil, bad
			}
			var err1, err2 error
			e.size, err1 = strconv.ParseInt(field(), 10, 64)
			e.mtime, err2 = strconv.ParseInt(field(), 10, 64)
			raw, err3 := hex.DecodeString(field())
			if err1 != nil || err2 != nil || err3 != nil || len(raw) != 32 {
				return nil, bad
			}
			copy(e.sha[:], raw)
			e.path, okp = quoted()
		}
		if !okp || rest != "" || !filepath.IsLocal(e.path) {
			return nil, bad
		}
		m.entries = append(m.entries, e)
	}
	return m, nil
}

// checkTree reports whether dir is a complete, unaltered extraction of
// the image with hash sha.
func checkTree(dir string, sha [32]byte) bool {
	data, err := readLimited(filepath.Join(dir, markerName), 64<<20)
	if err != nil {
		return false
	}
	m, err := parseMarker(data)
	if err != nil || m.sha != sha {
		return false
	}
	for _, e := range m.entries {
		p := filepath.Join(dir, filepath.FromSlash(e.path))
		st, err := os.Lstat(p)
		if err != nil {
			return false
		}
		switch e.kind {
		case 'd':
			if !st.IsDir() || st.Mode().Perm() != e.perm {
				return false
			}
		case 'l':
			if st.Mode()&fs.ModeSymlink == 0 {
				return false
			}
			if t, err := os.Readlink(p); err != nil || t != e.target {
				return false
			}
		case 'f':
			if !st.Mode().IsRegular() || st.Mode().Perm() != e.perm || st.Size() != e.size {
				return false
			}
			// Same size and a new time: written since; the content
			// decides.
			if st.ModTime().Unix() != e.mtime || st.ModTime().Nanosecond() != 0 {
				if !sameContent(p, e.sha) {
					return false
				}
			}
		default:
			return false
		}
	}
	return true
}

func readLimited(p string, limit int64) ([]byte, error) {
	f, err := os.Open(p)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	b, err := io.ReadAll(io.LimitReader(f, limit+1))
	if err != nil {
		return nil, err
	}
	if int64(len(b)) > limit {
		return nil, errors.New("too large")
	}
	return b, nil
}

func sameContent(p string, sha [32]byte) bool {
	f, err := os.Open(p)
	if err != nil {
		return false
	}
	defer f.Close()
	h := sha256.New()
	if _, err := io.Copy(h, f); err != nil {
		return false
	}
	return bytes.Equal(h.Sum(nil), sha[:])
}
