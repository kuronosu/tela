//go:build darwin

package darwin

// AppKit constants of the native menus and the quit request (NSMenu and
// NSMenuItem are reached through the runtime).
const (
	// NSEventMaskAny matches every event type in
	// -[NSApplication nextEventMatchingMask:untilDate:inMode:dequeue:].
	NSEventMaskAny = ^uint64(0)

	// NSTerminateNow is the answer of applicationShouldTerminate: that
	// lets -[NSApplication terminate:] go on: it posts
	// NSApplicationWillTerminateNotification and calls exit(). The cocoa
	// backend never gives it (Go code after Run must still run).
	NSTerminateNow = 1
)
