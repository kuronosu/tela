//go:build linux

package freedesktop

import (
	"bytes"
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"image/png"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/godbus/dbus/v5"

	"github.com/kuronosu/tela/internal/platform"
	"github.com/kuronosu/tela/internal/platform/assoc"
)

// The desktop surfaces of platform contract v16 (ADR-298). The files are
// package assoc's (plain file I/O on the XDG directories, tested on every
// OS): the autostart entry, the actions of the app's desktop entry and
// recently-used.xbel. In a Flatpak sandbox start at login goes through
// org.freedesktop.portal.Background, since the sandbox cannot write the
// host's autostart folder; the portal writes the host's entry (with a
// "flatpak run" command of its own), and the entry in the sandbox's
// $XDG_CONFIG_HOME records the answer for StartsAtLogin, which the portal
// cannot be asked. Every call blocks and is safe from any goroutine; the
// portal's runs on a connection of its own, like the trash's.

const (
	ifaceBackground = "org.freedesktop.portal.Background"
	// backgroundTimeout bounds the wait for the portal's answer, which may
	// come after the user answers a dialog.
	backgroundTimeout = 5 * time.Minute
)

// surfaceDirs returns the XDG directories of the session (a variable for
// tests).
var surfaceDirs = assoc.LinuxDirsFromEnv

// inFlatpak reports whether the process runs in a Flatpak sandbox (a
// variable for tests).
var inFlatpak = func() bool {
	_, err := os.Stat("/.flatpak-info")
	return err == nil
}

// SetStartAtLogin writes or removes the autostart entry (assoc), or asks
// the Background portal in a Flatpak sandbox.
func (d *Desktop) SetStartAtLogin(app platform.HandlerApp, on bool) error {
	platform.CheckHandlerApp("SetStartAtLogin", app, on)
	if d.isClosed() {
		return errClosed
	}
	if inFlatpak() {
		if err := d.requestBackground(app, on); err != nil {
			return err
		}
	}
	_, err := assoc.LinuxSetStartAtLogin(surfaceDirs(), app, on)
	return err
}

// StartsAtLogin reads the autostart entries (in a sandbox, its own
// record of the portal's answer).
func (d *Desktop) StartsAtLogin(app platform.HandlerApp) (bool, error) {
	platform.CheckHandlerApp("StartsAtLogin", app, false)
	return assoc.LinuxStartsAtLogin(surfaceDirs(), app)
}

// requestBackground asks the Background portal to start the app at login
// (autostart) with its command line and AtLoginFlag, or no longer. It
// waits for the portal's Response.
func (d *Desktop) requestBackground(app platform.HandlerApp, on bool) error {
	conn, err := d.dial()
	if err != nil {
		return err
	}
	defer conn.Close()
	if !nameOnBus(conn, portalName) {
		return fmt.Errorf("freedesktop: SetStartAtLogin in a Flatpak sandbox needs the XDG desktop portal (%s): %w", portalName, platform.ErrUnsupported)
	}
	names := conn.Names()
	if len(names) == 0 {
		return errors.New("freedesktop: SetStartAtLogin: the D-Bus connection has no name")
	}
	var buf [8]byte
	_, _ = rand.Read(buf[:])
	token := "tela_bg_" + hex.EncodeToString(buf[:])
	path := requestPath(names[0], token)
	signals := make(chan *dbus.Signal, 8)
	conn.Signal(signals)
	if err := addMatch(conn, responseMatch(path)...); err != nil {
		return fmt.Errorf("freedesktop: cannot subscribe to the portal's answer: %w", err)
	}
	cmd := append(append([]string{app.Exec}, app.Args...), platform.AtLoginFlag)
	opts := map[string]dbus.Variant{
		"handle_token": dbus.MakeVariant(token),
		"autostart":    dbus.MakeVariant(on),
		"reason":       dbus.MakeVariant("Start " + app.Name + " when you log in"),
	}
	if on {
		opts["commandline"] = dbus.MakeVariant(cmd)
	}
	ctx, cancel := context.WithTimeout(context.Background(), backgroundTimeout)
	defer cancel()
	var handle dbus.ObjectPath
	if err := conn.Object(portalName, portalPath).CallWithContext(ctx, ifaceBackground+".RequestBackground", 0, "", opts).Store(&handle); err != nil {
		return fmt.Errorf("freedesktop: the Background portal's RequestBackground failed: %w", err)
	}
	for {
		select {
		case <-ctx.Done():
			return fmt.Errorf("freedesktop: the Background portal did not answer within %v", backgroundTimeout)
		case s, ok := <-signals:
			if !ok {
				return errors.New("freedesktop: the session bus closed before the Background portal answered")
			}
			if (s.Path != path && s.Path != handle) || s.Name != ifaceRequest+".Response" || len(s.Body) < 2 {
				continue
			}
			code, _ := s.Body[0].(uint32)
			results, _ := s.Body[1].(map[string]dbus.Variant)
			return backgroundOutcome(on, code, results)
		}
	}
}

// backgroundOutcome is the error of the portal's Response to a request
// for on: nil when the portal granted it (or it turns autostart off).
func backgroundOutcome(on bool, code uint32, results map[string]dbus.Variant) error {
	switch code {
	case 0:
	case 1:
		return errors.New("freedesktop: SetStartAtLogin: the user did not allow the app to run in the background")
	default:
		return fmt.Errorf("freedesktop: SetStartAtLogin: the Background portal answered %d", code)
	}
	if !on {
		return nil
	}
	if v, ok := results["autostart"]; ok {
		if granted, _ := v.Value().(bool); !granted {
			return errors.New("freedesktop: SetStartAtLogin: the Background portal did not allow the app to start at login")
		}
	}
	return nil
}

// SetTasks writes the tasks' icons, then the actions of the app's
// desktop entry (assoc).
func (d *Desktop) SetTasks(app platform.HandlerApp, tasks []platform.Task) error {
	platform.CheckHandlerApp("SetTasks", app, true)
	platform.CheckTasks("SetTasks", tasks)
	dirs := surfaceDirs()
	icons, err := writeTaskIcons(filepath.Join(dirs.DataHome, "tela", "tasks", app.ID), tasks)
	if err != nil {
		return err
	}
	_, err = assoc.LinuxSetActions(dirs, app.ID, tasks, icons)
	return err
}

// writeTaskIcons writes each task's icon (its 48 px image, or the best
// one) as <dir>/<N>.png, removes the others of dir, and returns the paths
// ("" for a task without an icon).
func writeTaskIcons(dir string, tasks []platform.Task) ([]string, error) {
	paths := make([]string, len(tasks))
	keep := map[string]bool{}
	for i, t := range tasks {
		img := platform.PickIcon(t.Icon, 48)
		if img == nil {
			continue
		}
		if err := os.MkdirAll(dir, 0o700); err != nil {
			return nil, fmt.Errorf("freedesktop: SetTasks: %w", err)
		}
		name := strconv.Itoa(i+1) + ".png"
		p := filepath.Join(dir, name)
		var b bytes.Buffer
		if err := png.Encode(&b, img); err != nil {
			return nil, fmt.Errorf("freedesktop: SetTasks: the icon of %q: %w", t.Title, err)
		}
		if old, err := os.ReadFile(p); err != nil || !bytes.Equal(old, b.Bytes()) {
			if err := os.WriteFile(p+".tmp", b.Bytes(), 0o600); err != nil {
				return nil, fmt.Errorf("freedesktop: SetTasks: %w", err)
			}
			if err := os.Rename(p+".tmp", p); err != nil {
				return nil, fmt.Errorf("freedesktop: SetTasks: %w", err)
			}
		}
		paths[i], keep[name] = p, true
	}
	ents, _ := os.ReadDir(dir)
	for _, e := range ents {
		if !keep[e.Name()] && strings.HasSuffix(e.Name(), ".png") {
			_ = os.Remove(filepath.Join(dir, e.Name()))
		}
	}
	return paths, nil
}

// AddRecentDocument adds the file to recently-used.xbel (assoc).
func (d *Desktop) AddRecentDocument(app platform.HandlerApp, path string) error {
	platform.CheckHandlerApp("AddRecentDocument", app, true)
	clean, fi, err := platform.CheckRecent(path)
	if err != nil {
		return err
	}
	return assoc.LinuxAddRecent(surfaceDirs(), app, clean, assoc.RecentMIMEType(clean, fi.IsDir()), time.Now())
}

// ClearRecentDocuments removes the app's entries of recently-used.xbel.
func (d *Desktop) ClearRecentDocuments(app platform.HandlerApp) error {
	platform.CheckHandlerApp("ClearRecentDocuments", app, false)
	_, err := assoc.LinuxClearRecent(surfaceDirs(), app, time.Now())
	return err
}
