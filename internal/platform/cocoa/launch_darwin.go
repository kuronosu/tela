//go:build darwin

package cocoa

import (
	"fmt"
	"os"

	"github.com/kuronosu/tela/internal/darwin"
	"github.com/kuronosu/tela/internal/platform"
)

// The app ID, single instance and launches (ADR-224).
//
// Single instance is a lock file and a Unix socket in the per-user
// temporary folder (os.TempDir: $TMPDIR, /var/folders/…/T/, which only the
// user can enter), named from the app ID; instance_unix.go has the
// details. The forwarded launches arrive on a goroutine of the claim and
// hop to the main thread with onMain.
//
// Activation: since macOS 14 activation is cooperative, so [NSApp
// activate] in the first process (Window.ActivateWithToken, which is
// Activate) is honored only when the active app yields to it. The second
// process, on receiving the first's PID in the hello and before it sends
// the launch, calls -[NSApplication yieldActivationToApplication:] with
// the first's NSRunningApplication (macOS 14 and later; macOS 13's
// activateIgnoringOtherApps: needs no yield). A yield helps only when the
// second process is the active app: started from the Finder, the Dock or
// a link (Launch Services activates what it launches), it is; started
// from a terminal it usually is not, and the terminal keeps the focus
// unless the user's settings let the first process take it. The first
// process's activation is then up to the system, as for any app.
//
// Launch Services launches: when the app is a bundle that declares URL
// schemes or document types, the system hands it the URLs and files the
// user opens with it (a link clicked in another app, a file opened from
// the Finder) as an Apple Event, which AppKit gives the app delegate's
// application:openURLs:. Each call is one LaunchEvent: file URLs as their
// paths, other URLs as they are, Dir the working directory, no token.
// That happens whether or not the app claimed single instance: Launch
// Services sends a running app the event instead of starting a second
// process. A launch at the start of the app (the user opened a file to
// start it) comes as the same Apple Event, while [NSApp run] finishes
// launching, after the start's own Launch was taken (CurrentLaunch, whose
// Args do not hold it): it is a LaunchEvent like the later ones, so the
// layers above see it as a launch after the start (desktop.UseLaunch's
// Forwarded). Launches that reach the backend before its first delivery
// pass has ended (that one, and those forwarded by a second process
// before Run) are held until it has: the first pass delivers the main
// window's first frame, whose commit mounts the listeners of the layers
// above, which would otherwise miss them.
//
// AppKit also turns the command-line arguments of an unbundled program
// into application:openURLs: calls (NSTreatUnknownArgumentsAsOpen), which
// would deliver every argument twice: the backend registers that default
// as NO, as Chromium does, so only Launch Services' events become
// launches.

// launchState is the backend's launches and claims, on the main thread.
type launchState struct {
	appID  string                    // SetAppID's, "" until then
	claims map[string]*instanceClaim // by app ID
	passed bool                      // a delivery pass has ended
	held   []platform.Launch         // launches waiting for passed
}

// SetAppID keeps the app's ID for the services that need one. None does
// yet: notifications use the bundle's identifier, and the keychain and
// single instance the ID the layers above give each call.
func (d desktop) SetAppID(id string) {
	platform.CheckAppID("SetAppID", id)
	d.b.desk.launch.appID = id
}

// ClaimInstance claims appID's single instance (see the top of the file).
func (d desktop) ClaimInstance(appID string, launch platform.Launch) (bool, error) {
	platform.CheckAppID("ClaimInstance", appID)
	b := d.b
	if b.closed {
		return false, errClosed
	}
	if b.desk.launch.claims[appID] != nil {
		return true, nil
	}
	c, primary, err := claimInstance(os.TempDir(), appID, launch, b.forwardedLaunch, yieldActivation)
	if err != nil {
		return false, fmt.Errorf("cocoa: ClaimInstance(%q): %w", appID, err)
	}
	if !primary {
		return false, nil
	}
	if b.desk.launch.claims == nil {
		b.desk.launch.claims = map[string]*instanceClaim{}
	}
	b.desk.launch.claims[appID] = c
	return true, nil
}

// forwardedLaunch receives a launch from the claim's goroutine.
func (b *Backend) forwardedLaunch(l platform.Launch) {
	b.onMain(func() { b.launch(l) })
}

// launch delivers l as an app-scoped LaunchEvent, or holds it until the
// first delivery pass has ended. Main thread.
func (b *Backend) launch(l platform.Launch) {
	if b.closed {
		return
	}
	if !b.desk.launch.passed {
		b.desk.launch.held = append(b.desk.launch.held, l)
		b.notify()
		return
	}
	b.appEvent(platform.LaunchEvent{Launch: l})
}

// passEnded runs after every delivery pass: after the first one it
// queues the held launches for the next.
func (b *Backend) passEnded() {
	st := &b.desk.launch
	if st.passed {
		return
	}
	st.passed = true
	for _, l := range st.held {
		b.enqueue(nil, platform.LaunchEvent{Launch: l})
	}
	st.held = nil
}

// stopInstances ends the claims (Run returns): the listeners close,
// removing their sockets, and the locks are released.
func (b *Backend) stopInstances() {
	for id, c := range b.desk.launch.claims {
		c.close()
		delete(b.desk.launch.claims, id)
	}
	b.desk.launch.held = nil
}

// yieldActivation lets the first process (pid) take the activation from
// this one, the second, on macOS 14 and later (see the top of the file).
func yieldActivation(pid int) {
	pool := darwin.PoolPush()
	defer darwin.PoolPop(pool)
	if !darwin.SendBool(app.ns, lsSel.respondsToSelector, lsSel.yieldActivation) {
		return // macOS 13: activation is not cooperative
	}
	target := darwin.SendClass(darwin.GetClass("NSRunningApplication"), lsSel.withPID, int32(pid))
	if target == 0 {
		return
	}
	darwin.Send(app.ns, lsSel.yieldActivation, target)
}

// lsSel holds the selectors of the launches.
var lsSel = struct {
	respondsToSelector, yieldActivation, withPID darwin.SEL
	isFileURL, filePathURL, absoluteString       darwin.SEL
	registerDefaults, standardUserDefaults       darwin.SEL
}{
	respondsToSelector:   darwin.Sel("respondsToSelector:"),
	yieldActivation:      darwin.Sel("yieldActivationToApplication:"),
	withPID:              darwin.Sel("runningApplicationWithProcessIdentifier:"),
	isFileURL:            darwin.Sel("isFileURL"),
	filePathURL:          darwin.Sel("filePathURL"),
	absoluteString:       darwin.Sel("absoluteString"),
	registerDefaults:     darwin.Sel("registerDefaults:"),
	standardUserDefaults: darwin.Sel("standardUserDefaults"),
}

// appDelegateLaunchMethods are the app delegate's methods of the
// launches, registered with the delegate class (cocoa_darwin.go).
func appDelegateLaunchMethods() []darwin.Method {
	return []darwin.Method{
		{Sel: darwin.Sel("application:openURLs:"), Fn: applicationOpenURLs},
	}
}

// registerLaunchDefaults turns off AppKit's opening of the command-line
// arguments (see the top of the file), in the registration domain: the
// user's own defaults still win. Before [NSApp run].
func registerLaunchDefaults() {
	d := darwin.NewDictionary()
	darwin.DictionarySet(d, darwin.AutoString("NSTreatUnknownArgumentsAsOpen"), darwin.AutoString("NO"))
	darwin.Send(darwin.SendClass(app.userDefaults, lsSel.standardUserDefaults), lsSel.registerDefaults, d)
}

// applicationOpenURLs is -[NSApplicationDelegate application:openURLs:]:
// the URLs Launch Services hands the app, one launch.
func applicationOpenURLs(_ darwin.ID, _ darwin.SEL, _ darwin.ID, urls darwin.ID) {
	b := runningBackend()
	if b == nil {
		return
	}
	b.guard(func() {
		l := platform.Launch{}
		l.Dir, _ = os.Getwd()
		for _, u := range darwin.ArrayItems(urls) {
			if a := urlArg(u); a != "" {
				l.Args = append(l.Args, a)
			}
		}
		if len(l.Args) > 0 {
			b.launch(l)
		}
	})
}

// urlArg returns the launch argument of an NSURL: a file URL's path (a
// file reference URL resolved to its path), else the URL's text.
func urlArg(u darwin.ID) string {
	if u == 0 {
		return ""
	}
	if darwin.SendBool(u, lsSel.isFileURL) {
		return darwin.URLPath(darwin.Send(u, lsSel.filePathURL))
	}
	return darwin.GoString(darwin.Send(u, lsSel.absoluteString))
}

// runningBackend returns the backend whose [NSApp run] is running, or
// nil.
func runningBackend() *Backend {
	registry.Lock()
	defer registry.Unlock()
	for _, b := range registry.backends {
		if b.running {
			return b
		}
	}
	return nil
}
