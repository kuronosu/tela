package headless

import (
	"slices"

	"github.com/kuronosu/tela/internal/platform"
)

// --- the desktop surfaces (platform contract v16, ADR-298) ---
//
// Start at login, the tasks and the recent documents are recorded per
// app ID, as a desktop keeps them for the user: nothing is written to
// the system. Each group can be made unsupported (ServiceStartAtLogin,
// ServiceTasks, ServiceRecentDocuments) or fail (SetSurfaceError).

// surfacesState is the fake surfaces, guarded by b.mu.
type surfacesState struct {
	atLogin map[string]bool            // by app ID: SetStartAtLogin's, or the user's (SetStartsAtLogin)
	tasks   map[string][]platform.Task // by app ID
	recent  map[string][]string        // by app ID, most recent first
	err     error                      // SetSurfaceError
}

// SetSurfaceError makes every call of the desktop surfaces fail with err
// (after checking its arguments), with nothing recorded, until
// SetSurfaceError(nil): a registry the user may not write, a desktop entry
// that cannot be rewritten. Safe from any goroutine.
func (b *Backend) SetSurfaceError(err error) {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.desktop.surfaces.err = err
}

// SetStartsAtLogin plays the user switching the app's start at login in
// the OS's settings (Task Manager's Startup apps, System Settings' Login
// Items): StartsAtLogin reports on from then on. Safe from any goroutine.
func (b *Backend) SetStartsAtLogin(appID string, on bool) {
	platform.CheckAppID("SetStartsAtLogin", appID)
	b.mu.Lock()
	defer b.mu.Unlock()
	s := &b.desktop.surfaces
	if s.atLogin == nil {
		s.atLogin = map[string]bool{}
	}
	s.atLogin[appID] = on
}

// StartAtLogin reports whether the desktop starts the app appID at
// login (SetStartAtLogin, or SetStartsAtLogin). Safe from any goroutine.
func (b *Backend) StartAtLogin(appID string) bool {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.desktop.surfaces.atLogin[appID]
}

// Tasks returns a copy of the tasks the app appID set (SetTasks), nil
// without any. Safe from any goroutine.
func (b *Backend) Tasks(appID string) []platform.Task {
	b.mu.Lock()
	defer b.mu.Unlock()
	return platform.CloneTasks(b.desktop.surfaces.tasks[appID])
}

// RecentDocuments returns the recent documents of the app appID, most
// recent first. Safe from any goroutine.
func (b *Backend) RecentDocuments(appID string) []string {
	b.mu.Lock()
	defer b.mu.Unlock()
	return slices.Clone(b.desktop.surfaces.recent[appID])
}

// surfaceCall checks that the service is there and not failing.
func (d desktop) surfaceCall(s Service) error {
	if !d.supported(s) {
		return platform.ErrUnsupported
	}
	d.b.mu.Lock()
	defer d.b.mu.Unlock()
	return d.b.desktop.surfaces.err
}

// SetStartAtLogin records the app's start at login.
func (d desktop) SetStartAtLogin(app platform.HandlerApp, on bool) error {
	platform.CheckHandlerApp("SetStartAtLogin", app, on)
	if err := d.surfaceCall(ServiceStartAtLogin); err != nil {
		return err
	}
	d.b.SetStartsAtLogin(app.ID, on)
	return nil
}

// StartsAtLogin reports what was recorded.
func (d desktop) StartsAtLogin(app platform.HandlerApp) (bool, error) {
	platform.CheckHandlerApp("StartsAtLogin", app, false)
	if err := d.surfaceCall(ServiceStartAtLogin); err != nil {
		return false, err
	}
	return d.b.StartAtLogin(app.ID), nil
}

// SetTasks records the app's tasks.
func (d desktop) SetTasks(app platform.HandlerApp, tasks []platform.Task) error {
	platform.CheckHandlerApp("SetTasks", app, true)
	platform.CheckTasks("SetTasks", tasks)
	if err := d.surfaceCall(ServiceTasks); err != nil {
		return err
	}
	d.b.mu.Lock()
	defer d.b.mu.Unlock()
	s := &d.b.desktop.surfaces
	if s.tasks == nil {
		s.tasks = map[string][]platform.Task{}
	}
	if len(tasks) == 0 {
		delete(s.tasks, app.ID)
	} else {
		s.tasks[app.ID] = platform.CloneTasks(tasks)
	}
	return nil
}

// AddRecentDocument checks the file like every backend and records it
// first in the app's list (moved there when it was in it already).
func (d desktop) AddRecentDocument(app platform.HandlerApp, path string) error {
	platform.CheckHandlerApp("AddRecentDocument", app, true)
	if _, err := platform.CheckRecentPath(path); err != nil {
		return err
	}
	if err := d.surfaceCall(ServiceRecentDocuments); err != nil {
		return err
	}
	clean, _, err := platform.CheckRecent(path)
	if err != nil {
		return err
	}
	d.b.mu.Lock()
	defer d.b.mu.Unlock()
	s := &d.b.desktop.surfaces
	if s.recent == nil {
		s.recent = map[string][]string{}
	}
	list := slices.DeleteFunc(s.recent[app.ID], func(p string) bool { return p == clean })
	s.recent[app.ID] = append([]string{clean}, list...)
	return nil
}

// ClearRecentDocuments empties the app's list.
func (d desktop) ClearRecentDocuments(app platform.HandlerApp) error {
	platform.CheckHandlerApp("ClearRecentDocuments", app, false)
	if err := d.surfaceCall(ServiceRecentDocuments); err != nil {
		return err
	}
	d.b.mu.Lock()
	defer d.b.mu.Unlock()
	delete(d.b.desktop.surfaces.recent, app.ID)
	return nil
}
