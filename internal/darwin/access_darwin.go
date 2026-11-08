//go:build darwin

package darwin

import (
	"sync"

	"github.com/ebitengine/purego/objc"
)

// Accessibility: AppKit's NSAccessibility functions (notifications and
// role descriptions), NSNumber and NSURL values for the answers of
// accessibility elements, and, for tests, the AXUIElement client API of
// ApplicationServices (HIServices), opened on first use.

// NSAccessibility functions of AppKit, bound by LoadAccessibility.
var (
	nsAccessibilityPostNotification         func(element, name ID)
	nsAccessibilityPostNotificationUserInfo func(element, name, userInfo ID)
	nsAccessibilityRoleDescription          func(role, subrole ID) ID
	nsAccessibilityActionDescription        func(action ID) ID
)

var (
	accessOnce sync.Once
	accessErr  error
)

// LoadAccessibility binds AppKit's NSAccessibility functions, once (after
// Load); an error means AppKit lacks them.
func LoadAccessibility() error {
	accessOnce.Do(func() {
		accessErr = loadOptional(frameworkAppKit, func(lib uintptr) {
			bind(&nsAccessibilityPostNotification, lib, "NSAccessibilityPostNotification")
			bind(&nsAccessibilityPostNotificationUserInfo, lib, "NSAccessibilityPostNotificationWithUserInfo")
			bind(&nsAccessibilityRoleDescription, lib, "NSAccessibilityRoleDescription")
			bind(&nsAccessibilityActionDescription, lib, "NSAccessibilityActionDescription")
		})
	})
	return accessErr
}

// AccessibilityPostNotification posts the accessibility notification name
// (an NSString) for element (NSAccessibilityPostNotification). It does
// nothing before LoadAccessibility succeeded.
func AccessibilityPostNotification(element, name ID) {
	if nsAccessibilityPostNotification != nil && element != 0 && name != 0 {
		nsAccessibilityPostNotification(element, name)
	}
}

// AccessibilityPostNotificationWithUserInfo posts name for element with
// the dictionary userInfo (NSAccessibilityPostNotificationWithUserInfo).
func AccessibilityPostNotificationWithUserInfo(element, name, userInfo ID) {
	if nsAccessibilityPostNotificationUserInfo != nil && element != 0 && name != 0 {
		nsAccessibilityPostNotificationUserInfo(element, name, userInfo)
	}
}

// AccessibilityRoleDescription returns AppKit's localized description of
// the role and subrole (NSStrings; subrole may be 0), autoreleased, or 0.
func AccessibilityRoleDescription(role, subrole ID) ID {
	if nsAccessibilityRoleDescription == nil || role == 0 {
		return 0
	}
	return nsAccessibilityRoleDescription(role, subrole)
}

// AccessibilityActionDescription returns AppKit's localized description
// of the action name (an NSString such as AXPress), autoreleased, or 0.
func AccessibilityActionDescription(action ID) ID {
	if nsAccessibilityActionDescription == nil || action == 0 {
		return 0
	}
	return nsAccessibilityActionDescription(action)
}

var (
	selNumberWithDouble  = objc.RegisterName("numberWithDouble:")
	selNumberWithInteger = objc.RegisterName("numberWithInteger:")
	selURLWithString     = objc.RegisterName("URLWithString:")
)

// NSNumberFloat returns an autoreleased NSNumber holding f.
func NSNumberFloat(f float64) ID {
	return ID(foundationClasses().number).Send(selNumberWithDouble, f)
}

// NSNumberInt returns an autoreleased NSNumber holding the NSInteger i.
func NSNumberInt(i int) ID {
	return ID(foundationClasses().number).Send(selNumberWithInteger, i)
}

// URLString returns an autoreleased NSURL parsed from s, or 0 when s is
// not a valid URL (URLWithString:).
func URLString(s string) ID {
	return ID(foundationClasses().url).Send(selURLWithString, AutoString(s))
}

// The AXUIElement client API (AXUIElement.h of HIServices), bound by
// LoadAXClient: what an assistive technology calls. Only tests use it, to
// read the app's own accessibility tree the way VoiceOver does. The
// AXError results are 0 (kAXErrorSuccess) on success; values are CF
// objects owned by the caller (+1).
var (
	AXIsProcessTrusted            func() bool
	AXUIElementCreateApplication  func(pid int32) uintptr
	AXUIElementCopyAttributeValue func(element, attribute uintptr, value *uintptr) int32
	AXUIElementPerformAction      func(element, action uintptr) int32
)

const frameworkAppServices = "/System/Library/Frameworks/ApplicationServices.framework/ApplicationServices"

var (
	axClientOnce sync.Once
	axClientErr  error
)

// LoadAXClient opens ApplicationServices and binds the AXUIElement client
// functions, once.
func LoadAXClient() error {
	axClientOnce.Do(func() {
		axClientErr = loadOptional(frameworkAppServices, func(lib uintptr) {
			bind(&AXIsProcessTrusted, lib, "AXIsProcessTrusted")
			bind(&AXUIElementCreateApplication, lib, "AXUIElementCreateApplication")
			bind(&AXUIElementCopyAttributeValue, lib, "AXUIElementCopyAttributeValue")
			bind(&AXUIElementPerformAction, lib, "AXUIElementPerformAction")
		})
	})
	return axClientErr
}
