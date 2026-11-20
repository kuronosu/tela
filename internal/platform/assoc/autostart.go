package assoc

import (
	"bytes"
	"fmt"
	"path/filepath"
	"strings"

	"github.com/kuronosu/tela/internal/platform"
)

// Start at login on Linux (platform contract v16, ADR-298): the XDG
// Desktop Application Autostart specification. The desktop starts at
// login the entries of $XDG_CONFIG_HOME/autostart and of each
// $XDG_CONFIG_DIRS/autostart, the user's file of an ID shadowing a system
// one, unless the entry is Hidden=true (the specification's switch, which
// KDE's settings write) or X-GNOME-Autostart-enabled=false (GNOME
// Tweaks'). What is written, and nothing else:
//
//   - On: $XDG_CONFIG_HOME/autostart/<ID>.desktop, written from scratch
//     and marked X-Tela-Registered=true, whose Exec runs HandlerApp.Exec
//     and Args with platform.AtLoginFlag. An entry of the ID that already
//     starts the app (the user's own, a package's in /etc/xdg/autostart)
//     is left as it is: the app starts at login already.
//   - Off: Tela's entry is removed; an entry of someone else's in the
//     user's folder gets Hidden=true (the rest of the file kept); a system
//     entry is shadowed by a minimal one of Tela's with Hidden=true.
//
// The file is written atomically and only when its content changes.

const (
	autostartDir    = "autostart"
	hiddenKey       = "Hidden"
	gnomeEnabledKey = "X-GNOME-Autostart-enabled"
)

// autostartPath returns the user's autostart entry of the ID.
func autostartPath(dirs LinuxDirs, id string) string {
	return filepath.Join(dirs.ConfigHome, autostartDir, id+".desktop")
}

// autostartEntry renders Tela's autostart entry of app.
func autostartEntry(app platform.HandlerApp) []byte {
	var b strings.Builder
	b.WriteString("# Written by Tela (desktop.SetStartAtLogin): starts the app when the user\n")
	b.WriteString("# logs in. desktop.SetStartAtLogin(false) removes it.\n")
	b.WriteString("[" + entryGroup + "]\n")
	b.WriteString("Type=Application\n")
	b.WriteString("Version=1.5\n")
	b.WriteString("Name=" + escapeValue(app.Name) + "\n")
	var args []string
	for _, a := range append(append([]string{app.Exec}, app.Args...), platform.AtLoginFlag) {
		args = append(args, quoteExecArg(a))
	}
	b.WriteString("Exec=" + escapeValue(strings.Join(args, " ")) + "\n")
	if app.Icon != "" {
		b.WriteString("Icon=" + escapeValue(app.Icon) + "\n")
	}
	b.WriteString("Terminal=false\n")
	// No NoDisplay: the desktop's startup settings list the entry, so the
	// user can turn it off there too.
	b.WriteString(gnomeEnabledKey + "=true\n")
	b.WriteString(telaKey + "=true\n")
	return []byte(b.String())
}

// autostartOverride renders the minimal entry that turns a system entry
// off.
func autostartOverride(app platform.HandlerApp) []byte {
	var b strings.Builder
	b.WriteString("# Written by Tela (desktop.SetStartAtLogin(false)): keeps the system's\n")
	b.WriteString("# autostart entry of the app from starting it.\n")
	b.WriteString("[" + entryGroup + "]\n")
	b.WriteString("Type=Application\n")
	b.WriteString("Name=" + escapeValue(app.Name) + "\n")
	b.WriteString(hiddenKey + "=true\n")
	b.WriteString(telaKey + "=true\n")
	return []byte(b.String())
}

// autostartFile is one autostart entry found for the ID.
type autostartFile struct {
	path  string
	raw   []byte
	entry *keyFile
	user  bool // in $XDG_CONFIG_HOME
}

func (f *autostartFile) tela() bool {
	v, _ := f.entry.get(entryGroup, telaKey)
	return v == "true"
}

// enabled reports whether the desktop starts the entry.
func (f *autostartFile) enabled() bool {
	if v, _ := f.entry.get(entryGroup, hiddenKey); strings.EqualFold(strings.TrimSpace(v), "true") {
		return false
	}
	if v, ok := f.entry.get(entryGroup, gnomeEnabledKey); ok && strings.EqualFold(strings.TrimSpace(v), "false") {
		return false
	}
	return true
}

// autostartFiles returns the user's entry of the ID and the first
// system one, either nil when missing.
func autostartFiles(dirs LinuxDirs, id string) (user, system *autostartFile, err error) {
	read := func(path string, isUser bool) (*autostartFile, error) {
		raw, err := readMaybe(path)
		if err != nil || raw == nil {
			return nil, err
		}
		return &autostartFile{path: path, raw: raw, entry: parseKeyFile(raw), user: isUser}, nil
	}
	if user, err = read(autostartPath(dirs, id), true); err != nil {
		return nil, nil, fmt.Errorf("assoc: %w", err)
	}
	for _, d := range dirs.ConfigDirs {
		if d == "" {
			continue
		}
		if system, err = read(filepath.Join(d, autostartDir, id+".desktop"), false); err != nil {
			return nil, nil, fmt.Errorf("assoc: %w", err)
		}
		if system != nil {
			break
		}
	}
	return user, system, nil
}

func checkAutostartDirs(dirs LinuxDirs) error {
	if !filepath.IsAbs(dirs.ConfigHome) {
		return fmt.Errorf("assoc: LinuxDirs.ConfigHome %q is not an absolute path ($XDG_CONFIG_HOME)", dirs.ConfigHome)
	}
	return nil
}

// LinuxStartsAtLogin reports whether the desktop starts app at login:
// the user's autostart entry of its ID, else the first system one, is
// there and not turned off.
func LinuxStartsAtLogin(dirs LinuxDirs, app platform.HandlerApp) (bool, error) {
	platform.CheckHandlerApp("StartsAtLogin", app, false)
	if err := checkAutostartDirs(dirs); err != nil {
		return false, err
	}
	linuxMu.Lock()
	defer linuxMu.Unlock()
	user, system, err := autostartFiles(dirs, app.ID)
	switch {
	case err != nil:
		return false, err
	case user != nil:
		return user.enabled(), nil
	case system != nil:
		return system.enabled(), nil
	}
	return false, nil
}

// LinuxSetStartAtLogin turns app's start at login on or off (see the top
// of the file) and reports whether it wrote anything.
func LinuxSetStartAtLogin(dirs LinuxDirs, app platform.HandlerApp, on bool) (bool, error) {
	platform.CheckHandlerApp("SetStartAtLogin", app, on)
	if err := checkAutostartDirs(dirs); err != nil {
		return false, err
	}
	linuxMu.Lock()
	defer linuxMu.Unlock()
	user, system, err := autostartFiles(dirs, app.ID)
	if err != nil {
		return false, err
	}
	path := autostartPath(dirs, app.ID)
	write := func(data []byte) (bool, error) {
		if user != nil && bytes.Equal(user.raw, data) {
			return false, nil
		}
		if err := writeFileAtomic(path, data); err != nil {
			return false, fmt.Errorf("assoc: %w", err)
		}
		return true, nil
	}
	if on {
		switch {
		case user != nil && !user.tela() && user.enabled(),
			user == nil && system != nil && system.enabled():
			return false, nil // it starts already, from an entry not Tela's
		case user != nil && !user.tela():
			// Someone else's entry the user turned off: turn it on again,
			// keeping the rest of their file.
			user.entry.del(entryGroup, hiddenKey)
			if _, ok := user.entry.get(entryGroup, gnomeEnabledKey); ok {
				user.entry.set(entryGroup, gnomeEnabledKey, "true")
			}
			return write(user.entry.bytes())
		}
		return write(autostartEntry(app))
	}
	switch {
	case user != nil && user.tela():
		if system != nil && system.enabled() {
			return write(autostartOverride(app))
		}
		did, err := removeFile(path)
		if err != nil {
			return false, fmt.Errorf("assoc: %w", err)
		}
		return did, nil
	case user != nil:
		if !user.enabled() {
			return false, nil
		}
		user.entry.set(entryGroup, hiddenKey, "true")
		return write(user.entry.bytes())
	case system != nil && system.enabled():
		return write(autostartOverride(app))
	}
	return false, nil
}
