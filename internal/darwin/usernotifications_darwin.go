//go:build darwin

package darwin

import (
	"sync"

	"github.com/ebitengine/purego/objc"
)

// The UserNotifications framework (macOS 10.14+), for the notifications
// of the cocoa backend. Its classes are reached through the Objective-C
// runtime once LoadUserNotifications opened it. UNUserNotificationCenter
// raises an Objective-C exception (which cannot unwind through Go) in a
// process that has no bundle identifier, so callers check the main bundle
// first.

const frameworkUN = "/System/Library/Frameworks/UserNotifications.framework/UserNotifications"

// UNAuthorizationOptions, UNNotificationPresentationOptions and
// UNNotificationCategoryOptions bits (UNUserNotificationCenter.h,
// UNNotificationCategory.h).
const (
	UNAuthorizationOptionBadge = 1 << 0
	UNAuthorizationOptionSound = 1 << 1
	UNAuthorizationOptionAlert = 1 << 2

	UNNotificationPresentationOptionBadge  = 1 << 0
	UNNotificationPresentationOptionSound  = 1 << 1
	UNNotificationPresentationOptionList   = 1 << 3 // macOS 11+
	UNNotificationPresentationOptionBanner = 1 << 4 // macOS 11+

	// UNNotificationCategoryOptionCustomDismissAction makes the center
	// report a dismissal to the delegate (UNNotificationDismissActionIdentifier).
	UNNotificationCategoryOptionCustomDismissAction = 1 << 0
)

// The action identifiers of a UNNotificationResponse that are not the
// app's: the user clicked the notification itself, or dismissed it. Set by
// LoadUserNotifications (the framework's NSString constants, or strings of
// their documented values).
var (
	UNNotificationDefaultActionIdentifier string
	UNNotificationDismissActionIdentifier string
)

var (
	unOnce sync.Once
	unOK   bool
)

// LoadUserNotifications opens the UserNotifications framework once and
// reports whether its UNUserNotificationCenter class exists. Call it on
// the main thread, after Load.
func LoadUserNotifications() bool {
	unOnce.Do(func() {
		UNNotificationDefaultActionIdentifier = "com.apple.UNNotificationDefaultActionIdentifier"
		UNNotificationDismissActionIdentifier = "com.apple.UNNotificationDismissActionIdentifier"
		err := loadOptional(frameworkUN, func(lib uintptr) {
			if v := ID(symbolValue(lib, "UNNotificationDefaultActionIdentifier")); v != 0 {
				UNNotificationDefaultActionIdentifier = GoString(v)
			}
			if v := ID(symbolValue(lib, "UNNotificationDismissActionIdentifier")); v != 0 {
				UNNotificationDismissActionIdentifier = GoString(v)
			}
		})
		unOK = err == nil && objc.GetClass("UNUserNotificationCenter") != 0
	})
	return unOK
}
