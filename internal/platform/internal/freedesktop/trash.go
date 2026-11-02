//go:build linux

package freedesktop

import (
	"context"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"syscall"
	"time"

	"github.com/godbus/dbus/v5"
	"golang.org/x/sys/unix"

	"github.com/kuronosu/tela/internal/platform"
)

// The trash (ADR-216). MoveToTrash blocks and is safe from any goroutine,
// like the secrets: it dials a connection of its own for the portal and
// closes it when done.
//
//  1. The portal: org.freedesktop.portal.Trash.TrashFile(h fd) returns 1
//     when it trashed the file and 0 when it did not. The descriptor is
//     opened as GIO's g_trash_portal_trash_file does: a regular file
//     O_RDWR|O_NOFOLLOW (the portal refuses a descriptor that is not open
//     for writing, so a read-only file goes to step 2), a folder O_PATH.
//     A symbolic link (O_NOFOLLOW cannot open it, and the portal would
//     trash its target), a device, a FIFO or a socket skip the portal.
//  2. Without the portal, or when it answers 0 or an error, the home trash
//     of the freedesktop Trash specification 1.0, written here: see
//     moveToHomeTrash. Only a file on the home trash's file system goes
//     there; anything else is ErrUnsupported, and the file stays.

const (
	ifaceTrash = "org.freedesktop.portal.Trash"
	// trashCallTimeout bounds the portal call: the first may start the
	// portal through D-Bus activation.
	trashCallTimeout = 25 * time.Second
)

// MoveToTrash moves path to the trash (see trash.go).
func (d *Desktop) MoveToTrash(path string) error {
	clean, fi, err := platform.CheckTrash(path)
	if err != nil {
		return err
	}
	if d.isClosed() {
		return errClosed
	}
	why := d.portalTrash(clean, fi)
	if why == nil {
		return nil
	}
	dir, err := homeTrashDir()
	if err == nil {
		err = moveToHomeTrash(clean, fi, dir, time.Now())
	}
	if err != nil {
		return fmt.Errorf("freedesktop: MoveToTrash(%q): the portal did not take it (%v), and the home trash did not either: %w", clean, why, err)
	}
	return nil
}

// portalTrash hands path to the portal's Trash interface. It returns nil
// when the portal trashed it, else why not.
func (d *Desktop) portalTrash(path string, fi fs.FileInfo) error {
	var flags int
	switch {
	case fi.Mode().IsRegular():
		flags = unix.O_RDWR | unix.O_CLOEXEC | unix.O_NOFOLLOW
	case fi.IsDir():
		flags = unix.O_PATH | unix.O_CLOEXEC | unix.O_RDONLY
	default:
		return fmt.Errorf("the portal takes only files and folders, not a %v", fi.Mode().Type())
	}
	conn, err := d.dial()
	if err != nil {
		return err
	}
	defer conn.Close()
	if !nameOnBus(conn, portalName) {
		return fmt.Errorf("no XDG desktop portal (%s) on the session bus", portalName)
	}
	if !conn.SupportsUnixFDs() {
		return errors.New("the D-Bus connection cannot pass file descriptors")
	}
	fd, err := unix.Open(path, flags, 0)
	if err != nil {
		return fmt.Errorf("cannot open it for the portal: %w", err)
	}
	defer unix.Close(fd)
	ctx, cancel := context.WithTimeout(context.Background(), trashCallTimeout)
	defer cancel()
	var result uint32
	if err := conn.Object(portalName, portalPath).CallWithContext(ctx, ifaceTrash+".TrashFile", 0, dbus.UnixFD(fd)).Store(&result); err != nil {
		return fmt.Errorf("the portal's TrashFile failed: %w", err)
	}
	if result != 1 {
		return fmt.Errorf("the portal's TrashFile answered %d", result)
	}
	return nil
}

// nameOnBus reports whether a service is on the bus or can be started by
// it.
func nameOnBus(conn *dbus.Conn, name string) bool {
	ctx, cancel := context.WithTimeout(context.Background(), callTimeout)
	defer cancel()
	var owned bool
	if err := conn.BusObject().CallWithContext(ctx, busIface+".NameHasOwner", 0, name).Store(&owned); err == nil && owned {
		return true
	}
	var names []string
	if err := conn.BusObject().CallWithContext(ctx, busIface+".ListActivatableNames", 0).Store(&names); err != nil {
		return false
	}
	for _, n := range names {
		if n == name {
			return true
		}
	}
	return false
}

// --- the home trash (freedesktop Trash specification 1.0) ---

// homeTrashDir returns the home trash, $XDG_DATA_HOME/Trash, or
// ~/.local/share/Trash when XDG_DATA_HOME is unset or relative (the base
// directory specification says to ignore a relative one).
func homeTrashDir() (string, error) {
	data := os.Getenv("XDG_DATA_HOME")
	if !filepath.IsAbs(data) {
		home := os.Getenv("HOME")
		if !filepath.IsAbs(home) {
			return "", unsupported("no home trash: neither XDG_DATA_HOME nor HOME is an absolute path")
		}
		data = filepath.Join(home, ".local", "share")
	}
	return filepath.Join(data, "Trash"), nil
}

// deviceOf returns the device of a file's information (a variable for the
// tests, which cannot make two file systems).
var deviceOf = func(fi fs.FileInfo) (uint64, bool) {
	st, ok := fi.Sys().(*syscall.Stat_t)
	if !ok {
		return 0, false
	}
	return uint64(st.Dev), true
}

// moveToHomeTrash moves path (whose Lstat information is fi) into the
// home trash dir, as the specification describes. dir is created when
// missing (0700), with its files and info directories. Only a path on
// dir's file system is taken (dir's nearest existing ancestor tells
// which), so the move is a rename, never a copy and a delete; any other is
// ErrUnsupported (the specification's $topdir/.Trash-$uid of other mounts
// is not used). The name is reserved by creating info/<name>.trashinfo
// with O_EXCL, and the file is renamed to files/<name> without replacing
// anything (renameat2 RENAME_NOREPLACE); on a clash the next name is
// tried, "notes.2.md", "notes.3.md"… like GIO. The .trashinfo holds the
// original path, percent-encoded, and the deletion date in local time:
//
//	[Trash Info]
//	Path=/home/ana/My%20notes/todo.md
//	DeletionDate=2026-11-01T09:41:00
//
// The optional directorysizes cache is not written (its readers rebuild
// it). Nothing is left behind on failure: the info file goes again.
func moveToHomeTrash(path string, fi fs.FileInfo, dir string, now time.Time) error {
	switch {
	case path == dir || strings.HasPrefix(dir, path+string(filepath.Separator)):
		return fmt.Errorf("%s holds the trash itself: %w", path, fs.ErrInvalid)
	case strings.HasPrefix(path, dir+string(filepath.Separator)):
		return fmt.Errorf("%s is in the trash already: %w", path, fs.ErrInvalid)
	}
	dev, ok := deviceOf(fi)
	if !ok {
		return unsupported("cannot tell the file system of %s", path)
	}
	anc, err := existingAncestor(dir)
	if err != nil {
		return err
	}
	if adev, ok := deviceOf(anc); !ok || adev != dev {
		return unsupported("%s is on another file system than the home trash %s; only a rename moves a file there", path, dir)
	}
	files, info := filepath.Join(dir, "files"), filepath.Join(dir, "info")
	for _, d := range []string{files, info} {
		if err := os.MkdirAll(d, 0o700); err != nil {
			return fmt.Errorf("creating the home trash: %w", err)
		}
	}
	base := filepath.Base(path)
	for i := 1; i <= 10000; i++ {
		name := trashName(base, i)
		infoPath := filepath.Join(info, name+".trashinfo")
		f, err := os.OpenFile(infoPath, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o600)
		if errors.Is(err, fs.ErrExist) {
			continue
		}
		if err != nil {
			return fmt.Errorf("reserving a name in the trash: %w", err)
		}
		_, werr := f.WriteString(trashInfo(path, now))
		if cerr := f.Close(); werr == nil {
			werr = cerr
		}
		if werr != nil {
			os.Remove(infoPath)
			return fmt.Errorf("writing %s: %w", infoPath, werr)
		}
		err = renameNoReplace(path, filepath.Join(files, name))
		switch {
		case err == nil:
			return nil
		case errors.Is(err, fs.ErrExist):
			os.Remove(infoPath) // files/ has a stale entry with that name
			continue
		}
		os.Remove(infoPath)
		if errors.Is(err, unix.EXDEV) {
			return unsupported("%s is on another file system than the home trash %s", path, dir)
		}
		return fmt.Errorf("moving %s to the trash: %w", path, err)
	}
	return fmt.Errorf("no free name for %s in the trash", base)
}

// existingAncestor returns the information of dir, or of its nearest
// ancestor that exists.
func existingAncestor(dir string) (fs.FileInfo, error) {
	for p := dir; ; {
		fi, err := os.Stat(p)
		if err == nil {
			return fi, nil
		}
		parent := filepath.Dir(p)
		if parent == p || !errors.Is(err, fs.ErrNotExist) {
			return nil, fmt.Errorf("looking for the home trash: %w", err)
		}
		p = parent
	}
}

// renameNoReplace renames from to to, failing with an error wrapping
// fs.ErrExist when to exists. File systems without RENAME_NOREPLACE
// (EINVAL) get a check and a plain rename.
func renameNoReplace(from, to string) error {
	err := unix.Renameat2(unix.AT_FDCWD, from, unix.AT_FDCWD, to, unix.RENAME_NOREPLACE)
	if errors.Is(err, unix.EINVAL) || errors.Is(err, unix.ENOSYS) {
		if _, serr := os.Lstat(to); serr == nil {
			return fs.ErrExist
		}
		return os.Rename(from, to)
	}
	if errors.Is(err, unix.EEXIST) {
		return fs.ErrExist
	}
	if err != nil {
		return &os.LinkError{Op: "rename", Old: from, New: to, Err: err}
	}
	return nil
}

// trashName is the i-th name tried for base: base itself, then the number
// before the extension (after the first dot that does not start the name),
// as GIO does: "todo.md", "todo.2.md"; "notes", "notes.2"; ".bashrc",
// ".bashrc.2".
func trashName(base string, i int) string {
	if i == 1 {
		return base
	}
	if dot := strings.IndexByte(base[1:], '.'); dot >= 0 {
		return fmt.Sprintf("%s.%d%s", base[:dot+1], i, base[dot+1:])
	}
	return fmt.Sprintf("%s.%d", base, i)
}

// trashInfo is the content of a .trashinfo file.
func trashInfo(path string, now time.Time) string {
	return "[Trash Info]\nPath=" + escapeTrashPath(path) + "\nDeletionDate=" + now.Format("2006-01-02T15:04:05") + "\n"
}

// escapeTrashPath percent-encodes a path for a .trashinfo Path key, like
// GIO's g_uri_escape_string(path, "/", FALSE): letters, digits, "-._~"
// and "/" stay, every other byte is %XX (upper case).
func escapeTrashPath(p string) string {
	const hex = "0123456789ABCDEF"
	var b strings.Builder
	for i := 0; i < len(p); i++ {
		c := p[i]
		switch {
		case 'a' <= c && c <= 'z', 'A' <= c && c <= 'Z', '0' <= c && c <= '9',
			c == '-', c == '.', c == '_', c == '~', c == '/':
			b.WriteByte(c)
		default:
			b.WriteByte('%')
			b.WriteByte(hex[c>>4])
			b.WriteByte(hex[c&15])
		}
	}
	return b.String()
}
