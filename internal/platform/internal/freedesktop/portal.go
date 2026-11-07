//go:build linux

package freedesktop

import (
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/godbus/dbus/v5"
	"golang.org/x/sys/unix"

	"github.com/kuronosu/tela/internal/platform"
)

// --- OpenURI, OpenPath ---
//
// Both pass the parent window and an XDG activation token
// (Options.Prepare): the portal hands the token to the application it
// starts (OpenURI version 3 and later), so that its window may take the
// focus. Without a portal, xdg-open gets the token in its environment,
// as GLib's launchers do.

// OpenURI opens uri with the portal's OpenURI, or with xdg-open when there
// is no portal. A portal that answers the call with an error (it refused
// the URI) also falls back to xdg-open; a failure after that is not
// reported (contract).
func (d *Desktop) OpenURI(parent platform.Window, uri string) error {
	if err := platform.CheckURI(uri); err != nil {
		return err
	}
	if d.isClosed() {
		return errClosed
	}
	conn, _, err := d.portal(ifaceOpenURI)
	if err != nil {
		return d.openWithOpener(parent, uri, err)
	}
	return d.prepared(parent, true, func(id, token string) error {
		ch := make(chan *dbus.Call, 1)
		conn.Object(portalName, portalPath).Go(ifaceOpenURI+".OpenURI", 0, ch,
			id, uri, openOptions(token))
		go func() {
			if c := <-ch; c.Err != nil {
				d.runOpener(uri, token, c.Err)
			}
		}()
		return nil
	}, nil, nil)
}

// OpenPath opens a file or folder with the portal's OpenURI.OpenFile, which
// takes a file descriptor (opened O_PATH, as GTK does; a folder opens in
// the file manager), or with xdg-open when there is no portal or the
// connection cannot pass descriptors. A path that does not exist is an
// error.
func (d *Desktop) OpenPath(parent platform.Window, path string) error {
	if path == "" {
		return errors.New("freedesktop: OpenPath with an empty path")
	}
	abs, err := filepath.Abs(path)
	if err != nil {
		return fmt.Errorf("freedesktop: OpenPath: %w", err)
	}
	if _, err := os.Stat(abs); err != nil {
		return fmt.Errorf("freedesktop: OpenPath: %w", err)
	}
	if d.isClosed() {
		return errClosed
	}
	conn, _, err := d.portal(ifaceOpenURI)
	if err == nil && !conn.SupportsUnixFDs() {
		err = errors.New("the D-Bus connection cannot pass file descriptors")
	}
	if err != nil {
		return d.openWithOpener(parent, abs, err)
	}
	fd, err := unix.Open(abs, unix.O_PATH|unix.O_CLOEXEC, 0)
	if err != nil {
		return fmt.Errorf("freedesktop: OpenPath: %w", &os.PathError{Op: "open", Path: abs, Err: err})
	}
	return d.prepared(parent, true, func(id, token string) error {
		ch := make(chan *dbus.Call, 1)
		conn.Object(portalName, portalPath).Go(ifaceOpenURI+".OpenFile", 0, ch,
			id, dbus.UnixFD(fd), openOptions(token))
		go func() {
			c := <-ch // the message (and its descriptor) was sent before Go returned
			unix.Close(fd)
			if c.Err != nil {
				d.runOpener(abs, token, c.Err)
			}
		}()
		return nil
	}, nil, func() { unix.Close(fd) })
}

// openOptions are the options of OpenURI and OpenFile: the activation
// token, when there is one.
func openOptions(token string) map[string]dbus.Variant {
	o := map[string]dbus.Variant{}
	if token != "" {
		o["activation_token"] = dbus.MakeVariant(token)
	}
	return o
}

// openWithOpener opens target with xdg-open when the portal cannot (why).
// The opener must be installed (else the error wraps both reasons); it is
// started once the activation token is known, and a failure to start it
// then is returned only if that was at once. UI thread.
func (d *Desktop) openWithOpener(parent platform.Window, target string, why error) error {
	name, path, err := d.opener(target, why)
	if err != nil {
		return err
	}
	return d.prepared(parent, true, func(_, token string) error {
		return startOpener(name, path, target, token)
	}, nil, nil)
}

// runOpener runs xdg-open (Options.Opener) on target without waiting for
// it. why is the reason the portal was not used; when the opener is not
// installed either, the error wraps both. Safe from any goroutine.
func (d *Desktop) runOpener(target, token string, why error) error {
	name, path, err := d.opener(target, why)
	if err != nil {
		return err
	}
	return startOpener(name, path, target, token)
}

// opener finds xdg-open (Options.Opener).
func (d *Desktop) opener(target string, why error) (name, path string, err error) {
	name = d.opts.Opener
	if name == "" {
		name = "xdg-open"
	}
	path, err = exec.LookPath(name)
	if err != nil {
		return "", "", fmt.Errorf("freedesktop: cannot open %q: %v; %s is not installed either: %w", target, why, name, platform.ErrUnsupported)
	}
	return name, path, nil
}

// startOpener starts the opener at path on target without waiting for
// it. The activation token, if any, goes in XDG_ACTIVATION_TOKEN and
// DESKTOP_STARTUP_ID (GLib sets both, and xdg-open's helpers pass them
// on); a token this process was itself started with is never passed on,
// since it was meant for this process and is spent. Safe from any
// goroutine.
func startOpener(name, path, target, token string) error {
	cmd := exec.Command(path, target)
	cmd.Env = openerEnv(os.Environ(), token)
	if err := cmd.Start(); err != nil {
		return fmt.Errorf("freedesktop: cannot run %s: %w", name, err)
	}
	go cmd.Wait() // reap it
	return nil
}

// openerEnv returns env without the activation variables, plus token in
// both of them when it is not "".
func openerEnv(env []string, token string) []string {
	out := make([]string, 0, len(env)+2)
	for _, kv := range env {
		if strings.HasPrefix(kv, "XDG_ACTIVATION_TOKEN=") || strings.HasPrefix(kv, "DESKTOP_STARTUP_ID=") {
			continue
		}
		out = append(out, kv)
	}
	if token != "" {
		out = append(out, "XDG_ACTIVATION_TOKEN="+token, "DESKTOP_STARTUP_ID="+token)
	}
	return out
}

// --- file dialogs ---

// request is a portal request waiting for its Response signal. finish
// runs on a godbus goroutine with the answer (uris nil and err nil:
// canceled) and returns the callback for the UI thread.
type request struct {
	finish func(uris []string, err error) func()
}

// portalFilter is a FileChooser filter: (sa(us)), a name and a list of
// (kind, pattern) rules, kind 0 for a glob and 1 for a MIME type.
type portalFilter struct {
	Name  string
	Rules []portalRule
}

type portalRule struct {
	Kind    uint32
	Pattern string
}

// chooserOptions converts the options shared by every file dialog.
func chooserOptions(opts platform.FileDialogOptions) map[string]dbus.Variant {
	o := map[string]dbus.Variant{} // modal is set by choose, with the parent
	if len(opts.Filters) > 0 {
		filters := make([]portalFilter, len(opts.Filters))
		for i, f := range opts.Filters {
			filters[i].Name = f.Name
			if filters[i].Name == "" {
				filters[i].Name = strings.Join(f.Patterns, ", ")
			}
			for _, p := range f.Patterns {
				filters[i].Rules = append(filters[i].Rules, portalRule{0, p})
			}
		}
		o["filters"] = dbus.MakeVariant(filters)
		o["current_filter"] = dbus.MakeVariant(filters[0])
	}
	if opts.Directory != "" {
		if abs, err := filepath.Abs(opts.Directory); err == nil {
			// A byte string with its terminating NUL (type ay).
			o["current_folder"] = dbus.MakeVariant(append([]byte(abs), 0))
		}
	}
	return o
}

// OpenFiles shows the portal's OpenFile dialog.
func (d *Desktop) OpenFiles(opts platform.FileDialogOptions, done func([]string, error)) error {
	if done == nil {
		panic("freedesktop: OpenFiles called with a nil callback")
	}
	platform.CheckFileDialogOptions("OpenFiles", opts)
	o := chooserOptions(opts)
	o["multiple"] = dbus.MakeVariant(opts.Multiple)
	return d.choose("OpenFile", 1, opts, o, func(uris []string, err error) func() {
		var paths []string
		if err == nil {
			paths, err = uriPaths(uris)
		}
		if !opts.Multiple && len(paths) > 1 {
			paths = paths[:1]
		}
		return func() { done(paths, err) }
	})
}

// SaveFile shows the portal's SaveFile dialog.
func (d *Desktop) SaveFile(opts platform.FileDialogOptions, done func(string, error)) error {
	if done == nil {
		panic("freedesktop: SaveFile called with a nil callback")
	}
	platform.CheckFileDialogOptions("SaveFile", opts)
	o := chooserOptions(opts)
	if opts.Name != "" {
		o["current_name"] = dbus.MakeVariant(opts.Name)
	}
	return d.choose("SaveFile", 1, opts, o, firstPath(done))
}

// PickFolder shows the portal's OpenFile dialog in directory mode
// (FileChooser version 3 and later).
func (d *Desktop) PickFolder(opts platform.FileDialogOptions, done func(string, error)) error {
	if done == nil {
		panic("freedesktop: PickFolder called with a nil callback")
	}
	platform.CheckFileDialogOptions("PickFolder", opts)
	opts.Filters = nil // folders only
	o := chooserOptions(opts)
	o["directory"] = dbus.MakeVariant(true)
	return d.choose("OpenFile", 3, opts, o, firstPath(done))
}

func firstPath(done func(string, error)) func([]string, error) func() {
	return func(uris []string, err error) func() {
		p := ""
		if err == nil && len(uris) > 0 {
			p, err = uriPath(uris[0])
		}
		return func() { done(p, err) }
	}
}

// choose calls a FileChooser method. It subscribes to the Response signal
// at the request's predictable handle path before calling, so that a fast
// answer is not lost (the portal documentation's rule), then waits for the
// reply on a goroutine: an error reply ends the request, and a handle at
// another path (portals before 0.9) moves it there.
func (d *Desktop) choose(method string, minVersion uint32, opts platform.FileDialogOptions, o map[string]dbus.Variant, finish func([]string, error) func()) error {
	if d.isClosed() {
		return errClosed
	}
	conn, version, err := d.portal(ifaceFileChooser)
	if err != nil {
		return err
	}
	if version < minVersion {
		return fmt.Errorf("freedesktop: the XDG desktop portal's FileChooser is version %d; folder dialogs need version %d: %w", version, minVersion, platform.ErrUnsupported)
	}
	// The dialog is sent once its parent's identifier is known (Wayland
	// exports the toplevel first).
	return d.prepared(opts.Parent, false, func(parent, _ string) error {
		// Modal only with a parent to be modal to.
		o["modal"] = dbus.MakeVariant(parent != "")
		d.tokens++
		token := fmt.Sprintf("tela%d", d.tokens)
		path := requestPath(conn.Names()[0], token)
		o["handle_token"] = dbus.MakeVariant(token)

		req := &request{finish: finish}
		d.mu.Lock()
		d.requests[path] = req
		d.mu.Unlock()
		if err := addMatch(conn, responseMatch(path)...); err != nil {
			d.mu.Lock()
			delete(d.requests, path)
			d.mu.Unlock()
			return fmt.Errorf("freedesktop: cannot subscribe to the portal's answer: %w", err)
		}
		ch := make(chan *dbus.Call, 1)
		conn.Object(portalName, portalPath).Go(ifaceFileChooser+"."+method, 0, ch, parent, opts.Title, o)
		go d.awaitRequest(conn, path, ch)
		return nil
	}, func(err error) { finish(nil, err)() }, nil)
}

// requestPath is the handle path the portal gives a request:
// /org/freedesktop/portal/desktop/request/SENDER/TOKEN, SENDER being the
// caller's unique name without its ':' and with '.' replaced by '_'.
func requestPath(unique, token string) dbus.ObjectPath {
	sender := strings.ReplaceAll(strings.TrimPrefix(unique, ":"), ".", "_")
	return dbus.ObjectPath("/org/freedesktop/portal/desktop/request/" + sender + "/" + token)
}

func responseMatch(path dbus.ObjectPath) []dbus.MatchOption {
	return []dbus.MatchOption{
		dbus.WithMatchObjectPath(path),
		dbus.WithMatchInterface(ifaceRequest),
		dbus.WithMatchMember("Response"),
	}
}

// awaitRequest handles the reply to a FileChooser call (godbus goroutine).
func (d *Desktop) awaitRequest(conn *dbus.Conn, path dbus.ObjectPath, ch chan *dbus.Call) {
	c := <-ch
	if c.Err != nil {
		d.mu.Lock()
		req := d.requests[path]
		delete(d.requests, path)
		d.mu.Unlock()
		conn.RemoveMatchSignal(responseMatch(path)...)
		if req != nil {
			d.push(item{fn: req.finish(nil, fmt.Errorf("freedesktop: the file dialog failed: %w", c.Err))})
		}
		return
	}
	var handle dbus.ObjectPath
	if c.Store(&handle) != nil || handle == path {
		return
	}
	// An old portal chose its own handle path.
	d.mu.Lock()
	req := d.requests[path]
	if req != nil {
		delete(d.requests, path)
		d.requests[handle] = req
	}
	d.mu.Unlock()
	conn.RemoveMatchSignal(responseMatch(path)...)
	if req != nil {
		addMatch(conn, responseMatch(handle)...)
	}
}

// onResponse handles a Request's Response signal: (u response, a{sv}
// results), response 0 for success, 1 for canceled, other codes for a
// dialog that ended another way.
func (d *Desktop) onResponse(conn *dbus.Conn, s *dbus.Signal) {
	d.mu.Lock()
	req := d.requests[s.Path]
	delete(d.requests, s.Path)
	d.mu.Unlock()
	if req == nil {
		return
	}
	go conn.RemoveMatchSignal(responseMatch(s.Path)...)
	var (
		code    uint32
		results map[string]dbus.Variant
		uris    []string
		err     error
	)
	if dbus.Store(s.Body, &code, &results) != nil {
		err = errors.New("freedesktop: the portal sent an invalid answer")
	} else {
		switch code {
		case 0:
			if v, ok := results["uris"]; ok {
				if v.Store(&uris) != nil {
					err = errors.New("freedesktop: the portal sent invalid URIs")
				}
			}
		case 1: // canceled
		default:
			err = fmt.Errorf("freedesktop: the file dialog ended without an answer (portal response %d)", code)
		}
	}
	d.push(item{fn: req.finish(uris, err)})
}
