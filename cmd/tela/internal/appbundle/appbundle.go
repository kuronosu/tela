// Package appbundle writes a macOS application bundle: a folder
// <Name>.app with Contents/Info.plist, Contents/MacOS/<executable> and,
// when there are files for it, Contents/Resources. It is the bundle writer
// of `tela build` and `tela run` (ADR-224, ADR-227), and the one the
// packaging phase extends with icons, signing and installers. It runs on
// every OS: a Linux machine can build the bundle of a darwin program.
//
// # Identity
//
// Launch Services knows an app by its bundle identifier (CFBundleIdentifier)
// and where the bundle is. Writing a bundle that already exists therefore
// changes only what differs, so a rebuild of an unchanged app leaves every
// file, with its modification time and inode, as it was:
//
//   - Info.plist is read, merged and rewritten only when the encoded bytes
//     differ. The keys Spec manages are set from it; every other key is kept,
//     including the ones a running app added (CFBundleURLTypes and
//     CFBundleDocumentTypes, written by the registration of URL schemes and
//     file types) and the ones Spec.Extra sets.
//   - The executable is replaced only when its bytes differ, by writing a
//     temporary file and renaming it over the old one, so an instance that is
//     still running keeps its own file (overwriting a running Mach-O in place
//     can get the process killed).
//   - Other files of Contents/MacOS (the executable of an earlier name) are
//     removed, and the files named by Spec.Resources are replaced only when
//     their contents differ. Nothing else is touched.
//
// # Code signature
//
// The package never runs codesign. The Go linker ad-hoc signs the
// executables it writes for darwin/arm64 (Apple silicon refuses to run an
// unsigned arm64 binary), and darwin/amd64 executables carry no signature.
// That signature covers the Mach-O file only: it binds neither Info.plist
// nor Resources (no CodeResources seal), which is why registration may edit
// Info.plist later without invalidating anything. A bundle written here is
// therefore a development bundle: fine to run on the machine that built it,
// not signed with a developer identity, not notarized, and without the seal
// a distributed app needs. Signing and notarization belong to packaging.
package appbundle

import (
	"bytes"
	"crypto/sha256"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"strings"

	"github.com/kuronosu/tela/internal/plist"
)

// MinSystemVersion is the macOS version Tela supports (ADR-222): the default
// LSMinimumSystemVersion.
const MinSystemVersion = "13.0"

// Spec describes a bundle.
type Spec struct {
	// ID is the CFBundleIdentifier, the app's AppID. It must pass
	// [ValidID].
	ID string
	// Name is CFBundleName, the short name of the app. DisplayName
	// (CFBundleDisplayName) defaults to it.
	Name        string
	DisplayName string
	// Executable is the file name of the program in Contents/MacOS and
	// CFBundleExecutable. It defaults to Name, made safe as a file name.
	Executable string
	// Version is CFBundleShortVersionString and BuildVersion
	// CFBundleVersion; both default to "0.0.0".
	Version      string
	BuildVersion string
	// MinSystem is LSMinimumSystemVersion (default [MinSystemVersion]).
	MinSystem string
	// Extra holds more Info.plist keys, set after the ones above, for the
	// packaging phase (CFBundleIconFile, LSApplicationCategoryType, ...).
	Extra plist.Dict
	// Resources maps a path inside Contents/Resources (slash-separated) to
	// the file whose contents go there.
	Resources map[string]string
}

// Result says what Write changed.
type Result struct {
	// Dir is the bundle folder, Exe the executable inside it.
	Dir, Exe string
	// Created is true when the bundle did not exist.
	Created bool
	// InfoChanged and ExeChanged are true when Info.plist and the
	// executable were written (they always are when Created).
	InfoChanged, ExeChanged bool
}

// ValidID checks id as a CFBundleIdentifier: letters, digits, hyphens and
// periods only (Apple's rule; reverse-DNS names such as com.example.notes
// follow it), and not empty. The error names the offending character and
// says what to do.
func ValidID(id string) error {
	if id == "" {
		return errors.New("the app ID is empty")
	}
	for _, r := range id {
		switch {
		case r >= 'a' && r <= 'z', r >= 'A' && r <= 'Z', r >= '0' && r <= '9', r == '-', r == '.':
		default:
			return fmt.Errorf("the app ID %q has %q, which CFBundleIdentifier does not allow: use only letters, digits, hyphens and periods, for example \"com.example.notes\"", id, r)
		}
	}
	if strings.HasPrefix(id, ".") || strings.HasSuffix(id, ".") || strings.Contains(id, "..") {
		return fmt.Errorf("the app ID %q has an empty part between periods; use a reverse-DNS name such as \"com.example.notes\"", id)
	}
	return nil
}

// FileName makes name safe as one file name on every OS: path separators,
// colons (the old Finder separator) and control characters become hyphens,
// and leading periods and surrounding spaces go. The result is never
// empty.
func FileName(name string) string {
	var b strings.Builder
	for _, r := range name {
		switch {
		case r == '/' || r == '\\' || r == ':' || r == '\x00' || r < 0x20 || r == 0x7f:
			b.WriteByte('-')
		default:
			b.WriteRune(r)
		}
	}
	s := strings.Trim(strings.TrimLeft(b.String(), "."), " ")
	if s == "" {
		return "App"
	}
	return s
}

// Info returns the Info.plist keys that spec manages (the ones Write sets
// every time), with the defaults applied. It does not check them.
func (s Spec) Info() plist.Dict {
	name := s.Name
	display := s.DisplayName
	if display == "" {
		display = name
	}
	exe := s.Executable
	if exe == "" {
		exe = FileName(name)
	}
	version := orDefault(s.Version, "0.0.0")
	d := plist.Dict{
		"CFBundleIdentifier":                   s.ID,
		"CFBundleName":                         name,
		"CFBundleDisplayName":                  display,
		"CFBundleExecutable":                   exe,
		"CFBundlePackageType":                  "APPL",
		"CFBundleInfoDictionaryVersion":        "6.0",
		"CFBundleShortVersionString":           version,
		"CFBundleVersion":                      orDefault(s.BuildVersion, version),
		"LSMinimumSystemVersion":               orDefault(s.MinSystem, MinSystemVersion),
		"NSHighResolutionCapable":              true,
		"NSSupportsAutomaticGraphicsSwitching": true,
	}
	for k, v := range s.Extra {
		d[k] = v
	}
	return d
}

func orDefault(s, def string) string {
	if s == "" {
		return def
	}
	return s
}

// Write makes the bundle at dir (the path of the <Name>.app folder, which
// is created with its parents when missing) from spec and the program at
// exe, changing only what differs. See the package documentation.
func Write(dir string, spec Spec, exe string) (Result, error) {
	res := Result{Dir: dir}
	if err := ValidID(spec.ID); err != nil {
		return res, err
	}
	if spec.Name == "" {
		return res, errors.New("appbundle: the bundle needs a name (Spec.Name)")
	}
	info := spec.Info()
	exeName := info["CFBundleExecutable"].(string)
	if exeName != FileName(exeName) || strings.TrimSpace(exeName) != exeName {
		return res, fmt.Errorf("appbundle: %q is not usable as the executable's file name; use Spec.Executable with a plain name", exeName)
	}
	if _, err := os.Stat(dir); errors.Is(err, fs.ErrNotExist) {
		res.Created = true
	} else if err != nil {
		return res, err
	}

	contents := filepath.Join(dir, "Contents")
	macOS := filepath.Join(contents, "MacOS")
	if err := os.MkdirAll(macOS, 0o755); err != nil {
		return res, err
	}

	// Info.plist: merge into what is there.
	plistPath := filepath.Join(contents, "Info.plist")
	merged := plist.Dict{}
	old, err := os.ReadFile(plistPath)
	switch {
	case err == nil:
		// An unreadable file is replaced: the keys it had cannot be known.
		if d, perr := plist.UnmarshalDict(old); perr == nil {
			merged = d
		}
	case !errors.Is(err, fs.ErrNotExist):
		return res, err
	}
	for k, v := range info {
		merged[k] = v
	}
	data, err := plist.Marshal(merged)
	if err != nil {
		return res, fmt.Errorf("appbundle: Info.plist: %w", err)
	}
	if !bytes.Equal(data, old) {
		if err := writeAtomic(plistPath, bytes.NewReader(data), 0o644); err != nil {
			return res, err
		}
		res.InfoChanged = true
	}

	// The executable.
	res.Exe = filepath.Join(macOS, exeName)
	same, err := sameFile(exe, res.Exe)
	if err != nil {
		return res, err
	}
	if !same {
		src, err := os.Open(exe)
		if err != nil {
			return res, err
		}
		err = writeAtomic(res.Exe, src, 0o755)
		src.Close()
		if err != nil {
			return res, err
		}
		res.ExeChanged = true
	}
	// A leftover of another name (the app was renamed) is stale.
	entries, err := os.ReadDir(macOS)
	if err != nil {
		return res, err
	}
	for _, e := range entries {
		if e.Name() != exeName {
			if err := os.RemoveAll(filepath.Join(macOS, e.Name())); err != nil {
				return res, err
			}
		}
	}

	// Resources.
	if len(spec.Resources) > 0 {
		resDir := filepath.Join(contents, "Resources")
		for rel, src := range spec.Resources {
			clean := filepath.FromSlash(rel)
			if !filepath.IsLocal(clean) {
				return res, fmt.Errorf("appbundle: the resource path %q must stay inside Contents/Resources", rel)
			}
			dst := filepath.Join(resDir, clean)
			if err := os.MkdirAll(filepath.Dir(dst), 0o755); err != nil {
				return res, err
			}
			same, err := sameFile(src, dst)
			if err != nil {
				return res, err
			}
			if same {
				continue
			}
			f, err := os.Open(src)
			if err != nil {
				return res, err
			}
			err = writeAtomic(dst, f, 0o644)
			f.Close()
			if err != nil {
				return res, err
			}
		}
	}
	return res, nil
}

// writeAtomic writes r to path through a temporary file in the same folder
// and a rename, so that readers (and a running program) see the old file or
// the new one whole.
func writeAtomic(path string, r io.Reader, perm fs.FileMode) error {
	tmp, err := os.CreateTemp(filepath.Dir(path), ".tmp-"+filepath.Base(path)+"-*")
	if err != nil {
		return err
	}
	name := tmp.Name()
	cleanup := func(err error) error {
		tmp.Close()
		os.Remove(name)
		return err
	}
	if _, err := io.Copy(tmp, r); err != nil {
		return cleanup(err)
	}
	if err := tmp.Chmod(perm); err != nil {
		return cleanup(err)
	}
	if err := tmp.Close(); err != nil {
		return cleanup(err)
	}
	if err := os.Rename(name, path); err != nil {
		os.Remove(name)
		return err
	}
	return nil
}

// sameFile reports whether the files at a and b have the same contents; a
// missing b is not the same, a missing a is an error.
func sameFile(a, b string) (bool, error) {
	sa, err := os.Stat(a)
	if err != nil {
		return false, err
	}
	sb, err := os.Stat(b)
	if errors.Is(err, fs.ErrNotExist) {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	if !sb.Mode().IsRegular() || sa.Size() != sb.Size() {
		return false, nil
	}
	ha, err := hashFile(a)
	if err != nil {
		return false, err
	}
	hb, err := hashFile(b)
	if err != nil {
		return false, err
	}
	return ha == hb, nil
}

func hashFile(path string) ([sha256.Size]byte, error) {
	var sum [sha256.Size]byte
	f, err := os.Open(path)
	if err != nil {
		return sum, err
	}
	defer f.Close()
	h := sha256.New()
	if _, err := io.Copy(h, f); err != nil {
		return sum, err
	}
	copy(sum[:], h.Sum(nil))
	return sum, nil
}
