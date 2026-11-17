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
	"strings"
)

// unsupportedExchange is an exchange error the caller handles as
// errors.ErrUnsupported, falling back to two renames (exchange_*.go).
type unsupportedExchange struct{ err error }

func (e unsupportedExchange) Error() string { return "atomic exchange unsupported: " + e.err.Error() }

func (e unsupportedExchange) Is(target error) bool { return target == errors.ErrUnsupported }

func (e unsupportedExchange) Unwrap() error { return e.err }

// extractZip unpacks the zip file into dir, refusing entries whose names
// or symbolic links leave dir. File modes are kept (the executable bits
// of a bundle's programs), symbolic links too (a bundle's frameworks).
func extractZip(file, dir string) error {
	zr, err := zip.OpenReader(file)
	if err != nil {
		return fmt.Errorf("update: the artifact is not a zip file: %w", err)
	}
	defer zr.Close()
	for _, f := range zr.File {
		name := f.Name
		clean := path.Clean(strings.TrimSuffix(name, "/"))
		if name == "" || strings.Contains(name, `\`) || path.IsAbs(name) || clean == ".." || strings.HasPrefix(clean, "../") || filepath.VolumeName(name) != "" {
			return fmt.Errorf("update: the zip artifact has an entry outside its folder: %q", name)
		}
		dst := filepath.Join(dir, filepath.FromSlash(clean))
		mode := f.Mode()
		switch {
		case mode.IsDir():
			if err := os.MkdirAll(dst, 0o755); err != nil {
				return err
			}
		case mode&fs.ModeSymlink != 0:
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
		default:
			if err := os.MkdirAll(filepath.Dir(dst), 0o755); err != nil {
				return err
			}
			if err := writeEntry(f, dst, filePerm(mode)); err != nil {
				return err
			}
		}
	}
	return nil
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
