package headless

import (
	"bytes"
	"fmt"
	"maps"
	"slices"
	"sync"

	"github.com/kuronosu/tela/internal/platform"
	"github.com/kuronosu/tela/internal/platform/assoc"
)

// --- app ID ---

// SetAppID records the app ID (ADR-217), which DesktopAppID returns.
func (d desktop) SetAppID(id string) {
	platform.CheckAppID("SetAppID", id)
	d.b.mu.Lock()
	defer d.b.mu.Unlock()
	d.b.desktop.appID = id
}

// SetStrings records the texts the layers above gave (ADR-251), which
// DesktopStrings returns; a headless backend shows none of them.
func (d desktop) SetStrings(strings platform.Strings) {
	d.b.mu.Lock()
	defer d.b.mu.Unlock()
	d.b.desktop.strings = maps.Clone(strings)
}

// DesktopStrings returns a copy of the strings the layers above gave with
// Desktop().SetStrings, nil before any. Safe from any goroutine.
func (b *Backend) DesktopStrings() platform.Strings {
	b.mu.Lock()
	defer b.mu.Unlock()
	return maps.Clone(b.desktop.strings)
}

// DesktopAppID returns the app ID the layers above gave with
// Desktop().SetAppID, "" before any. Safe from any goroutine.
func (b *Backend) DesktopAppID() string {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.desktop.appID
}

// --- global shortcuts ---

// ShortcutRecord is a global shortcut registered with RegisterShortcut and
// not unregistered.
type ShortcutRecord struct {
	Shortcut platform.Shortcut
	// Status and Trigger are the latest binding outcome
	// (ShortcutStatusEvent) given to the app.
	Status  platform.ShortcutStatus
	Trigger string
}

// SetShortcutResponder sets the function that decides the outcome of the
// RegisterShortcut calls made from then on, as the OS (or the user in the
// portal's dialog) would: it is called on the UI thread inside the
// request, without the backend's lock held, and its event (whose ID is
// forced to the shortcut's) reaches the app through the loop. nil (the
// default) binds every shortcut to its accelerator (ShortcutBound, Trigger
// Accelerator.String()). Safe from any goroutine.
func (b *Backend) SetShortcutResponder(fn func(platform.Shortcut) platform.ShortcutStatusEvent) {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.desktop.shortcutResponder = fn
}

// SetShortcutError makes RegisterShortcut return err, with no request
// made and no status to follow, until SetShortcutError(nil): a desktop
// that refused the app ID (an error wrapping
// platform.ErrAppNotRegistered, as the portal answers without the app's
// desktop entry). Safe from any goroutine.
func (b *Backend) SetShortcutError(err error) {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.desktop.shortcutErr = err
}

// Shortcuts returns the registered global shortcuts, in registration
// order (a shortcut registered again keeps its place).
func (b *Backend) Shortcuts() []ShortcutRecord {
	b.mu.Lock()
	defer b.mu.Unlock()
	return slices.Clone(b.desktop.shortcuts)
}

// checkShortcutEvent returns why e could not come from a real backend, or
// ""; b.mu must be held.
func (b *Backend) checkShortcutEvent(e platform.ShortcutEvent) string {
	i := slices.IndexFunc(b.desktop.shortcuts, func(r ShortcutRecord) bool { return r.Shortcut.ID == e.ID })
	switch {
	case i < 0:
		return fmt.Sprintf("no shortcut %q is registered", e.ID)
	case b.desktop.shortcuts[i].Status != platform.ShortcutBound:
		return fmt.Sprintf("shortcut %q is not bound (%v)", e.ID, b.desktop.shortcuts[i].Status)
	case e.Action > platform.ShortcutReleased:
		return fmt.Sprintf("invalid Action %d", e.Action)
	}
	return ""
}

// applyShortcutStatus records a later status change of a registered
// shortcut, or returns why it could not come from a real backend; b.mu
// must be held.
func (b *Backend) applyShortcutStatus(e platform.ShortcutStatusEvent) string {
	i := slices.IndexFunc(b.desktop.shortcuts, func(r ShortcutRecord) bool { return r.Shortcut.ID == e.ID })
	switch {
	case i < 0:
		return fmt.Sprintf("no shortcut %q is registered", e.ID)
	case e.Status > platform.ShortcutFailed:
		return fmt.Sprintf("invalid Status %d", e.Status)
	}
	b.desktop.shortcuts[i].Status, b.desktop.shortcuts[i].Trigger = e.Status, e.Trigger
	return ""
}

func (d desktop) RegisterShortcut(s platform.Shortcut) error {
	platform.CheckShortcut(s)
	if !d.supported(ServiceShortcuts) {
		return platform.ErrUnsupported
	}
	b := d.b
	b.mu.Lock()
	fn, failed := b.desktop.shortcutResponder, b.desktop.shortcutErr
	b.mu.Unlock()
	if failed != nil {
		return failed
	}
	ev := platform.ShortcutStatusEvent{Status: platform.ShortcutBound, Trigger: s.Accelerator.String()}
	if fn != nil {
		ev = fn(s)
		if ev.Status > platform.ShortcutFailed {
			panic(fmt.Sprintf("headless: the shortcut responder returned an invalid Status %d", ev.Status))
		}
	}
	ev.ID = s.ID
	b.mu.Lock()
	rec := ShortcutRecord{Shortcut: s, Status: ev.Status, Trigger: ev.Trigger}
	if i := slices.IndexFunc(b.desktop.shortcuts, func(r ShortcutRecord) bool { return r.Shortcut.ID == s.ID }); i >= 0 {
		b.desktop.shortcuts[i] = rec
	} else {
		b.desktop.shortcuts = append(b.desktop.shortcuts, rec)
	}
	b.loop.Enqueue(nil, ev)
	b.mu.Unlock()
	b.signal()
	return nil
}

func (d desktop) UnregisterShortcut(id string) error {
	platform.CheckShortcutID("UnregisterShortcut", id)
	if !d.supported(ServiceShortcuts) {
		return platform.ErrUnsupported
	}
	b := d.b
	b.mu.Lock()
	defer b.mu.Unlock()
	b.desktop.shortcuts = slices.DeleteFunc(b.desktop.shortcuts, func(r ShortcutRecord) bool { return r.Shortcut.ID == id })
	return nil
}

// --- secrets ---

type secretKey struct{ appID, name string }

// SetSecretError makes every secret call fail with err (after checking its
// arguments) until SetSecretError(nil): a locked keyring the user did not
// unlock, a Secret Service that went away. Safe from any goroutine.
func (b *Backend) SetSecretError(err error) {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.desktop.secretErr = err
}

// StoredSecret returns a copy of the secret stored under name for appID.
func (b *Backend) StoredSecret(appID, name string) ([]byte, bool) {
	b.mu.Lock()
	defer b.mu.Unlock()
	v, ok := b.desktop.secrets[secretKey{appID, name}]
	return bytes.Clone(v), ok
}

// secretState returns the error a secret call reports before touching the
// store: ErrUnsupported or the injected error; b.mu must be held.
func (d desktop) secretState() error {
	switch {
	case d.b.desktop.unsupported[ServiceSecrets]:
		return platform.ErrUnsupported
	case d.b.desktop.secretErr != nil:
		return d.b.desktop.secretErr
	}
	return nil
}

func (d desktop) SetSecret(appID, name string, secret []byte) error {
	if err := platform.CheckSecret(appID, name, secret); err != nil {
		return err
	}
	b := d.b
	b.mu.Lock()
	defer b.mu.Unlock()
	if err := d.secretState(); err != nil {
		return err
	}
	if b.desktop.secrets == nil {
		b.desktop.secrets = map[secretKey][]byte{}
	}
	v := bytes.Clone(secret)
	if v == nil {
		v = []byte{}
	}
	b.desktop.secrets[secretKey{appID, name}] = v
	return nil
}

func (d desktop) Secret(appID, name string) ([]byte, error) {
	platform.CheckSecretName("Secret", appID, name)
	b := d.b
	b.mu.Lock()
	defer b.mu.Unlock()
	if err := d.secretState(); err != nil {
		return nil, err
	}
	v, ok := b.desktop.secrets[secretKey{appID, name}]
	if !ok {
		return nil, platform.ErrSecretNotFound
	}
	return bytes.Clone(v), nil
}

func (d desktop) DeleteSecret(appID, name string) error {
	platform.CheckSecretName("DeleteSecret", appID, name)
	b := d.b
	b.mu.Lock()
	defer b.mu.Unlock()
	if err := d.secretState(); err != nil {
		return err
	}
	delete(b.desktop.secrets, secretKey{appID, name})
	return nil
}

// --- single instance ---

// instances is the process-wide registry of claimed app IDs: the
// "session" the headless backends of a process share, so a test plays a
// first and a second instance with two backends.
var instances = struct {
	sync.Mutex
	m map[string]*Backend
}{m: map[string]*Backend{}}

func (d desktop) ClaimInstance(appID string, launch platform.Launch) (bool, error) {
	platform.CheckAppID("ClaimInstance", appID)
	if !d.supported(ServiceInstance) {
		return false, platform.ErrUnsupported
	}
	b := d.b
	instances.Lock()
	defer instances.Unlock()
	switch p := instances.m[appID]; {
	case p == b:
		return true, nil
	case p != nil:
		p.receiveLaunch(launch.Clone())
		return false, nil
	}
	instances.m[appID] = b
	b.mu.Lock()
	b.desktop.instances = append(b.desktop.instances, appID)
	b.mu.Unlock()
	return true, nil
}

// receiveLaunch queues the LaunchEvent of a launch forwarded by another
// backend.
func (b *Backend) receiveLaunch(l platform.Launch) {
	b.mu.Lock()
	b.desktop.launches = append(b.desktop.launches, l.Clone())
	b.loop.Enqueue(nil, platform.LaunchEvent{Launch: l})
	b.mu.Unlock()
	b.signal()
}

// ReleaseInstances gives up the app IDs this backend claimed, as the
// process exiting would; Run does it when it returns. Tests driving the
// backend with Step and Drain call it in a cleanup, since claims are
// shared by the whole test process. Safe from any goroutine.
func (b *Backend) ReleaseInstances() {
	instances.Lock()
	defer instances.Unlock()
	b.mu.Lock()
	ids := b.desktop.instances
	b.desktop.instances = nil
	b.mu.Unlock()
	for _, id := range ids {
		if instances.m[id] == b {
			delete(instances.m, id)
		}
	}
}

// Instances returns the app IDs this backend holds the claim on.
func (b *Backend) Instances() []string {
	b.mu.Lock()
	defer b.mu.Unlock()
	return slices.Clone(b.desktop.instances)
}

// Launches returns the launches forwarded to this backend, in order.
func (b *Backend) Launches() []platform.Launch {
	b.mu.Lock()
	defer b.mu.Unlock()
	out := make([]platform.Launch, len(b.desktop.launches))
	for i, l := range b.desktop.launches {
		out[i] = l.Clone()
	}
	return out
}

// --- associations ---

// Associations returns the backend's fake registration of schemes and
// file types, which Desktop().Associations answers with (unless
// ServiceAssociations is unsupported): seed other applications and
// inspect what the app registered through it.
func (b *Backend) Associations() *assoc.Fake {
	b.mu.Lock()
	defer b.mu.Unlock()
	if b.desktop.assoc == nil {
		b.desktop.assoc = &assoc.Fake{}
	}
	return b.desktop.assoc
}

func (d desktop) Associations() platform.Associations {
	if !d.supported(ServiceAssociations) {
		return platform.UnsupportedAssociations{}
	}
	return d.b.Associations()
}
