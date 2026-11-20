package assoc

import (
	"bytes"
	"fmt"
	"strconv"
	"strings"

	"github.com/kuronosu/tela/internal/platform"
)

// The app's tasks on Linux (platform contract v16, ADR-298): the
// "Additional application actions" of the Desktop Entry specification in
// the app's own entry, the one Tela wrote ($XDG_DATA_HOME/applications/
// <ID>.desktop: RegisterApp's, or the associations'). Launchers show them
// in the app's menu (GNOME Shell's dash, KDE's task manager, rofi's drun).
// What is written:
//
//   - Actions=tela-1;tela-2;… in [Desktop Entry], and a
//     [Desktop Action tela-N] group per task with Name (the title), Exec
//     (the entry's Exec without its field code, then the task's arguments,
//     each quoted) and Icon (the path the caller wrote the icon to, if
//     any). The actions replace every earlier one of the entry.
//   - No tasks remove Actions and the action groups.
//
// The associations and RegisterApp keep the actions when they render the
// entry again (keepActions). An entry Tela did not write, an installer's
// or a package's, is never edited: its actions are its own, and
// LinuxSetActions returns an error wrapping platform.ErrAppNotRegistered.

const (
	actionsKey        = "Actions"
	actionGroupPrefix = "Desktop Action "
	actionIDPrefix    = "tela-"
)

// LinuxSetActions sets the actions of the app's desktop entry to tasks;
// icons[i], if any, is the absolute path of task i's icon file. It
// reports whether it wrote anything.
func LinuxSetActions(dirs LinuxDirs, appID string, tasks []platform.Task, icons []string) (bool, error) {
	platform.CheckAppID("SetTasks", appID)
	platform.CheckTasks("SetTasks", tasks)
	l := &linuxAssoc{dirs: dirs}
	if err := l.check(); err != nil {
		return false, err
	}
	linuxMu.Lock()
	defer linuxMu.Unlock()
	own := l.scanEntries()[appID+".desktop"]
	switch {
	case own == nil || own.hide:
		if len(tasks) == 0 {
			return false, nil
		}
		return false, fmt.Errorf("assoc: SetTasks: %w: no desktop entry %s.desktop (desktop.RegisterApp writes the app's)", platform.ErrAppNotRegistered, appID)
	case !own.tela:
		if len(tasks) == 0 {
			return false, nil
		}
		return false, fmt.Errorf("assoc: SetTasks: %w: the desktop entry %s is not Tela's (an installer's or a package's), so its actions are not Tela's to change", platform.ErrAppNotRegistered, own.path)
	}
	exec, _ := own.entry.get(entryGroup, "Exec")
	exec = withFieldCode(exec, "")
	kf := parseKeyFile(own.raw)
	dropActions(kf)
	if len(tasks) > 0 {
		ids := make([]string, len(tasks))
		for i := range tasks {
			ids[i] = actionIDPrefix + strconv.Itoa(i+1)
		}
		kf.set(entryGroup, actionsKey, joinList(ids))
		for i, t := range tasks {
			cmd := exec
			for _, a := range t.Args {
				cmd += " " + escapeValue(quoteExecArg(a))
			}
			kf.lines = append(kf.lines, "", "["+actionGroupPrefix+ids[i]+"]",
				"Name="+escapeValue(t.Title), "Exec="+cmd)
			if i < len(icons) && icons[i] != "" {
				kf.lines = append(kf.lines, "Icon="+escapeValue(icons[i]))
			}
		}
	}
	data := kf.bytes()
	if bytes.Equal(data, own.raw) {
		return false, nil
	}
	return l.apply([]change{{own.path, data}})
}

// dropActions removes the Actions key and every action group of kf, with
// the blank lines before them.
func dropActions(kf *keyFile) {
	kf.del(entryGroup, actionsKey)
	var out []string
	skip := false
	for _, l := range kf.lines {
		if g, ok := groupOf(l); ok {
			skip = strings.HasPrefix(g, actionGroupPrefix)
			if skip {
				for len(out) > 0 && strings.TrimSpace(out[len(out)-1]) == "" {
					out = out[:len(out)-1]
				}
			}
		}
		if !skip {
			out = append(out, l)
		}
	}
	kf.lines = out
}

// actionLines returns the Actions value and the action groups' lines of
// an entry, for keepActions; "" and nil when it has none.
func actionLines(e *desktopEntry) (string, []string) {
	if e == nil || e.entry == nil {
		return "", nil
	}
	v, ok := e.entry.get(entryGroup, actionsKey)
	if !ok {
		return "", nil
	}
	var lines []string
	in := false
	for _, l := range e.entry.lines {
		if g, ok := groupOf(l); ok {
			in = strings.HasPrefix(g, actionGroupPrefix)
			if in {
				lines = append(lines, "")
			}
		}
		if in {
			lines = append(lines, l)
		}
	}
	return v, lines
}

// keepActions returns data, an entry rendered again, with the actions of
// old, the entry it replaces (SetTasks's, which RegisterApp and the
// associations must not drop).
func keepActions(data []byte, old *desktopEntry) []byte {
	v, lines := actionLines(old)
	if v == "" {
		return data
	}
	kf := parseKeyFile(data)
	kf.set(entryGroup, actionsKey, v)
	kf.lines = append(kf.lines, lines...)
	return kf.bytes()
}
