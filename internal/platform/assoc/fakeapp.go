package assoc

import (
	"slices"
	"strings"

	"github.com/kuronosu/tela/internal/platform"
)

// The Fake's RegisterApp, UnregisterApp and AppStatus (ADR-217).

// SetAppEntry plays an entry of the app ID appID that Tela did not write
// (a package's, an installer's), at path: RegisterApp then writes nothing
// unless Replace, and AppStatus reports it as Other. "" removes it.
func (f *Fake) SetAppEntry(appID, path string) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.appOthers == nil {
		f.appOthers = map[string]string{}
	}
	if path == "" {
		delete(f.appOthers, appID)
	} else {
		f.appOthers[appID] = path
	}
}

// Apps returns the apps RegisterApp registered and UnregisterApp did not
// remove, sorted by ID.
func (f *Fake) Apps() []platform.HandlerApp {
	f.mu.Lock()
	defer f.mu.Unlock()
	var out []platform.HandlerApp
	for _, app := range f.apps {
		app.Args = slices.Clone(app.Args)
		out = append(out, app)
	}
	slices.SortFunc(out, func(a, b platform.HandlerApp) int { return strings.Compare(a.ID, b.ID) })
	return out
}

func (f *Fake) RegisterApp(app platform.HandlerApp, opts platform.RegisterOptions) (bool, error) {
	platform.CheckHandlerApp("Associations.RegisterApp", app, true)
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.err != nil {
		return false, f.err
	}
	if f.appOthers[app.ID] != "" {
		if !opts.Replace {
			return false, nil // another entry makes the desktop know the app
		}
		delete(f.appOthers, app.ID)
	} else if old, ok := f.apps[app.ID]; ok && sameApp(old, app) {
		return false, nil // looked before writing
	}
	if f.apps == nil {
		f.apps = map[string]platform.HandlerApp{}
	}
	app.Args = slices.Clone(app.Args)
	f.apps[app.ID] = app
	f.writes++
	return true, nil
}

func (f *Fake) UnregisterApp(app platform.HandlerApp) (bool, error) {
	platform.CheckHandlerApp("Associations.UnregisterApp", app, false)
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.err != nil {
		return false, f.err
	}
	if _, ok := f.apps[app.ID]; !ok {
		return false, nil
	}
	delete(f.apps, app.ID)
	f.writes++
	return true, nil
}

func (f *Fake) AppStatus(app platform.HandlerApp) (platform.AppStatus, error) {
	platform.CheckHandlerApp("Associations.AppStatus", app, false)
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.err != nil {
		return platform.AppStatus{}, f.err
	}
	var st platform.AppStatus
	_, st.Requested = f.apps[app.ID]
	st.Other = f.appOthers[app.ID]
	st.Registered = st.Requested || st.Other != ""
	for k := range f.regs {
		// An association's entry makes the desktop know the app too (the
		// desktop entry Linux shares).
		if strings.HasPrefix(k, app.ID+"\x00") {
			st.Registered = true
		}
	}
	return st, nil
}
