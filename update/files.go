package update

import (
	"archive/zip"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path"
	"path/filepath"
	"runtime"
	"strings"
)

// unsupportedExchange is an exchange error the caller handles as
// errors.ErrUnsupported, falling back to two renames (exchange_*.go).
type unsupportedExchange struct{ err error }

func (e unsupportedExchange) Error() string { return "atomic exchange unsupported: " + e.err.Error() }

func (e unsupportedExchange) Is(target error) bool { return target == errors.ErrUnsupported }

func (e unsupportedExchange) Unwrap() error { return e.err }

// extractZip unpacks the zip file into dir, an empty folder, refusing
// entries whose names or symbolic links leave dir. File modes are kept
// (the executable bits of a bundle's programs), symbolic links too (a
// bundle's frameworks, whose links go through other links).
//
// A link's target is checked lexically when it is read, and nothing is
// written through a link (an entry whose parent is a link on disk is
// refused), so every entry lands where its name says; once every entry is
// written, each link is resolved on disk, following the links it goes
// through, and must end inside dir: a chain of links that each stay
// inside lexically (a/l -> .., b -> a/l/..) cannot reach out.
//
// On Windows a link is refused: a Windows install (the zip format) has
// none, the bundles that do are macOS's, and Wine's links are not seen by
// Lstat, which the checks above depend on.
func extractZip(file, dir string) error {
	zr, err := zip.OpenReader(file)
	if err != nil {
		return fmt.Errorf("update: the artifact is not a zip file: %w", err)
	}
	defer zr.Close()
	var links []string
	for _, f := range zr.File {
		name := f.Name
		clean := path.Clean(strings.TrimSuffix(name, "/"))
		if name == "" || strings.Contains(name, `\`) || path.IsAbs(name) || clean == ".." || strings.HasPrefix(clean, "../") || filepath.VolumeName(name) != "" {
			return fmt.Errorf("update: the zip artifact has an entry outside its folder: %q", name)
		}
		dst := filepath.Join(dir, filepath.FromSlash(clean))
		if err := noLinkAbove(dir, clean); err != nil {
			return err
		}
		mode := f.Mode()
		switch {
		case mode.IsDir():
			if err := os.MkdirAll(dst, 0o755); err != nil {
				return err
			}
		case mode&fs.ModeSymlink != 0:
			if runtime.GOOS == "windows" {
				return fmt.Errorf("update: the zip artifact has a symbolic link, %q, and a Windows install has none", name)
			}
			target, err := readEntry(f, 4096)
			if err != nil {
				return err
			}
			t := string(target)
			if path.IsAbs(t) || strings.Contains(t, `\`) {
				return fmt.Errorf("update: the zip artifact's link %q points outside its folder", name)
			}
			if rel := path.Clean(path.Join(path.Dir(clean), t)); rel == ".." || strings.HasPrefix(rel, "../") {
				return fmt.Errorf("update: the zip artifact's link %q points outside its folder", name)
			}
			if err := os.MkdirAll(filepath.Dir(dst), 0o755); err != nil {
				return err
			}
			if err := os.Symlink(filepath.FromSlash(t), dst); err != nil {
				return err
			}
			links = append(links, clean)
		default:
			if err := os.MkdirAll(filepath.Dir(dst), 0o755); err != nil {
				return err
			}
			if err := writeEntry(f, dst, filePerm(mode)); err != nil {
				return err
			}
		}
	}
	for _, l := range links {
		if err := resolvesInside(dir, l); err != nil {
			return fmt.Errorf("update: the zip artifact's link %q %w", l, err)
		}
	}
	return nil
}

// noLinkAbove refuses the entry at rel (a clean slash path in dir) when
// one of its parents in dir is a symbolic link: writing it would write
// through the link.
func noLinkAbove(dir, rel string) error {
	p := dir
	parts := strings.Split(rel, "/")
	for _, c := range parts[:len(parts)-1] {
		p = filepath.Join(p, c)
		st, err := os.Lstat(p)
		if err != nil {
			return nil // missing: nothing below it exists either
		}
		if st.Mode()&fs.ModeSymlink != 0 {
			return fmt.Errorf("update: the zip artifact's entry %q is inside a link", rel)
		}
	}
	return nil
}

// maxLinkHops bounds the links followed in resolving one path, as the
// kernels' own limits do (Linux's 40).
const maxLinkHops = 40

var (
	errLinkOutside = errors.New("points outside its folder")
	errLinkLoop    = errors.New("is part of a loop of links")
)

// resolvesInside resolves the link at rel (a clean slash path in dir) on
// disk, following every link on the way as the OS would, and returns
// errLinkOutside when it ends outside dir, errLinkLoop when it does not
// end. A missing component is taken as named (a dangling link to a file
// a framework's build stripped is fine while it names a place inside).
func resolvesInside(dir, rel string) error {
	hops := 0
	// walk appends the components of target to cur, a path in dir split
	// in components, following links.
	var walk func(cur []string, target string) ([]string, error)
	walk = func(cur []string, target string) ([]string, error) {
		for _, c := range strings.Split(target, "/") {
			switch c {
			case "", ".":
				continue
			case "..":
				if len(cur) == 0 {
					return nil, errLinkOutside
				}
				cur = cur[:len(cur)-1]
				continue
			}
			cur = append(cur, c)
			p := filepath.Join(dir, filepath.Join(cur...))
			st, err := os.Lstat(p)
			if err != nil || st.Mode()&fs.ModeSymlink == 0 {
				continue // a file, a folder or nothing: as named
			}
			if hops++; hops > maxLinkHops {
				return nil, errLinkLoop
			}
			t, err := os.Readlink(p)
			if err != nil {
				return nil, err
			}
			t = filepath.ToSlash(t)
			if path.IsAbs(t) || filepath.VolumeName(t) != "" {
				return nil, errLinkOutside
			}
			if cur, err = walk(cur[:len(cur)-1], t); err != nil {
				return nil, err
			}
		}
		return cur, nil
	}
	_, err := walk(nil, rel)
	return err
}

// filePerm is 0755 for a file with an executable bit, else 0644.
func filePerm(m fs.FileMode) fs.FileMode {
	if m&0o111 != 0 {
		return 0o755
	}
	return 0o644
}

func readEntry(f *zip.File, limit int64) ([]byte, error) {
	r, err := f.Open()
	if err != nil {
		return nil, err
	}
	defer r.Close()
	return io.ReadAll(io.LimitReader(r, limit))
}

func writeEntry(f *zip.File, dst string, perm fs.FileMode) error {
	r, err := f.Open()
	if err != nil {
		return err
	}
	defer r.Close()
	w, err := os.OpenFile(dst, os.O_WRONLY|os.O_CREATE|os.O_EXCL, perm)
	if err != nil {
		return err
	}
	if _, err := io.Copy(w, r); err != nil {
		w.Close()
		return err
	}
	return w.Close()
}

// copyFile copies the file src to dst, which must not exist, with perm.
func copyFile(src, dst string, perm fs.FileMode) error {
	r, err := os.Open(src)
	if err != nil {
		return err
	}
	defer r.Close()
	w, err := os.OpenFile(dst, os.O_WRONLY|os.O_CREATE|os.O_EXCL, perm)
	if err != nil {
		return err
	}
	if _, err := io.Copy(w, r); err != nil {
		w.Close()
		os.Remove(dst)
		return err
	}
	if err := w.Sync(); err != nil {
		w.Close()
		return err
	}
	return w.Close()
}

// copyTree copies the file, link or folder src to dst, keeping modes and
// links.
func copyTree(src, dst string) error {
	return filepath.WalkDir(src, func(p string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		rel, err := filepath.Rel(src, p)
		if err != nil {
			return err
		}
		to := filepath.Join(dst, rel)
		info, err := d.Info()
		if err != nil {
			return err
		}
		switch {
		case d.Type()&fs.ModeSymlink != 0:
			t, err := os.Readlink(p)
			if err != nil {
				return err
			}
			return os.Symlink(t, to)
		case d.IsDir():
			return os.MkdirAll(to, info.Mode().Perm()|0o700)
		default:
			return copyFile(p, to, info.Mode().Perm())
		}
	})
}
