package assoc

import (
	"errors"
	"fmt"
	"strings"
	"sync"

	"github.com/kuronosu/tela/internal/platform"
)

// The Windows associations, written against a Registry so that the tests
// run on every OS with a MemRegistry. What is written, with ADR-209's
// conditions (per user, never administrator rights, look before writing,
// nothing overwritten without Replace):
//
// A URL scheme is the key Classes\<scheme>: the "URL Protocol" value, a
// default value "URL:<Name>", DefaultIcon and shell\open\command, plus two
// values that say whose it is ("Tela AppID", "Tela Exec": the app and the
// executable the command was written for; a key whose command now names
// another executable was taken over, and is not the app's). The scheme
// name is also the ProgID the app's capabilities name for it, which is
// what the Default apps settings write to UserChoice when the user picks
// the app.
//
// A file type is the ProgID Classes\<AppID>.<ext without the dot>
// (description, DefaultIcon, shell\open\command) and the value
// Classes\.<ext>\OpenWithProgids\<ProgID>, which lists the app in "Open
// with". The extension's default value, the fallback Windows opens a file
// with when the user chose nothing, is set to the ProgID only when no
// per-user and no machine-wide registration names one; another ProgID there
// is replaced only with Replace.
//
// Both are also listed in Software\<AppID>\Capabilities (URLAssociations,
// FileAssociations) and Software\RegisteredApplications, which is what makes
// the Default apps page offer the app.
//
// The user's choice is where Windows keeps it, and is only read: UserChoice
// is protected by a hash that only the settings page can make.
//
// Roles (platform.HandlerRole): Windows has none. HandlerEditor,
// HandlerViewer and HandlerShell write the open verb above, and no "edit"
// verb: the shell runs a ProgID's default verb (open) on a double click and
// in "Open with", "edit" only adds a second context menu item running the
// same command (the app could not tell them apart from its arguments), and
// Windows' own handlers write "edit" only for types with a separate editor
// (scripts, images). A file type with HandlerNone is declared, not handled:
// the ProgID keeps its description and icon (and the extension's default
// value names it, with the rule above, so Explorer shows the type's name
// and icon), but it has no shell key, the extension's OpenWithProgids does
// not list it, and the capabilities do not name it; a user's choice naming
// it is removed, as Unregister does.

const (
	classesKey        = `Software\Classes`
	registeredAppsKey = `Software\RegisteredApplications`
	urlUserChoiceKey  = `Software\Microsoft\Windows\Shell\Associations\UrlAssociations`
	fileUserChoiceKey = `Software\Microsoft\Windows\CurrentVersion\Explorer\FileExts`

	// Values in a scheme key that say it was written for an app.
	markerAppID = "Tela AppID"
	markerExec  = "Tela Exec"

	defaultAppsURI = "ms-settings:defaultapps"
)

type windows struct {
	mu  sync.Mutex // one change at a time: read, then write
	reg Registry
}

var _ platform.Associations = (*windows)(nil)

// tx is one call's access to the registry: reads, writes that look first
// and count what they changed, and the first error (after which nothing
// else is read or written).
type tx struct {
	reg     Registry
	err     error
	changed bool
}

func (t *tx) fail(what string, err error) {
	if t.err == nil && err != nil {
		t.err = fmt.Errorf("assoc: %s: %w", what, err)
	}
}

// get returns a string value; ok is false when it does not exist.
func (t *tx) get(key, name string) (string, bool) {
	if t.err != nil {
		return "", false
	}
	v, ok, err := t.reg.String(key, name)
	t.fail("reading "+key, err)
	return v, ok && err == nil
}

// machine returns a string value of HKEY_LOCAL_MACHINE, "" when there is no
// way to read it.
func (t *tx) machine(key, name string) string {
	m, ok := t.reg.(MachineReader)
	if !ok || t.err != nil {
		return ""
	}
	v, found, err := m.MachineString(key, name)
	t.fail("reading HKLM\\"+key, err)
	if !found {
		return ""
	}
	return v
}

// set writes a string value unless it already has that value.
func (t *tx) set(key, name, value string) {
	if t.err != nil {
		return
	}
	if cur, ok := t.get(key, name); ok && cur == value {
		return
	}
	if t.err == nil {
		t.fail("writing "+key, t.reg.SetString(key, name, value))
		t.changed = t.changed || t.err == nil
	}
}

// hasValue reports whether key has a value called name, of any type.
func (t *tx) hasValue(key, name string) bool {
	if t.err != nil {
		return false
	}
	names, err := t.reg.ValueNames(key)
	t.fail("reading "+key, err)
	for _, n := range names {
		if strings.EqualFold(n, name) {
			return true
		}
	}
	return false
}

// ensureValue creates an empty value that exists only by its name (the
// entries of OpenWithProgids), unless the key has it already, of whatever
// type.
func (t *tx) ensureValue(key, name string) {
	if !t.hasValue(key, name) {
		t.set(key, name, "")
	}
}

// deleteValue removes a value if it exists.
func (t *tx) deleteValue(key, name string) {
	if t.hasValue(key, name) {
		t.fail("deleting a value of "+key, t.reg.DeleteValue(key, name))
		t.changed = t.changed || t.err == nil
	}
}

// empty reports whether key has no values and no subkeys (or does not
// exist).
func (t *tx) empty(key string) bool {
	if t.err != nil {
		return false
	}
	names, err := t.reg.ValueNames(key)
	t.fail("reading "+key, err)
	subs, err2 := t.reg.SubKeys(key)
	t.fail("reading "+key, err2)
	return len(names) == 0 && len(subs) == 0
}

// exists reports whether key has any value or subkey.
func (t *tx) exists(key string) bool { return !t.empty(key) && t.err == nil }

// deleteKey removes key and everything below it; it counts as a change when
// there was something.
func (t *tx) deleteKey(key string) {
	if t.exists(key) {
		t.fail("deleting "+key, t.reg.DeleteKey(key))
		t.changed = t.changed || t.err == nil
	}
}

// deleteIfEmpty removes key if nothing is left in it.
func (t *tx) deleteIfEmpty(key string) {
	if t.err == nil && t.empty(key) {
		t.fail("deleting "+key, t.reg.DeleteKey(key))
	}
}

// finish tells the shell about the changes of a call that did not fail and
// returns its error.
func (w *windows) finish(t *tx) error {
	if t.err == nil && t.changed {
		if s, ok := w.reg.(Shell); ok {
			s.AssociationsChanged()
		}
	}
	return t.err
}

// --- names and commands ---

func schemeKey(a platform.Association) string { return classesKey + `\` + a.Scheme }
func extKey(a platform.Association) string    { return classesKey + `\` + a.Extension }

// progID is the ProgID of the app's handler of a file type: "<AppID>.note"
// for ".note".
func progID(app platform.HandlerApp, a platform.Association) string {
	return app.ID + "." + strings.TrimPrefix(a.Extension, ".")
}

// assocID is the name the association has in the app's capabilities, and
// the ProgID that UserChoice names for it: the scheme (whose key handles
// it) or the file type's ProgID.
func assocID(app platform.HandlerApp, a platform.Association) string {
	if a.Scheme != "" {
		return a.Scheme
	}
	return progID(app, a)
}

func userChoiceKey(a platform.Association) string {
	if a.Scheme != "" {
		return urlUserChoiceKey + `\` + a.Scheme + `\UserChoice`
	}
	return fileUserChoiceKey + `\` + a.Extension + `\UserChoice`
}

func capabilitiesKey(app platform.HandlerApp) string { return `Software\` + app.ID + `\Capabilities` }

// capabilityList is the capabilities subkey that lists a.
func capabilityList(app platform.HandlerApp, a platform.Association) string {
	if a.Scheme != "" {
		return capabilitiesKey(app) + `\URLAssociations`
	}
	return capabilitiesKey(app) + `\FileAssociations`
}

// capabilityName is the value name of a in its capabilities list.
func capabilityName(a platform.Association) string {
	if a.Scheme != "" {
		return a.Scheme
	}
	return a.Extension
}

// windowsCommand is the command line of the handler: the executable
// (always quoted), the app's arguments and the "%1" that the shell replaces
// with the URI or the path.
func windowsCommand(app platform.HandlerApp) string {
	var b strings.Builder
	b.WriteString(`"` + app.Exec + `"`)
	for _, a := range app.Args {
		b.WriteByte(' ')
		b.WriteString(quoteArg(a))
	}
	b.WriteString(` "%1"`)
	return b.String()
}

// quoteArg quotes an argument for a Windows command line so that
// CommandLineToArgvW reads it back: only an empty argument or one with a
// space, tab or quote needs quotes; backslashes count only before a quote.
func quoteArg(s string) string {
	if s != "" && !strings.ContainsAny(s, " \t\"") {
		return s
	}
	var b strings.Builder
	b.WriteByte('"')
	slashes := 0
	for _, r := range s {
		switch r {
		case '\\':
			slashes++
			b.WriteRune(r)
			continue
		case '"':
			b.WriteString(strings.Repeat(`\`, slashes+1))
		}
		slashes = 0
		b.WriteRune(r)
	}
	b.WriteString(strings.Repeat(`\`, slashes)) // before the closing quote
	b.WriteByte('"')
	return b.String()
}

// commandExe returns the executable a command line starts with.
func commandExe(cmd string) string {
	cmd = strings.TrimSpace(cmd)
	if rest, ok := strings.CutPrefix(cmd, `"`); ok {
		exe, _, _ := strings.Cut(rest, `"`)
		return exe
	}
	exe, _, _ := strings.Cut(cmd, " ")
	return exe
}

// sameExe compares two executable paths as Windows does: without regard to
// case or to the direction of the slashes.
func sameExe(a, b string) bool {
	return a != "" && strings.EqualFold(strings.ReplaceAll(a, "/", `\`), strings.ReplaceAll(b, "/", `\`))
}

// iconValue is the DefaultIcon of the app's handlers.
func iconValue(app platform.HandlerApp) string {
	if app.Icon != "" {
		return app.Icon
	}
	return `"` + app.Exec + `",0`
}

// --- who holds what ---

// schemeOwner inspects the key of a scheme. ours: the app wrote it, or its
// command is the app's executable (exec, when known). other: the
// executable of whoever else holds the scheme, in the user's key or, when
// there is none, the machine's; "" when nobody does.
func (t *tx) schemeOwner(app platform.HandlerApp, a platform.Association, exec string) (ours bool, other string) {
	key := schemeKey(a)
	if cmd, ok := t.get(key+`\shell\open\command`, ""); ok && strings.TrimSpace(cmd) != "" {
		cur := commandExe(cmd)
		id, _ := t.get(key, markerAppID)
		wrote, _ := t.get(key, markerExec)
		switch {
		case strings.EqualFold(id, app.ID) && sameExe(cur, wrote), sameExe(cur, exec):
			return true, ""
		case cur == "":
			return false, strings.TrimSpace(cmd)
		}
		return false, cur
	}
	if cmd := t.machine(key+`\shell\open\command`, ""); strings.TrimSpace(cmd) != "" {
		if cur := commandExe(cmd); !sameExe(cur, exec) {
			return false, cur
		}
	}
	return false, ""
}

// userChoice returns the ProgID the user chose for a, "" when they chose
// nothing.
func (t *tx) userChoice(a platform.Association) string {
	v, _ := t.get(userChoiceKey(a), "ProgId")
	return v
}

// status computes the status of the app for a. exec is the app's
// executable when the caller knows it.
func (t *tx) status(app platform.HandlerApp, a platform.Association, exec string) platform.AssociationStatus {
	var s platform.AssociationStatus
	uc := t.userChoice(a)
	id := assocID(app, a)
	if a.Scheme != "" {
		ours, other := t.schemeOwner(app, a, exec)
		s.Registered = ours
		if uc != "" && !strings.EqualFold(uc, id) {
			s.Other = uc
		} else {
			// The user chose the app, or nothing: the scheme's own key is
			// what Windows uses.
			s.Default = ours
		}
		if s.Other == "" {
			s.Other = other
		}
		return s
	}
	s.Registered = t.hasValue(extKey(a)+`\OpenWithProgids`, id)
	if pkey := classesKey + `\` + id; !s.Registered && t.exists(pkey) && !t.exists(pkey+`\shell\open\command`) {
		// Declared, not handled (ADR-259); a handler's role is not kept.
		s.Role = platform.HandlerNone
	}
	cur, _ := t.get(extKey(a), "")
	if cur == "" {
		cur = t.machine(extKey(a), "")
	}
	switch {
	case uc != "" && strings.EqualFold(uc, id):
		s.Default = s.Registered
	case uc != "":
		s.Other = uc
	case strings.EqualFold(cur, id):
		s.Default = s.Registered
	default:
		s.Other = cur
	}
	if a.Role == platform.HandlerNone {
		// A declaration: the ProgID exists, whatever else names the app.
		s.Registered, s.Default = t.exists(classesKey+`\`+id), false
	}
	return s
}

// --- Associations ---

func (w *windows) Status(app platform.HandlerApp, a platform.Association) (platform.AssociationStatus, error) {
	platform.CheckHandlerApp("Associations.Status", app, false)
	platform.CheckAssociation("Associations.Status", a)
	w.mu.Lock()
	defer w.mu.Unlock()
	t := &tx{reg: w.reg}
	s := t.status(app, a, "")
	return s, t.err
}

func (w *windows) Register(app platform.HandlerApp, a platform.Association, opts platform.RegisterOptions) (bool, error) {
	platform.CheckHandlerApp("Associations.Register", app, true)
	platform.CheckAssociation("Associations.Register", a)
	w.mu.Lock()
	defer w.mu.Unlock()
	t := &tx{reg: w.reg}
	w.register(t, app, a, opts)
	err := w.finish(t)
	return t.changed && err == nil, err
}

// register is Register on t.
func (w *windows) register(t *tx, app platform.HandlerApp, a platform.Association, opts platform.RegisterOptions) {
	cmd := windowsCommand(app)
	if a.Scheme != "" {
		key := schemeKey(a)
		if ours, other := t.schemeOwner(app, a, app.Exec); t.err == nil && !ours && other != "" && !opts.Replace {
			t.err = platform.TakenError(a, other)
			return
		}
		t.set(key, "", "URL:"+app.Name)
		t.set(key, "URL Protocol", "")
		t.set(key, markerAppID, app.ID)
		t.set(key, markerExec, app.Exec)
		t.set(key+`\DefaultIcon`, "", iconValue(app))
		t.set(key+`\shell\open\command`, "", cmd) // last: the key works only when complete
	} else {
		id := progID(app, a)
		pkey := classesKey + `\` + id
		ext := extKey(a)
		t.set(pkey, "", a.Description)
		t.set(pkey+`\DefaultIcon`, "", iconValue(app))
		if a.Role == platform.HandlerNone {
			// Declared, not handled: the ProgID names and draws the type,
			// and nothing lets the shell open a file with the app.
			named := strings.EqualFold(t.userChoice(a), id)
			t.deleteKey(pkey + `\shell`)
			t.deleteValue(ext+`\OpenWithProgids`, id)
			t.deleteIfEmpty(ext + `\OpenWithProgids`)
			if named {
				w.dropUserChoice(t, a)
			}
		} else {
			t.set(pkey+`\shell\open\command`, "", cmd)
			t.ensureValue(ext+`\OpenWithProgids`, id)
		}
		cur, _ := t.get(ext, "")
		if cur == "" {
			cur = t.machine(ext, "")
		}
		if cur == "" || (opts.Replace && !strings.EqualFold(cur, id)) {
			// Nothing else opens the type (or the caller said to take it
			// over): the fallback handler is the app's (with HandlerNone,
			// the ProgID that gives the type its name and icon).
			t.set(ext, "", id)
		}
		if a.Role == platform.HandlerNone {
			w.dropCapability(t, app, a)
			return
		}
	}
	// What the Default apps page lists the app by.
	caps := capabilitiesKey(app)
	t.set(caps, "ApplicationName", app.Name)
	t.set(caps, "ApplicationDescription", app.Name)
	t.set(capabilityList(app, a), capabilityName(a), assocID(app, a))
	t.set(registeredAppsKey, app.ID, caps)
}

func (w *windows) Unregister(app platform.HandlerApp, a platform.Association) (bool, error) {
	platform.CheckHandlerApp("Associations.Unregister", app, false)
	platform.CheckAssociation("Associations.Unregister", a)
	w.mu.Lock()
	defer w.mu.Unlock()
	t := &tx{reg: w.reg}
	id := assocID(app, a)
	named := strings.EqualFold(t.userChoice(a), id) // the user's choice names the app
	removed := false
	if a.Scheme != "" {
		if ours, _ := t.schemeOwner(app, a, ""); ours {
			t.deleteKey(schemeKey(a))
			removed = true
		}
	} else {
		ext := extKey(a)
		removed = t.hasValue(ext+`\OpenWithProgids`, id) || t.exists(classesKey+`\`+id)
		t.deleteValue(ext+`\OpenWithProgids`, id)
		if cur, ok := t.get(ext, ""); ok && strings.EqualFold(cur, id) {
			t.deleteValue(ext, "")
		}
		t.deleteKey(classesKey + `\` + id)
		t.deleteIfEmpty(ext + `\OpenWithProgids`)
		t.deleteIfEmpty(ext)
	}
	if named && removed {
		w.dropUserChoice(t, a)
	}
	w.dropCapability(t, app, a)
	err := w.finish(t)
	return t.changed && err == nil, err
}

// dropUserChoice removes the user's choice of a handler for a, which named
// the app's handler that is going away. Best effort: the settings page
// protects UserChoice, and a stale choice of a removed handler is
// harmless.
func (w *windows) dropUserChoice(t *tx, a platform.Association) {
	if t.err == nil && t.exists(userChoiceKey(a)) && w.reg.DeleteKey(userChoiceKey(a)) == nil {
		t.changed = true
	}
}

// dropCapability removes a from the app's capabilities, and the
// capabilities themselves, the registered application and the app's key
// when nothing else is in them: the app's own settings below
// Software\<AppID> are left alone.
func (w *windows) dropCapability(t *tx, app platform.HandlerApp, a platform.Association) {
	list := capabilityList(app, a)
	t.deleteValue(list, capabilityName(a))
	t.deleteIfEmpty(list)
	caps := capabilitiesKey(app)
	subs, err := w.reg.SubKeys(caps)
	t.fail("reading "+caps, err)
	if t.err != nil || len(subs) > 0 {
		return
	}
	t.deleteKey(caps)
	if v, ok := t.get(registeredAppsKey, app.ID); ok && strings.EqualFold(v, caps) {
		t.deleteValue(registeredAppsKey, app.ID)
	}
	t.deleteIfEmpty(`Software\` + app.ID)
}

func (w *windows) MakeDefault(app platform.HandlerApp, a platform.Association) (platform.DefaultResult, error) {
	platform.CheckHandlerApp("Associations.MakeDefault", app, true)
	platform.CheckMakeDefault("Associations.MakeDefault", a)
	w.mu.Lock()
	defer w.mu.Unlock()
	t := &tx{reg: w.reg}
	w.register(t, app, a, platform.RegisterOptions{})
	if err := w.finish(t); err != nil {
		return platform.DefaultNone, err
	}
	st := t.status(app, a, app.Exec)
	if t.err != nil {
		return platform.DefaultNone, t.err
	}
	if st.Default {
		// A scheme nobody overrides: the app's own key is what Windows
		// uses. Or the user already chose the app.
		return platform.DefaultSet, nil
	}
	// The user's choice is protected: only the settings page may change it.
	s, ok := w.reg.(Shell)
	if !ok {
		return platform.DefaultNone, errors.New("assoc: MakeDefault: cannot open the Default apps settings page with this registry")
	}
	if err := s.OpenSettings(defaultAppsURI); err != nil {
		return platform.DefaultNone, fmt.Errorf("assoc: MakeDefault: opening %s: %w", defaultAppsURI, err)
	}
	return platform.DefaultAsked, nil
}

// --- RegisterApp, UnregisterApp, AppStatus (ADR-217) ---
//
// Windows identifies a desktop app by its executable: RegisterHotKey,
// the notification area and the single-instance mutex need no entry, so
// RegisterApp writes nothing (the Start menu shortcut that makes an app
// "installed" is an installer's, from packaging) and AppStatus reports the app
// registered. The registry is not even read.

func (w *windows) RegisterApp(app platform.HandlerApp, _ platform.RegisterOptions) (bool, error) {
	platform.CheckHandlerApp("Associations.RegisterApp", app, true)
	return false, nil
}

func (w *windows) UnregisterApp(app platform.HandlerApp) (bool, error) {
	platform.CheckHandlerApp("Associations.UnregisterApp", app, false)
	return false, nil
}

func (w *windows) AppStatus(app platform.HandlerApp) (platform.AppStatus, error) {
	platform.CheckHandlerApp("Associations.AppStatus", app, false)
	return platform.AppStatus{Registered: true}, nil
}
