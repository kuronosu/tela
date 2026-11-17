package win32

import (
	"bytes"
	"crypto/sha1"
	"crypto/sha256"
	"encoding/hex"
	"encoding/xml"
	"fmt"
	"image"
	"image/png"
	"net/url"
	"os"
	"path/filepath"
	"slices"
	"strconv"
	"strings"
	"time"

	"github.com/kuronosu/tela/internal/platform"
	"github.com/kuronosu/tela/internal/platform/assoc"
)

// The pure half of the WinRT toast notifications (toast_windows.go, ADR-235):
// the toast XML, the activation arguments, the mapping of the toast events to
// NotificationEvents, the AppUserModelID and its per-user registration, the
// cache of icon files, and the interface IDs (with the algorithm that derives
// the IDs of parameterized interfaces, to check the hard-coded ones).

// maxToastActions is the number of buttons a toast shows; Windows refuses
// the XML of a toast with more, so the rest are dropped.
const maxToastActions = 5

// maxAUMID is the longest AppUserModelID Windows takes (shobjidl_core.h:
// 128 characters, no spaces).
const maxAUMID = 128

// toastAUMID returns the AppUserModelID of an app ID: the ID itself when it
// is printable ASCII without spaces and fits, else the ID with every other
// character replaced by '_', cut, and followed by a hash of the original, so
// that two app IDs never share an AUMID.
func toastAUMID(appID string) string {
	ok := len(appID) > 0 && len(appID) <= maxAUMID
	var b strings.Builder
	for i := 0; i < len(appID); i++ {
		c := appID[i]
		if c > ' ' && c < 0x7F && c != '\\' && c != '/' {
			b.WriteByte(c)
		} else {
			b.WriteByte('_')
			ok = false
		}
	}
	if ok {
		return appID
	}
	sum := sha256.Sum256([]byte(appID))
	suffix := "-" + hex.EncodeToString(sum[:8])
	s := b.String()
	if len(s) > maxAUMID-len(suffix) {
		s = s[:maxAUMID-len(suffix)]
	}
	return s + suffix
}

// toastRun names the run of the process (its start time, in base 36), which
// the activation arguments carry.
func toastRun(start time.Time) string { return strconv.FormatInt(start.UnixNano(), 36) }

// toastGroup is the Group of the process's toasts: tags are notification IDs,
// which start at 1 in every run, so the group names the run (toastRun),
// or a toast of this run would replace one the last run left in the Action
// Center.
func toastGroup(start time.Time) string { return "tela-" + toastRun(start) }

// Activation arguments. The body of a toast (its launch attribute) and each
// button carry a query string with the notification's ID, the run that
// showed it (toastRun) and, for a button, its action ID:
// "tela-notification=3&run=kx3j9a" and
// "tela-notification=3&run=kx3j9a&action=save". While the app runs the
// event's sender already names the toast; the ID and the run are there for
// the COM activator (toastactivator_windows.go), which gets only the
// string, possibly in a later process: the run tells a toast of this
// process from an earlier one's, whose ID may be one of this process's.
const (
	toastArgNote   = "tela-notification"
	toastArgRun    = "run"
	toastArgAction = "action"
)

// toastArguments returns the activation arguments of a notification's body
// (action "") or of one of its buttons; run is "" in tests only.
func toastArguments(id platform.NotificationID, run, action string) string {
	s := toastArgNote + "=" + strconv.FormatUint(uint64(id), 10)
	if run != "" {
		s += "&" + toastArgRun + "=" + url.QueryEscape(run)
	}
	if action != "" {
		s += "&" + toastArgAction + "=" + url.QueryEscape(action)
	}
	return s
}

// parseToastArguments decodes toastArguments; ok is false for a string Tela
// did not write. run is "" in the arguments of a Tela that did not write
// it (before ADR-297).
func parseToastArguments(s string) (id platform.NotificationID, run, action string, ok bool) {
	v, err := url.ParseQuery(s)
	if err != nil || !v.Has(toastArgNote) {
		return 0, "", "", false
	}
	n, err := strconv.ParseUint(v.Get(toastArgNote), 10, 32)
	if err != nil || n == 0 {
		return 0, "", "", false
	}
	return platform.NotificationID(n), v.Get(toastArgRun), v.Get(toastArgAction), true
}

// activationEvent maps the arguments a COM activation carries (a toast's
// launch, or a button's arguments) to the NotificationEvent it reports; run
// is this process's toastRun. thisRun says the toast is one of this
// process's, which its Activated delegate reports too (the caller reports
// the click once, while the toast is still known); any other is Earlier.
// ok is false for arguments Tela did not write.
func activationEvent(args, run string) (ev platform.NotificationEvent, thisRun, ok bool) {
	id, from, action, ok := parseToastArguments(args)
	if !ok {
		return ev, false, false
	}
	ev = platform.NotificationEvent{ID: id, Kind: platform.NotificationActivated}
	if action != "" {
		ev.Kind, ev.Action = platform.NotificationActionInvoked, action
	}
	thisRun = from != "" && from == run
	ev.Earlier = !thisRun
	return ev, thisRun, true
}

// toastContent is what a toast shows.
type toastContent struct {
	ID      platform.NotificationID
	Run     string // toastRun of the process
	Title   string
	Body    string
	Image   string // a file: URI of the logo image, or ""
	Actions []platform.NotificationAction
}

// xml returns the toast XML (the ToastGeneric template of the adaptive toast
// schema): the title and the body as the first two texts (a lone one when the
// other is empty, a space when both are, as a toast needs a text), the image
// as appLogoOverride, the activation arguments as the toast's launch and the
// buttons' arguments, at most maxToastActions buttons. Every text and
// attribute is escaped; characters XML cannot hold become U+FFFD.
func (c toastContent) xml() string {
	var b strings.Builder
	esc := func(s string) {
		_ = xml.EscapeText(&b, []byte(s)) // a strings.Builder does not fail
	}
	b.WriteString(`<toast launch="`)
	esc(toastArguments(c.ID, c.Run, ""))
	b.WriteString(`" activationType="foreground"><visual><binding template="ToastGeneric">`)
	texts := slices.DeleteFunc([]string{c.Title, c.Body}, func(s string) bool { return s == "" })
	if len(texts) == 0 {
		texts = []string{" "}
	}
	for _, t := range texts {
		b.WriteString("<text>")
		esc(t)
		b.WriteString("</text>")
	}
	if c.Image != "" {
		b.WriteString(`<image placement="appLogoOverride" src="`)
		esc(c.Image)
		b.WriteString(`"/>`)
	}
	b.WriteString("</binding></visual>")
	if len(c.Actions) > 0 {
		b.WriteString("<actions>")
		for _, a := range c.Actions[:min(len(c.Actions), maxToastActions)] {
			label := a.Label
			if label == "" {
				label = a.ID // a button needs a content
			}
			b.WriteString(`<action content="`)
			esc(label)
			b.WriteString(`" arguments="`)
			esc(toastArguments(c.ID, c.Run, a.ID))
			b.WriteString(`" activationType="foreground"/>`)
		}
		b.WriteString("</actions>")
	}
	b.WriteString("</toast>")
	return b.String()
}

// fileURI returns the file: URI of an absolute Windows path
// (C:\Users\a b\x.png is file:///C:/Users/a%20b/x.png; a UNC path
// \\server\share\x.png is file://server/share/x.png).
func fileURI(path string) string {
	p := strings.ReplaceAll(path, `\`, "/")
	if host, rest, ok := strings.Cut(strings.TrimPrefix(p, "//"), "/"); ok && strings.HasPrefix(p, "//") {
		return (&url.URL{Scheme: "file", Host: host, Path: "/" + rest}).String()
	}
	return (&url.URL{Scheme: "file", Path: "/" + p}).String()
}

// ToastDismissalReason (windows.ui.notifications.h).
const (
	toastUserCanceled      = 0 // the user closed it
	toastApplicationHidden = 1 // ToastNotifier.Hide, or replaced
	toastTimedOut          = 2 // the popup went into the Action Center
)

// toastEventKind is the event of a toast a delegate reports.
type toastEventKind uint8

const (
	toastActivated toastEventKind = iota
	toastDismissed
	toastFailed
)

// toastEvent is what a delegate saw, on whatever thread WinRT called it,
// handed to the UI thread.
type toastEvent struct {
	id     platform.NotificationID
	kind   toastEventKind
	args   string // toastActivated: the activation arguments
	reason int32  // toastDismissed: the ToastDismissalReason
}

// notificationEvent maps a toast event to the NotificationEvent it reports
// (report false: none) and tells whether the toast is finished, so that its
// objects are released. A click on the toast or a button reports and
// finishes it (Windows removes an activated toast). A dismissal by the user
// and a failure are NotificationClosed. A timeout is nothing: the toast
// moved into the Action Center, where the user may still click it. A toast
// hidden by someone else than CloseNotification (which forgets the toast
// first) is closed.
func (e toastEvent) notificationEvent() (ev platform.NotificationEvent, report, finished bool) {
	ev.ID = e.id
	switch e.kind {
	case toastActivated:
		ev.Kind = platform.NotificationActivated
		if _, _, action, ok := parseToastArguments(e.args); ok && action != "" {
			ev.Kind, ev.Action = platform.NotificationActionInvoked, action
		}
		return ev, true, true
	case toastDismissed:
		if e.reason == toastTimedOut {
			return ev, false, false
		}
	}
	ev.Kind = platform.NotificationClosed
	return ev, true, true
}

// The registration of the AppUserModelID (per user, no administrator rights):
// HKCU\Software\Classes\AppUserModelId\<AUMID> with DisplayName and IconUri,
// what the Windows App SDK writes for an unpackaged app, and the marker
// value X-Tela-App that says Tela created the key.
const (
	aumidKeyPrefix = `Software\Classes\AppUserModelId\`
	aumidMarker    = "X-Tela-App"
)

// aumidRegistry is the part of the registry the registration uses: string
// values below HKEY_CURRENT_USER (the semantics of assoc.Registry, which
// assoc.MemRegistry implements for the tests).
type aumidRegistry interface {
	String(key, name string) (value string, ok bool, err error)
	SetString(key, name, value string) error
	// ValueNames returns the names of the values of key, nil when the key
	// does not exist.
	ValueNames(key string) ([]string, error)
	// SubKeys returns the names of the direct subkeys of key, nil when it
	// does not exist.
	SubKeys(key string) ([]string, error)
}

// registerAUMID registers aumid for the current user with displayName and,
// when iconPath is not "", the icon file iconPath. It looks before writing
// and writes nothing that is already there. A key that exists with values
// and without Tela's marker belongs to an installer or to the user: it is
// left as it is (owned false), and the toasts use it. Nothing is ever
// removed.
func registerAUMID(reg aumidRegistry, aumid, displayName, iconPath string) (owned, changed bool, err error) {
	key := aumidKeyPrefix + aumid
	names, err := reg.ValueNames(key)
	if err != nil {
		return false, false, err
	}
	if len(names) > 0 && !slices.ContainsFunc(names, func(n string) bool { return strings.EqualFold(n, aumidMarker) }) {
		return false, false, nil
	}
	set := func(name, value string) error {
		old, ok, err := reg.String(key, name)
		if err != nil || (ok && old == value) {
			return err
		}
		changed = true
		return reg.SetString(key, name, value)
	}
	// The marker first: a key Tela started is Tela's even if a later write
	// fails.
	if err := set(aumidMarker, "1"); err != nil {
		return true, changed, err
	}
	if err := set("DisplayName", displayName); err != nil {
		return true, changed, err
	}
	if iconPath != "" {
		if err := set("IconUri", iconPath); err != nil {
			return true, changed, err
		}
	}
	return true, changed, nil
}

// The registration of the toast activator (ADR-297), beside the AUMID's:
// the COM local server HKCU\Software\Classes\CLSID\{clsid}, whose
// LocalServer32 default value is the command COM runs, with Tela's marker
// on the CLSID key, and the AUMID key's CustomActivator naming it, what
// the Windows App SDK writes for an unpackaged app (it registers its own
// flag; the toast documentation's is -ToastActivated).
const (
	clsidKeyPrefix       = `Software\Classes\CLSID\`
	customActivatorValue = "CustomActivator"
)

// toastServerCommand returns the LocalServer32 command of the executable
// exe: the quoted path and the activation flag.
func toastServerCommand(exe string) string { return `"` + exe + `" ` + assoc.ToastActivatorFlag }

// registerToastActivator registers the COM server clsid (upper case,
// without braces) with command, and makes it the CustomActivator of aumid,
// whose key the caller made sure is Tela's (registerAUMID's owned). It
// writes only what changed: a moved executable rewrites LocalServer32. A
// CLSID key with values or subkeys and without the marker belongs to the
// app's installer, which registered the same server: it is left as it is,
// and the AUMID still names it. The server goes first, so CustomActivator
// never names a CLSID nothing serves. Nothing is ever removed.
func registerToastActivator(reg aumidRegistry, aumid, clsid, command string) (changed bool, err error) {
	set := func(key, name, value string) error {
		old, ok, err := reg.String(key, name)
		if err != nil || (ok && old == value) {
			return err
		}
		changed = true
		return reg.SetString(key, name, value)
	}
	key := clsidKeyPrefix + "{" + clsid + "}"
	names, err := reg.ValueNames(key)
	if err != nil {
		return false, err
	}
	subs, err := reg.SubKeys(key)
	if err != nil {
		return false, err
	}
	ours := slices.ContainsFunc(names, func(n string) bool { return strings.EqualFold(n, aumidMarker) })
	if ours || len(names) == 0 && len(subs) == 0 {
		if err := set(key, aumidMarker, "1"); err != nil {
			return changed, err
		}
		if err := set(key+`\LocalServer32`, "", command); err != nil {
			return changed, err
		}
	}
	return changed, set(aumidKeyPrefix+aumid, customActivatorValue, "{"+clsid+"}")
}

// toastIconMaxAge is how long an icon file the app no longer uses stays in
// the cache: toasts left in the Action Center keep showing it meanwhile.
const toastIconMaxAge = 30 * 24 * time.Hour

// writeToastIcon writes img as a PNG file in dir, named by a hash of its
// pixels so that an unchanged icon is written once (later uses only touch
// the file, which keeps it from being pruned), and returns its path.
func writeToastIcon(dir string, img image.Image, now time.Time) (string, error) {
	var buf bytes.Buffer
	if err := png.Encode(&buf, platform.IconNRGBA(img)); err != nil {
		return "", err
	}
	sum := sha256.Sum256(buf.Bytes())
	path := filepath.Join(dir, hex.EncodeToString(sum[:12])+".png")
	if _, err := os.Stat(path); err == nil {
		_ = os.Chtimes(path, now, now)
		return path, nil
	}
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return "", err
	}
	tmp, err := os.CreateTemp(dir, ".icon-*.tmp")
	if err != nil {
		return "", err
	}
	_, werr := tmp.Write(buf.Bytes())
	cerr := tmp.Close()
	if werr == nil {
		werr = cerr
	}
	if werr == nil {
		werr = os.Rename(tmp.Name(), path)
	}
	if werr != nil {
		_ = os.Remove(tmp.Name())
		return "", werr
	}
	return path, nil
}

// pruneToastIcons removes the icon files of dir (*.png and leftover
// temporary files) older than toastIconMaxAge, except those in keep.
func pruneToastIcons(dir string, keep []string, now time.Time) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return
	}
	for _, e := range entries {
		name := e.Name()
		if e.IsDir() || !(strings.HasSuffix(name, ".png") || strings.HasSuffix(name, ".tmp")) {
			continue
		}
		path := filepath.Join(dir, name)
		if slices.ContainsFunc(keep, func(k string) bool { return strings.EqualFold(k, path) }) {
			continue
		}
		if info, err := e.Info(); err == nil && now.Sub(info.ModTime()) > toastIconMaxAge {
			_ = os.Remove(path)
		}
	}
}

// Interface IDs of WinRT, from the Windows SDK headers
// (windows.ui.notifications.h, windows.data.xml.dom.h, objidl.h), checked
// against the windows crate (windows-rs 0.58, define_interface!). The three
// delegate IDs are also derived from their signatures by
// TestPinterfaceIIDs, which matches both.
const (
	iidStrInspectable              = "af86e2e0-b12d-4c6a-9c5a-d7aa65101e90"
	iidStrAgileObject              = "94ea2b94-e9cc-49e0-c0ff-ee64ca8f5b90"
	iidStrToastManagerStatics      = "50ac103f-d235-4598-bbef-98fe4d1a3ad4" // IToastNotificationManagerStatics
	iidStrToastManagerStatics2     = "7ab93c52-0e48-4750-ba9d-1a4113981847" // IToastNotificationManagerStatics2
	iidStrToastFactory             = "04124b20-82c6-4229-b109-fd9ed4662b53" // IToastNotificationFactory
	iidStrToastNotification        = "997e2675-059e-4e60-8b06-1760917c8b80" // IToastNotification
	iidStrToastNotification2       = "9dfb9fd1-143a-490e-90bf-b9fba7132de7" // IToastNotification2
	iidStrToastActivatedEventArgs  = "e3bf92f3-c197-436f-8265-0625824f8dac" // IToastActivatedEventArgs
	iidStrToastDismissedEventArgs  = "3f89d935-d9cb-4538-a0f0-ffe7659938f8" // IToastDismissedEventArgs
	iidStrToastFailedEventArgs     = "35176862-cfd4-44f8-ad64-f500fd896c3b" // IToastFailedEventArgs
	iidStrXMLDocument              = "f7f3a506-1e87-42d6-bcfb-b8c809fa5494" // IXmlDocument
	iidStrXMLDocumentIO            = "6cd0e74e-ee65-4489-9ebf-ca43e87ba637" // IXmlDocumentIO
	iidStrTypedEventHandler        = "9de1c534-6ae1-11e0-84e1-18a905bcc53f" // Windows.Foundation.TypedEventHandler`2
	iidStrToastActivatedHandler    = "ab54de2d-97d9-5528-b6ad-105afe156530" // TypedEventHandler<ToastNotification, IInspectable>
	iidStrToastDismissedHandler    = "61c2402f-0ed0-5a18-ab69-59f4aa99a368" // TypedEventHandler<ToastNotification, ToastDismissedEventArgs>
	iidStrToastFailedHandler       = "95e3e803-c969-5e3a-9753-ea2ad22a9a33" // TypedEventHandler<ToastNotification, ToastFailedEventArgs>
	toastNotificationRuntimeClass  = "Windows.UI.Notifications.ToastNotification"
	toastManagerRuntimeClass       = "Windows.UI.Notifications.ToastNotificationManager"
	toastDismissedArgsRuntimeClass = "Windows.UI.Notifications.ToastDismissedEventArgs"
	toastFailedArgsRuntimeClass    = "Windows.UI.Notifications.ToastFailedEventArgs"
	xmlDocumentRuntimeClass        = "Windows.Data.Xml.Dom.XmlDocument"
)

// The signatures of the three delegate types of a toast's events, in the
// WinRT type system's grammar: a parameterized interface's ID is not in any
// header of the type itself but derived from its signature (pinterfaceIID).
var (
	sigToastNotification     = "rc(" + toastNotificationRuntimeClass + ";{" + iidStrToastNotification + "})"
	sigToastActivatedHandler = "pinterface({" + iidStrTypedEventHandler + "};" + sigToastNotification + ";cinterface(IInspectable))"
	sigToastDismissedHandler = "pinterface({" + iidStrTypedEventHandler + "};" + sigToastNotification +
		";rc(" + toastDismissedArgsRuntimeClass + ";{" + iidStrToastDismissedEventArgs + "}))"
	sigToastFailedHandler = "pinterface({" + iidStrTypedEventHandler + "};" + sigToastNotification +
		";rc(" + toastFailedArgsRuntimeClass + ";{" + iidStrToastFailedEventArgs + "}))"
)

// pinterfaceIID returns the interface ID of an instance of a parameterized
// WinRT interface from its signature, as the WinRT type system specifies it
// (and MIDL, C++/WinRT's guid_of and the windows crate compute it): a
// version 5 UUID (RFC 4122, SHA-1) of the UTF-8 signature in the namespace
// 11f47ad5-7b73-42c0-abae-878b1e16adee. Returned lower case, without braces.
func pinterfaceIID(signature string) string {
	ns := []byte{0x11, 0xf4, 0x7a, 0xd5, 0x7b, 0x73, 0x42, 0xc0, 0xab, 0xae, 0x87, 0x8b, 0x1e, 0x16, 0xad, 0xee}
	h := sha1.Sum(append(ns, signature...))
	h[6] = h[6]&0x0F | 0x50 // version 5
	h[8] = h[8]&0x3F | 0x80 // RFC 4122 variant
	return fmt.Sprintf("%x-%x-%x-%x-%x", h[0:4], h[4:6], h[6:8], h[8:10], h[10:16])
}

// parseGUIDFields parses a GUID written as in iidStr…, without braces, into
// the fields of a GUID; it panics on a malformed literal (a programmer
// error in this file).
func parseGUIDFields(s string) (d1 uint32, d2, d3 uint16, d4 [8]byte) {
	h := strings.ReplaceAll(s, "-", "")
	raw, err := hex.DecodeString(h)
	if err != nil || len(raw) != 16 || len(s) != 36 {
		panic("win32: malformed GUID literal " + s)
	}
	d1 = uint32(raw[0])<<24 | uint32(raw[1])<<16 | uint32(raw[2])<<8 | uint32(raw[3])
	d2 = uint16(raw[4])<<8 | uint16(raw[5])
	d3 = uint16(raw[6])<<8 | uint16(raw[7])
	copy(d4[:], raw[8:])
	return d1, d2, d3, d4
}
