package update

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"slices"
	"strings"

	"github.com/kuronosu/tela/desktop"
)

// HealthyStarts is how many starts a new version gets to call
// [MarkHealthy]: on the next start without it, the old copy is put back,
// the rollback recorded ([RolledBack]) and the old version started in its
// place. The version is not offered again on that install.
const HealthyStarts = 2

// Apply puts the downloaded update in place of the install, keeping the
// old copy, for the formats that update themselves:
//
//   - appimage: the file at $APPIMAGE is replaced by an atomic rename;
//     the old file stays in .<name>.AppImage.update next to it.
//   - app: the macOS bundle is swapped for the one in the artifact's zip
//     by renames in its parent folder (an atomic exchange where the file
//     system has one); the old one stays in .<Name>.app.update.
//   - zip: the Windows folder's entries listed by the old and new
//     MarkerName files are moved into the folder's .update folder and the
//     new ones moved in. Windows lets a running executable be renamed,
//     not replaced, which is what this does.
//
// The new copy is unpacked in the cache folder and renamed into place,
// or copied next to the install first when the cache is on another
// volume, so the final step is a rename on one volume.
//
// The running program keeps running the old version; the new one runs
// from the next start ([Restart] starts it now). Until a start of the new
// version calls [MarkHealthy], it is on trial: see [HealthyStarts].
// Applying again before a restart replaces the version on trial and
// keeps the old copy.
func (u *Update) Apply() error {
	if u.file == "" {
		return errNotDownloaded
	}
	if err := verifyFile(u.file, u.Artifact.Size, u.Artifact.SHA256); err != nil {
		return err
	}
	stateMu.Lock()
	defer stateMu.Unlock()
	in := u.inst
	st, err := in.loadState()
	if err != nil {
		return err
	}
	cur := currentVersion()
	// The version on trial applies another update: it works well
	// enough, and it is the copy to keep.
	if p := st.Pending; p != nil && p.Version == cur {
		st.Pending = nil
	}
	// replacing: the installed copy is a version on trial that never ran
	// (applied, not restarted yet), discarded rather than kept.
	replacing := st.Pending != nil
	dir := in.updateDir()
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return err
	}
	// What a rollback could not delete (a Windows program that was
	// running then).
	os.RemoveAll(filepath.Join(dir, "failed"))
	stage, err := stageDir()
	if err != nil {
		return err
	}
	defer os.RemoveAll(stage)

	p := &pending{Version: u.Version, From: cur, Applied: now().UTC()}
	if replacing {
		p.From, p.OldFiles = st.Pending.From, st.Pending.OldFiles
	}
	switch in.format {
	case "appimage":
		err = applyFile(in, u.file, replacing)
	case "app":
		err = applyBundle(in, u.file, stage, replacing)
	case "zip":
		err = applyFolder(in, u.file, stage, replacing, st.Pending, p)
	default:
		err = fmt.Errorf("update: cannot apply to a %s install", in.format)
	}
	if err != nil {
		return err
	}
	st.Pending = p
	if err := in.saveState(st); err != nil {
		return err
	}
	os.Remove(u.file)
	u.file = ""
	return nil
}

// stageDir returns a new folder in the cache for unpacking.
func stageDir() (string, error) {
	dir, err := desktop.CacheDir()
	if err != nil {
		return "", err
	}
	dir = filepath.Join(dir, "update")
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return "", err
	}
	return os.MkdirTemp(dir, "stage-")
}

// moveTo moves src to dst, which must not exist: a rename, or a copy and
// a removal when they are on different volumes.
func moveTo(src, dst string) error {
	if err := os.Rename(src, dst); err == nil {
		return nil
	}
	if err := copyTree(src, dst); err != nil {
		os.RemoveAll(dst)
		return err
	}
	return os.RemoveAll(src)
}

// applyFile replaces the AppImage file.
func applyFile(in *install, file string, replacing bool) error {
	dir := in.updateDir()
	next, old := filepath.Join(dir, "new"), filepath.Join(dir, "old")
	os.RemoveAll(next)
	// Copy rather than move: the cache keeps the verified file until the
	// swap is done, and a copy next to the install is on its volume.
	if err := copyFile(file, next, 0o755); err != nil {
		return err
	}
	if mode, err := os.Stat(in.path); err == nil {
		os.Chmod(next, mode.Mode().Perm()|0o100)
	}
	if !replacing {
		os.Remove(old)
		// A hard link keeps the old file without a moment where the
		// install's path is missing; file systems without links get a
		// rename.
		if err := os.Link(in.path, old); err != nil {
			if err := os.Rename(in.path, old); err != nil {
				return err
			}
		}
	}
	if err := os.Rename(next, in.path); err != nil {
		if !replacing {
			restoreFile(old, in.path)
		}
		return err
	}
	return nil
}

func restoreFile(old, path string) {
	if _, err := os.Stat(path); errors.Is(err, fs.ErrNotExist) {
		os.Rename(old, path)
	}
}

// applyBundle swaps the macOS bundle.
func applyBundle(in *install, file, stage string, replacing bool) error {
	if err := extractZip(file, stage); err != nil {
		return err
	}
	ents, err := os.ReadDir(stage)
	if err != nil {
		return err
	}
	var app string
	for _, e := range ents {
		if e.IsDir() && strings.HasSuffix(e.Name(), ".app") {
			if app != "" {
				return fmt.Errorf("update: the artifact holds several bundles (%s, %s)", app, e.Name())
			}
			app = e.Name()
		}
	}
	if app == "" {
		return errors.New("update: the artifact holds no .app bundle at its top level")
	}
	dir := in.updateDir()
	next, old, failed := filepath.Join(dir, "new"), filepath.Join(dir, "old"), filepath.Join(dir, "failed")
	os.RemoveAll(next)
	if err := moveTo(filepath.Join(stage, app), next); err != nil {
		return err
	}
	return swapDir(in.path, next, old, failed, replacing)
}

// swapDir puts the folder next at path. The folder at path goes to old,
// or, when replacing, to failed and is deleted.
func swapDir(path, next, old, failed string, replacing bool) error {
	aside := old
	if replacing {
		aside = failed
	}
	os.RemoveAll(aside)
	// An atomic exchange leaves no moment without a bundle at path.
	if err := exchange(next, path); err == nil {
		if err := os.Rename(next, aside); err != nil {
			return err
		}
	} else {
		if err := os.Rename(path, aside); err != nil {
			return err
		}
		if err := os.Rename(next, path); err != nil {
			os.Rename(aside, path)
			return err
		}
	}
	if replacing {
		os.RemoveAll(failed)
	}
	return nil
}

// applyFolder moves the zip folder's entries.
func applyFolder(in *install, file, stage string, replacing bool, prev, p *pending) error {
	if err := extractZip(file, stage); err != nil {
		return err
	}
	root, m, err := zipRoot(stage)
	if err != nil {
		return err
	}
	news := m.Files
	if len(news) == 0 {
		ents, err := os.ReadDir(root)
		if err != nil {
			return err
		}
		for _, e := range ents {
			news = append(news, e.Name())
		}
	}
	for _, n := range news {
		if !validEntry(n) || n == ".update" {
			return fmt.Errorf("update: the new %s lists %q, which is not a top-level name it may hold", MarkerName, n)
		}
		if _, err := os.Lstat(filepath.Join(root, n)); err != nil {
			return fmt.Errorf("update: the new %s lists %q, which the artifact lacks", MarkerName, n)
		}
	}
	dir := in.updateDir()
	next, old, failed := filepath.Join(dir, "new"), filepath.Join(dir, "old"), filepath.Join(dir, "failed")
	os.RemoveAll(next)
	if err := moveTo(root, next); err != nil {
		return err
	}
	defer os.RemoveAll(next)

	if replacing {
		// The installed entries are the version on trial, which never
		// ran: set them aside and delete them; the old copy stays.
		os.RemoveAll(failed)
		if err := moveEntries(in.path, failed, prev.Files); err != nil {
			return err
		}
		defer os.RemoveAll(failed)
	} else {
		// The old copy is the installed entries, the old marker's and
		// any the new version would overwrite.
		olds := slices.Clone(in.marker.Files)
		for _, n := range news {
			if !slices.Contains(olds, n) {
				olds = append(olds, n)
			}
		}
		if err := os.RemoveAll(old); err != nil {
			return fmt.Errorf("update: cannot remove the previous old copy (%w); restart the app before applying another update", err)
		}
		if err := os.MkdirAll(old, 0o755); err != nil {
			return err
		}
		var moved []string
		for _, n := range olds {
			if !validEntry(n) || n == ".update" {
				continue
			}
			if _, err := os.Lstat(filepath.Join(in.path, n)); err != nil {
				continue
			}
			moved = append(moved, n)
		}
		// The running executable is renamed into old, which Windows
		// allows (deleting or overwriting it is not).
		if err := moveEntries(in.path, old, moved); err != nil {
			moveEntries(old, in.path, moved)
			return err
		}
		p.OldFiles = moved
	}
	if err := moveEntries(next, in.path, news); err != nil {
		return err
	}
	p.Files = news
	in.exe = filepath.Join(in.path, m.Exe)
	return nil
}

// moveEntries renames the entries names of folder from into folder to,
// stopping at the first error. Missing entries are skipped.
func moveEntries(from, to string, names []string) error {
	if err := os.MkdirAll(to, 0o755); err != nil {
		return err
	}
	for _, n := range names {
		src := filepath.Join(from, n)
		if _, err := os.Lstat(src); errors.Is(err, fs.ErrNotExist) {
			continue
		}
		dst := filepath.Join(to, n)
		if err := os.Rename(src, dst); err != nil {
			return err
		}
	}
	return nil
}

func validEntry(n string) bool {
	return n != "" && n != "." && n != ".." && !strings.ContainsAny(n, `/\:`)
}

// zipRoot returns the folder of an unpacked zip artifact that holds its
// marker: the top level, or its one folder.
func zipRoot(stage string) (string, *Marker, error) {
	if m, err := readMarker(filepath.Join(stage, MarkerName)); err == nil {
		return stage, m, nil
	} else if !errors.Is(err, fs.ErrNotExist) {
		return "", nil, err
	}
	ents, err := os.ReadDir(stage)
	if err != nil {
		return "", nil, err
	}
	if len(ents) == 1 && ents[0].IsDir() {
		root := filepath.Join(stage, ents[0].Name())
		m, err := readMarker(filepath.Join(root, MarkerName))
		if err == nil {
			return root, m, nil
		}
		if !errors.Is(err, fs.ErrNotExist) {
			return "", nil, err
		}
	}
	return "", nil, fmt.Errorf("update: the zip artifact has no %s at its top level or in its one folder", MarkerName)
}

// rollback puts the old copy of in back, records why, and returns the
// program to start. Call it with stateMu held.
func rollback(in *install, st *installState, reason string) error {
	p := st.Pending
	dir := in.updateDir()
	old, failed := filepath.Join(dir, "old"), filepath.Join(dir, "failed")
	var err error
	switch in.format {
	case "appimage":
		err = os.Rename(old, in.path)
	case "app":
		os.RemoveAll(failed)
		if err = exchange(old, in.path); err == nil {
			err = os.Rename(old, failed)
		} else if err = os.Rename(in.path, failed); err == nil {
			err = os.Rename(old, in.path)
		}
		os.RemoveAll(failed)
	case "zip":
		// The running executable is the new version's: renamed aside,
		// it is deleted at the next Apply or rollback.
		os.RemoveAll(failed)
		if err = moveEntries(in.path, failed, p.Files); err == nil {
			err = moveEntries(old, in.path, p.OldFiles)
		}
		os.RemoveAll(failed)
		if m, merr := readMarker(filepath.Join(in.path, MarkerName)); merr == nil {
			in.exe = filepath.Join(in.path, m.Exe)
		}
	}
	if err != nil {
		return fmt.Errorf("update: rolling back %s: %w", p.Version, err)
	}
	st.Pending = nil
	st.Rollbacks = append(st.Rollbacks, Rollback{Version: p.Version, From: p.From, Reason: reason, At: now().UTC()})
	return in.saveState(st)
}

// MarkHealthy confirms that the running version works, ending its trial
// after an update ([HealthyStarts]): call it once the app has shown its
// window and read its data. It does nothing when the running version is
// not on trial, and for installs that do not update themselves.
func MarkHealthy() error {
	in, err := findSelf()
	if err != nil {
		return nil
	}
	stateMu.Lock()
	defer stateMu.Unlock()
	st, err := in.loadState()
	if err != nil {
		return err
	}
	if st.Pending == nil || st.Pending.Version != currentVersion() {
		return nil
	}
	st.Pending = nil
	return in.saveState(st)
}

// RolledBack returns the newest rollback of this install, to tell the
// user ("Notes 1.2.0 did not start and was undone"); false when there was
// none. Compare its At with the last one shown.
func RolledBack() (Rollback, bool) {
	in, err := findSelf()
	if err != nil {
		return Rollback{}, false
	}
	stateMu.Lock()
	defer stateMu.Unlock()
	st, err := in.loadState()
	if err != nil || len(st.Rollbacks) == 0 {
		return Rollback{}, false
	}
	return st.Rollbacks[len(st.Rollbacks)-1], true
}

// startCheck runs at the start of the app (tela.Run, before any window):
// it counts a start of a version on trial, and past HealthyStarts rolls
// it back. It returns the program to start instead, "" to go on.
func startCheck() (restart string) {
	in, err := findSelf()
	if err != nil {
		return ""
	}
	stateMu.Lock()
	defer stateMu.Unlock()
	st, err := in.loadState()
	if err != nil || st.Pending == nil || st.Pending.Version != currentVersion() {
		return ""
	}
	st.Pending.Starts++
	if st.Pending.Starts <= HealthyStarts {
		in.saveState(st)
		return ""
	}
	reason := fmt.Sprintf("version %s was not marked healthy within %d starts", st.Pending.Version, HealthyStarts)
	if err := rollback(in, st, reason); err != nil {
		fmt.Fprintln(os.Stderr, err)
		return ""
	}
	return in.exe
}
