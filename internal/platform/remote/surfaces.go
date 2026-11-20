package remote

import (
	"github.com/kuronosu/tela/internal/platform"
)

// The desktop surfaces of platform contract v16 (protocol 17, ADR-298):
// start at login, the app's tasks and its recent documents go to the
// host's backend, which owns the desktop. They are blocking calls (like
// the secrets and the trash), served off the host's UI thread, and carry
// the child's HandlerApp, so the host writes the app's ID and executable,
// not its own. A Dock menu task the host's Cocoa backend delivers is an
// app-scoped LaunchEvent, which goes to the active child like any launch.

// SetStartAtLogin asks the host's backend (see the top of the file).
func (d childDesktop) SetStartAtLogin(app platform.HandlerApp, on bool) error {
	platform.CheckHandlerApp("SetStartAtLogin", app, on)
	_, err := d.callFor(dSetStartAtLogin, 0, secretTimeout, func(e *enc) { e.handlerApp(app); e.boolean(on) })
	return err
}

// StartsAtLogin asks the host's backend.
func (d childDesktop) StartsAtLogin(app platform.HandlerApp) (bool, error) {
	platform.CheckHandlerApp("StartsAtLogin", app, false)
	r, err := d.callFor(dStartsAtLogin, 0, secretTimeout, func(e *enc) { e.handlerApp(app) })
	if err != nil {
		return false, err
	}
	on := r.boolean()
	return on, r.end()
}

// SetTasks asks the host's backend.
func (d childDesktop) SetTasks(app platform.HandlerApp, tasks []platform.Task) error {
	platform.CheckHandlerApp("SetTasks", app, true)
	platform.CheckTasks("SetTasks", tasks)
	_, err := d.callFor(dSetTasks, 0, secretTimeout, func(e *enc) { e.handlerApp(app); e.tasks(tasks) })
	return err
}

// AddRecentDocument asks the host's backend (the same machine and file
// system).
func (d childDesktop) AddRecentDocument(app platform.HandlerApp, path string) error {
	platform.CheckHandlerApp("AddRecentDocument", app, true)
	clean, err := platform.CheckRecentPath(path)
	if err != nil {
		return err
	}
	_, err = d.callFor(dAddRecentDocument, 0, secretTimeout, func(e *enc) { e.handlerApp(app); e.str(clean) })
	return err
}

// ClearRecentDocuments asks the host's backend.
func (d childDesktop) ClearRecentDocuments(app platform.HandlerApp) error {
	platform.CheckHandlerApp("ClearRecentDocuments", app, false)
	_, err := d.callFor(dClearRecentDocuments, 0, secretTimeout, func(e *enc) { e.handlerApp(app) })
	return err
}

// serveSurface serves a surface call on the host, off its UI thread
// (serveBlocking); handled is false for another operation.
func (h *Host) serveSurface(op uint8, d *dec, result *func(*enc)) (handled bool, err error) {
	desk := h.desk
	switch op {
	case dSetStartAtLogin:
		app, on := d.handlerApp(), d.boolean()
		if err := d.end(); err != nil {
			return true, err
		}
		return true, desk.SetStartAtLogin(app, on)
	case dStartsAtLogin:
		app := d.handlerApp()
		if err := d.end(); err != nil {
			return true, err
		}
		on, err := desk.StartsAtLogin(app)
		*result = func(e *enc) { e.boolean(on) }
		return true, err
	case dSetTasks:
		app, tasks := d.handlerApp(), d.tasks()
		if err := d.end(); err != nil {
			return true, err
		}
		return true, desk.SetTasks(app, tasks)
	case dAddRecentDocument:
		app, path := d.handlerApp(), d.str()
		if err := d.end(); err != nil {
			return true, err
		}
		return true, desk.AddRecentDocument(app, path)
	case dClearRecentDocuments:
		app := d.handlerApp()
		if err := d.end(); err != nil {
			return true, err
		}
		return true, desk.ClearRecentDocuments(app)
	}
	return false, nil
}

// tasks encodes the tasks of SetTasks.
func (e *enc) tasks(ts []platform.Task) {
	e.u32(uint32(len(ts)))
	for _, t := range ts {
		e.str(t.Title)
		e.str(t.Description)
		e.strs(t.Args)
		e.images(t.Icon)
	}
}

func (d *dec) tasks() []platform.Task {
	n := d.count(platform.MaxTasks, "tasks")
	if n == 0 || d.err != nil {
		return nil
	}
	out := make([]platform.Task, n)
	for i := range out {
		out[i] = platform.Task{Title: d.str(), Description: d.str(), Args: d.strs(), Icon: d.images()}
	}
	return out
}
