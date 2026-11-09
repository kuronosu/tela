package core

import (
	"fmt"
	"sort"
	"strings"
	"sync"
	"sync/atomic"

	"github.com/kuronosu/tela/internal/platform"
)

// Built-in labels (ADR-251): the texts Tela itself draws or hands the OS,
// each with a fixed ID and an English default. A LabelSource (package
// i18n installs one) may answer an ID with the app's text in its current
// language; the platform backends get the answers as plain strings
// (platform.Desktop.SetStrings).

// labelDefaults are the IDs and their English texts. A {name} in a text is
// an argument ("About {name}").
var labelDefaults = map[string]string{
	// The fallback title bar (chrome.go).
	"tela.titlebar.minimize": "Minimize",
	"tela.titlebar.maximize": "Maximize",
	"tela.titlebar.restore":  "Restore",
	"tela.titlebar.close":    "Close",
	"tela.titlebar.controls": "Window controls",

	// Message boxes, drawn and native (desktop.ShowMessage).
	"tela.messagebox.ok":       "OK",
	"tela.messagebox.cancel":   "Cancel",
	"tela.messagebox.yes":      "Yes",
	"tela.messagebox.no":       "No",
	"tela.messagebox.info":     "Information",
	"tela.messagebox.warning":  "Warning",
	"tela.messagebox.error":    "Error",
	"tela.messagebox.question": "Question",

	// kit's toast viewport and its close button.
	"tela.toast.region": "Notifications",
	"tela.toast.close":  "Close toast",

	// Menus: the labels of role items (desktop.Role.Label) and what the
	// macOS backend adds to the app's menu bar.
	"tela.menu.about":           "About {name}",
	"tela.menu.services":        "Services",
	"tela.menu.hide":            "Hide {name}",
	"tela.menu.hideOthers":      "Hide Others",
	"tela.menu.showAll":         "Show All",
	"tela.menu.quit":            "Quit {name}",
	"tela.menu.edit":            "Edit",
	"tela.menu.window":          "Window",
	"tela.menu.help":            "Help",
	"tela.menu.undo":            "Undo",
	"tela.menu.redo":            "Redo",
	"tela.menu.cut":             "Cut",
	"tela.menu.copy":            "Copy",
	"tela.menu.paste":           "Paste",
	"tela.menu.selectAll":       "Select All",
	"tela.menu.minimize":        "Minimize",
	"tela.menu.zoom":            "Zoom",
	"tela.menu.close":           "Close",
	"tela.menu.bringAllToFront": "Bring All to Front",

	// The macOS save panel's file type popup.
	"tela.panel.format": "Format:",
}

// LabelSource answers a label ID with the text in the app's language, or
// false when it has none (the English default applies). args are name/value
// pairs for the {name} placeholders of the English text.
type LabelSource func(id string, args ...any) (string, bool)

var labels struct {
	src     atomic.Pointer[LabelSource]
	version atomic.Uint64

	mu   sync.Mutex
	subs map[uint64]func()
	seq  uint64
}

// SetLabelSource sets the source of the labels' texts; nil removes it.
// Call LabelsChanged when its answers change.
func SetLabelSource(src LabelSource) {
	if src == nil {
		labels.src.Store(nil)
	} else {
		labels.src.Store(&src)
	}
	LabelsChanged()
}

// LabelIDs returns the IDs of the built-in labels, sorted.
func LabelIDs() []string {
	ids := make([]string, 0, len(labelDefaults))
	for id := range labelDefaults {
		ids = append(ids, id)
	}
	sort.Strings(ids)
	return ids
}

// LabelDefault returns the English text of a label, with its {name}
// placeholders still in it; ok is false for an unknown ID.
func LabelDefault(id string) (string, bool) {
	s, ok := labelDefaults[id]
	return s, ok
}

// Label returns the text of a built-in label: the source's, else the
// English default, with args filled in ("name", "Notes"). An unknown ID
// panics, since it is a programmer's error.
func Label(id string, args ...any) string {
	def, ok := labelDefaults[id]
	if !ok {
		panic(fmt.Sprintf("tela: unknown label ID %q; the IDs are listed by tela.Labels, such as \"tela.toast.close\"", id))
	}
	if p := labels.src.Load(); p != nil {
		if s, ok := (*p)(id, args...); ok {
			return s
		}
	}
	return fill(def, args)
}

// LabelEnglish returns the English text of a label with args filled in,
// whatever the source says (package i18n falls back to it when its
// catalogs of a locale lack the ID). An unknown ID panics, as Label does.
func LabelEnglish(id string, args ...any) string {
	def, ok := labelDefaults[id]
	if !ok {
		panic(fmt.Sprintf("tela: unknown label ID %q; the IDs are listed by tela.Labels, such as \"tela.toast.close\"", id))
	}
	return fill(def, args)
}

// fill replaces {name} in s by the value of the argument pair "name".
func fill(s string, args []any) string {
	if !strings.Contains(s, "{") {
		return s
	}
	for i := 0; i+1 < len(args); i += 2 {
		if k, ok := args[i].(string); ok {
			s = strings.ReplaceAll(s, "{"+k+"}", fmt.Sprint(args[i+1]))
		}
	}
	return s
}

// LabelOverrides returns the texts the source gives, by ID: what the
// platform backends are told. The labels that take arguments are asked
// with each argument equal to its own placeholder ("name", "{name}"), so
// the backend fills them in (its own app name).
func LabelOverrides() map[string]string {
	p := labels.src.Load()
	if p == nil {
		return nil
	}
	out := map[string]string{}
	for id, def := range labelDefaults {
		var args []any
		for rest := def; ; {
			i := strings.IndexByte(rest, '{')
			if i < 0 {
				break
			}
			j := strings.IndexByte(rest[i:], '}')
			if j < 0 {
				break
			}
			name := rest[i+1 : i+j]
			args = append(args, name, "{"+name+"}")
			rest = rest[i+j+1:]
		}
		if s, ok := (*p)(id, args...); ok {
			out[id] = s
		}
	}
	return out
}

// LabelsChanged tells everything that shows built-in labels that the
// source's answers changed (a new locale, a catalog loaded): the
// components that called UseLabels build again, and the platform backends
// are given the texts again. It is safe from any goroutine.
func LabelsChanged() {
	labels.version.Add(1)
	labels.mu.Lock()
	subs := make([]func(), 0, len(labels.subs))
	for _, f := range labels.subs {
		subs = append(subs, f)
	}
	labels.mu.Unlock()
	for _, f := range subs {
		f()
	}
}

// OnLabelsChanged registers f to run when LabelsChanged does, on the
// caller's goroutine; the returned function unregisters it.
func OnLabelsChanged(f func()) (cancel func()) {
	labels.mu.Lock()
	defer labels.mu.Unlock()
	if labels.subs == nil {
		labels.subs = map[uint64]func(){}
	}
	labels.seq++
	id := labels.seq
	labels.subs[id] = f
	return func() {
		labels.mu.Lock()
		delete(labels.subs, id)
		labels.mu.Unlock()
	}
}

// UseLabels returns the function that gives the text of a built-in label
// ([Label]) and subscribes the component to the source: when
// [LabelsChanged] runs, the component builds again. It is a hook.
func UseLabels() func(id string, args ...any) string {
	_, set := UseState(labels.version.Load())
	UseEffect(func() func() {
		catchUp := func() {
			// Applies on the UI thread reading the version at that moment,
			// so changes that race end on the latest one.
			set.Update(func(uint64) uint64 { return labels.version.Load() })
		}
		cancel := OnLabelsChanged(catchUp)
		catchUp() // a change between this build and the effect
		return cancel
	})
	return Label
}

// syncStrings gives the backend the texts it shows itself.
func (h *Host) syncStrings() {
	if h.ended {
		return
	}
	h.services.SetStrings(platform.Strings(LabelOverrides()))
}

// WatchStrings keeps the backend's strings current until the returned
// function is called.
func (h *Host) WatchStrings() func() {
	return OnLabelsChanged(func() { Post(h.syncStrings) })
}
