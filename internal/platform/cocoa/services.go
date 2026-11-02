package cocoa

import (
	"fmt"
	"hash/fnv"
	"io/fs"
	"math"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/kuronosu/tela/internal/platform"
)

// The pure half of the desktop services other than the dialogs
// (dialog.go), tested on every OS: what the keychain's and the file
// manager's errors mean, how notifications are named, and how big a tray
// image is.

// --- secrets ---

// OSStatus codes of the keychain (SecBase.h).
const (
	errSecSuccess               = 0
	errSecUnimplemented         = -4
	errSecUserCanceled          = -128
	errSecNotAvailable          = -25291 // no keychain is available
	errSecAuthFailed            = -25293 // the keychain's password was wrong, or the prompt failed
	errSecNoSuchKeychain        = -25294
	errSecDuplicateItem         = -25299
	errSecItemNotFound          = -25300
	errSecNoDefaultKeychain     = -25307
	errSecInteractionNotAllowed = -25308 // a prompt was needed and the session cannot show one
	errSecMissingEntitlement    = -34018
)

// secretStatusError turns the OSStatus of a SecItem call into the
// platform's error (nil for errSecSuccess):
//
//   - errSecItemNotFound is ErrSecretNotFound;
//   - a dismissed prompt (errSecUserCanceled) or a failed unlock
//     (errSecAuthFailed) is ErrSecretCanceled: nothing was read or written;
//   - no keychain (errSecNotAvailable, errSecNoSuchKeychain,
//     errSecNoDefaultKeychain), a session that cannot show the unlock prompt
//     (errSecInteractionNotAllowed: an SSH login, a daemon), and a keychain
//     the process may not use (errSecMissingEntitlement, errSecUnimplemented)
//     are ErrUnsupported, so the layers above disable what needs secrets;
//   - anything else is described with its code.
func secretStatusError(what string, status int32) error {
	switch status {
	case errSecSuccess:
		return nil
	case errSecItemNotFound:
		return platform.ErrSecretNotFound
	case errSecUserCanceled, errSecAuthFailed:
		return fmt.Errorf("cocoa: %s: %w (OSStatus %d)", what, platform.ErrSecretCanceled, status)
	case errSecNotAvailable, errSecNoSuchKeychain, errSecNoDefaultKeychain:
		return fmt.Errorf("cocoa: %s: no keychain is available (OSStatus %d): %w", what, status, platform.ErrUnsupported)
	case errSecInteractionNotAllowed:
		return fmt.Errorf("cocoa: %s: the keychain needs a prompt this session cannot show (OSStatus %d): %w", what, status, platform.ErrUnsupported)
	case errSecMissingEntitlement, errSecUnimplemented:
		return fmt.Errorf("cocoa: %s: the keychain refused this process (OSStatus %d): %w", what, status, platform.ErrUnsupported)
	}
	return fmt.Errorf("cocoa: %s: keychain error (OSStatus %d)", what, status)
}

// --- trash ---

// NSError domains and codes of -[NSFileManager trashItemAtURL:...]
// (FoundationErrors.h, sys/errno.h on darwin).
const (
	cocoaErrorDomain = "NSCocoaErrorDomain"
	posixErrorDomain = "NSPOSIXErrorDomain"

	nsFileNoSuchFileError          = 4
	nsFileReadNoPermissionError    = 257
	nsFileReadNoSuchFileError      = 260
	nsFileWriteNoPermissionError   = 513
	nsFileWriteVolumeReadOnlyError = 642
	nsFeatureUnsupportedError      = 3328

	darwinEPERM      = 1
	darwinENOENT     = 2
	darwinEACCES     = 13
	darwinEXDEV      = 18
	darwinEROFS      = 30
	darwinENOTSUP    = 45
	darwinEOPNOTSUPP = 102
)

// trashError turns the NSError of a failed trashItemAtURL: into
// MoveToTrash's error. The file manager never deletes what it cannot
// trash, so the file is still in place in every case: a volume without a
// trash (NSFeatureUnsupportedError, a read-only volume) is ErrUnsupported,
// a missing file wraps fs.ErrNotExist and a denied one fs.ErrPermission;
// anything else is described.
func trashError(path, domain string, code int, desc string) error {
	if desc == "" {
		desc = fmt.Sprintf("%s error %d", domain, code)
	}
	var kind error
	switch {
	case domain == cocoaErrorDomain && (code == nsFeatureUnsupportedError || code == nsFileWriteVolumeReadOnlyError),
		domain == posixErrorDomain && (code == darwinEXDEV || code == darwinEROFS || code == darwinENOTSUP || code == darwinEOPNOTSUPP):
		return fmt.Errorf("cocoa: MoveToTrash(%q): %s; the file is still there: %w", path, desc, platform.ErrUnsupported)
	case domain == cocoaErrorDomain && (code == nsFileNoSuchFileError || code == nsFileReadNoSuchFileError),
		domain == posixErrorDomain && code == darwinENOENT:
		kind = fs.ErrNotExist
	case domain == cocoaErrorDomain && (code == nsFileReadNoPermissionError || code == nsFileWriteNoPermissionError),
		domain == posixErrorDomain && (code == darwinEPERM || code == darwinEACCES):
		kind = fs.ErrPermission
	}
	if kind != nil {
		return fmt.Errorf("cocoa: MoveToTrash(%q): %s: %w", path, desc, kind)
	}
	return fmt.Errorf("cocoa: MoveToTrash(%q): %s (%s %d); the file is still there", path, desc, domain, code)
}

// --- notifications ---

// bundled reports whether the main bundle is an application bundle that
// UNUserNotificationCenter accepts: its path ends in ".app" and it has a
// bundle identifier. A plain executable (go run, go build, a test binary)
// has neither; there, +[UNUserNotificationCenter
// currentNotificationCenter] raises an Objective-C exception, which cannot
// unwind through Go, so it is never called.
func bundled(bundlePath, identifier string) bool {
	return identifier != "" && strings.EqualFold(filepath.Ext(strings.TrimSuffix(bundlePath, "/")), ".app")
}

// errNoBundle is Notify's error outside an app bundle.
var errNoBundle = fmt.Errorf("cocoa: Notify: macOS shows notifications only for an app bundle with a bundle identifier; "+
	"run the app with `tela run` (or build it with `tela build`), not go run: %w", platform.ErrUnsupported)

// errNotificationsDenied is Notify's error once the user turned the app's
// notifications off.
var errNotificationsDenied = fmt.Errorf("cocoa: Notify: notifications are turned off for this app "+
	"(System Settings > Notifications): %w", platform.ErrUnsupported)

// noteIdentifier returns the UNNotificationRequest identifier of a
// notification: the process's prefix (unique per run, so a notification
// left in Notification Center by an earlier run is not taken for one of
// this run) and the ID.
func noteIdentifier(prefix string, id platform.NotificationID) string {
	return prefix + strconv.FormatUint(uint64(id), 10)
}

// noteID returns the ID of a request identifier made by noteIdentifier
// with prefix, or false for another run's or app's.
func noteID(prefix, identifier string) (platform.NotificationID, bool) {
	rest, ok := strings.CutPrefix(identifier, prefix)
	if !ok {
		return 0, false
	}
	n, err := strconv.ParseUint(rest, 10, 32)
	if err != nil || n == 0 {
		return 0, false
	}
	return platform.NotificationID(n), true
}

// noteEvent returns the NotificationEvent of a response with the action
// identifier action to notification id: the default action (a click on
// the notification) is NotificationActivated, the dismiss action (a
// category with the custom dismiss option) NotificationClosed, and any
// other an action of the app's.
func noteEvent(id platform.NotificationID, action, defaultAction, dismissAction string) platform.NotificationEvent {
	switch action {
	case defaultAction:
		return platform.NotificationEvent{ID: id, Kind: platform.NotificationActivated}
	case dismissAction:
		return platform.NotificationEvent{ID: id, Kind: platform.NotificationClosed}
	}
	return platform.NotificationEvent{ID: id, Kind: platform.NotificationActionInvoked, Action: action}
}

// noteCategory returns the identifier of the UNNotificationCategory of a
// notification with actions: one category per distinct list of actions
// (IDs and labels), registered once per process. Every category has the
// custom dismiss option, so a dismissal is reported.
func noteCategory(actions []platform.NotificationAction) string {
	if len(actions) == 0 {
		return "dev.telaui.notification"
	}
	h := fnv.New64a()
	for _, a := range actions {
		h.Write([]byte(a.ID))
		h.Write([]byte{0})
		h.Write([]byte(a.Label))
		h.Write([]byte{0})
	}
	return fmt.Sprintf("dev.telaui.notification.%016x", h.Sum64())
}

// --- tray ---

// trayIconPoints returns the height, in points, of a tray image in a menu
// bar thickness points tall: the bar less 2 points above and below (18 in
// the usual 22-point bar, 20 in the 24-point bar of Macs with a notch), as
// AppKit's own status items leave, and never under 16.
func trayIconPoints(thickness float64) float64 {
	if !(thickness > 0) || thickness > 100 {
		thickness = 22
	}
	return max(16, math.Round(thickness)-4)
}

// trayImageSize returns the size in points of a tray image of w x h pixels
// shown side points tall (the width keeps the image's aspect ratio).
func trayImageSize(w, h int, side float64) (float64, float64) {
	if w <= 0 || h <= 0 {
		return side, side
	}
	return math.Round(side*float64(w)/float64(h)*100) / 100, side
}
