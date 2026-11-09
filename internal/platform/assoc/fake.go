package assoc

import (
	"slices"
	"strings"
	"sync"

	"github.com/kuronosu/tela/internal/platform"
)

// Fake is an in-memory platform.Associations that follows the contract's
// rules deterministically: Register makes the app a candidate and writes
// nothing when nothing changes, refuses what another application holds
// unless Replace, and leaves the default alone; a sole candidate with no
// explicit default is the default (as GIO and Windows pick it);
// MakeDefault sets the default, or asks the user for file types when
// AskForFileDefaults is set (Windows). A registration keeps its role
// (resolved): Register with another role changes it, and HandlerNone
// declares the type without making the app a handler (Status asked with
// HandlerNone reports the declaration). Tests seed other applications with
// SetDefault, SetConflict and SetAppEntry, inject failures with SetError,
// and inspect Registrations, Apps, Writes and Asked. The zero value is
// ready; it is safe for concurrent use. The headless backend gives each
// Backend one.
type Fake struct {
	mu sync.Mutex
	// askFiles: MakeDefault of a file type returns DefaultAsked.
	askFiles  bool
	regs      map[string]fakeReg // by app ID + "\x00" + key
	defaults  map[string]string  // key -> app ID or other app
	conflicts map[string]string  // key -> other app
	asked     []platform.Association
	apps      map[string]platform.HandlerApp // RegisterApp's, by app ID
	appOthers map[string]string              // app ID -> another's entry
	writes    int
	err       error
}

var _ platform.Associations = (*Fake)(nil)

// fakeReg is a registration: the app, and the role resolved
// (Association.ResolvedRole).
type fakeReg struct {
	app  platform.HandlerApp
	role platform.HandlerRole
}

// opens reports whether the registration makes the app a handler.
func (r fakeReg) opens() bool { return r.role != platform.HandlerNone }

// fakeKey identifies an association: the lower-cased scheme or
// extension.
func fakeKey(a platform.Association) string {
	if a.Scheme != "" {
		return strings.ToLower(a.Scheme) + ":"
	}
	return strings.ToLower(a.Extension)
}

// SetAskForFileDefaults makes MakeDefault of a file type behave as on
// Windows: it returns DefaultAsked and records the request (Asked)
// without changing the default; ChooseDefault then plays the user.
func (f *Fake) SetAskForFileDefaults(ask bool) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.askFiles = ask
}

// SetDefault makes holder (an application's ID, or this app's) the
// default handler of a; "" removes the explicit default. It plays another
// application or the user; it does not count as a write.
func (f *Fake) SetDefault(a platform.Association, holder string) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.defaults == nil {
		f.defaults = map[string]string{}
	}
	if holder == "" {
		delete(f.defaults, fakeKey(a))
	} else {
		f.defaults[fakeKey(a)] = holder
	}
}

// ChooseDefault plays the user picking app as the default in the
// system's settings after a DefaultAsked.
func (f *Fake) ChooseDefault(a platform.Association, appID string) { f.SetDefault(a, appID) }

// SetConflict makes holder own what Register would have to overwrite for
// a (a Windows scheme key, a desktop entry of the app's ID that Tela did
// not write, the extension's MIME mapping); "" removes it.
func (f *Fake) SetConflict(a platform.Association, holder string) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.conflicts == nil {
		f.conflicts = map[string]string{}
	}
	if holder == "" {
		delete(f.conflicts, fakeKey(a))
	} else {
		f.conflicts[fakeKey(a)] = holder
	}
}

// SetError makes every method return err (after checking its arguments)
// until SetError(nil): a read-only home, a registry access failure.
func (f *Fake) SetError(err error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.err = err
}

// Registration is an app registered for an association in a Fake.
type Registration struct {
	App         platform.HandlerApp
	Association string // "notes:" or ".note"
	// Role is the role registered, resolved: HandlerDefault is recorded
	// as HandlerEditor for a file type and HandlerViewer for a scheme.
	Role platform.HandlerRole
}

// Registrations returns the registrations, sorted by association then
// app ID.
func (f *Fake) Registrations() []Registration {
	f.mu.Lock()
	defer f.mu.Unlock()
	var out []Registration
	for k, r := range f.regs {
		_, key, _ := strings.Cut(k, "\x00")
		app := r.app
		app.Args = slices.Clone(app.Args)
		out = append(out, Registration{App: app, Association: key, Role: r.role})
	}
	slices.SortFunc(out, func(a, b Registration) int {
		if c := strings.Compare(a.Association, b.Association); c != 0 {
			return c
		}
		return strings.Compare(a.App.ID, b.App.ID)
	})
	return out
}

// Writes returns how many calls changed something (Register,
// Unregister, MakeDefault, RegisterApp, UnregisterApp), the system writes a
// real implementation would
// have made.
func (f *Fake) Writes() int {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.writes
}

// Asked returns the associations for which MakeDefault opened the
// system's settings (DefaultAsked), in order.
func (f *Fake) Asked() []platform.Association {
	f.mu.Lock()
	defer f.mu.Unlock()
	return slices.Clone(f.asked)
}

// status computes the status of app for key, asked with role (HandlerNone
// asks whether the declaration exists); f.mu must be held.
func (f *Fake) status(appID, key string, role platform.HandlerRole) platform.AssociationStatus {
	var s platform.AssociationStatus
	r, ok := f.regs[appID+"\x00"+key]
	s.Registered = ok && r.opens()
	if ok {
		s.Role = r.role // the fake keeps roles, as macOS does
	}
	declared := ok && !r.opens()
	def := f.defaults[key]
	switch {
	case def == appID:
		s.Default = !declared
	case def != "":
		s.Other = def
	default:
		// No explicit default: the sole candidate is the default (GIO
		// and Windows use the only handler there is).
		n := 0
		for k, r := range f.regs {
			if strings.HasSuffix(k, "\x00"+key) && r.opens() {
				n++
			}
		}
		s.Default = s.Registered && n == 1 && f.conflicts[key] == ""
	}
	if c := f.conflicts[key]; s.Other == "" && c != "" && c != appID {
		s.Other = c
	}
	if role == platform.HandlerNone {
		s.Registered, s.Default = ok, false
	}
	return s
}

func (f *Fake) Status(app platform.HandlerApp, a platform.Association) (platform.AssociationStatus, error) {
	platform.CheckHandlerApp("Associations.Status", app, false)
	platform.CheckAssociation("Associations.Status", a)
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.err != nil {
		return platform.AssociationStatus{}, f.err
	}
	return f.status(app.ID, fakeKey(a), a.Role), nil
}

func (f *Fake) Register(app platform.HandlerApp, a platform.Association, opts platform.RegisterOptions) (bool, error) {
	platform.CheckHandlerApp("Associations.Register", app, true)
	platform.CheckAssociation("Associations.Register", a)
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.err != nil {
		return false, f.err
	}
	return f.register(app, a, opts)
}

// register is Register; f.mu must be held.
func (f *Fake) register(app platform.HandlerApp, a platform.Association, opts platform.RegisterOptions) (bool, error) {
	key := fakeKey(a)
	role := a.ResolvedRole()
	if c := f.conflicts[key]; c != "" && c != app.ID {
		if !opts.Replace {
			return false, platform.TakenError(a, c)
		}
		delete(f.conflicts, key)
	} else if old, ok := f.regs[app.ID+"\x00"+key]; ok && sameApp(old.app, app) && old.role == role {
		return false, nil // looked before writing: nothing changes
	}
	if f.regs == nil {
		f.regs = map[string]fakeReg{}
	}
	app.Args = slices.Clone(app.Args)
	f.regs[app.ID+"\x00"+key] = fakeReg{app: app, role: role}
	if role == platform.HandlerNone && f.defaults[key] == app.ID {
		// The app stops being a handler: a default naming it goes, as
		// Unregister removes it.
		delete(f.defaults, key)
	}
	f.writes++
	return true, nil
}

func sameApp(a, b platform.HandlerApp) bool {
	return a.ID == b.ID && a.Name == b.Name && a.Exec == b.Exec && a.Icon == b.Icon && slices.Equal(a.Args, b.Args)
}

func (f *Fake) Unregister(app platform.HandlerApp, a platform.Association) (bool, error) {
	platform.CheckHandlerApp("Associations.Unregister", app, false)
	platform.CheckAssociation("Associations.Unregister", a)
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.err != nil {
		return false, f.err
	}
	key := fakeKey(a)
	changed := false
	if _, ok := f.regs[app.ID+"\x00"+key]; ok {
		delete(f.regs, app.ID+"\x00"+key)
		changed = true
	}
	if f.defaults[key] == app.ID {
		delete(f.defaults, key)
		changed = true
	}
	if changed {
		f.writes++
	}
	return changed, nil
}

func (f *Fake) MakeDefault(app platform.HandlerApp, a platform.Association) (platform.DefaultResult, error) {
	platform.CheckHandlerApp("Associations.MakeDefault", app, true)
	platform.CheckMakeDefault("Associations.MakeDefault", a)
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.err != nil {
		return platform.DefaultNone, f.err
	}
	if _, err := f.register(app, a, platform.RegisterOptions{}); err != nil {
		return platform.DefaultNone, err
	}
	if a.Extension != "" && f.askFiles {
		f.asked = append(f.asked, a)
		return platform.DefaultAsked, nil
	}
	key := fakeKey(a)
	if f.defaults[key] != app.ID {
		if f.defaults == nil {
			f.defaults = map[string]string{}
		}
		f.defaults[key] = app.ID
		f.writes++
	}
	return platform.DefaultSet, nil
}
