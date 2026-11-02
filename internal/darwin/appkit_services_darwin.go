//go:build darwin

package darwin

// AppKit constants of the desktop services: NSWorkspace, NSOpenPanel and
// NSSavePanel, NSAlert and NSStatusItem (their classes are reached through
// the runtime).
const (
	// NSModalResponse values: the panels' completion handlers and
	// -[NSAlert runModal]. An alert's buttons answer
	// NSAlertFirstButtonReturn + their index.
	NSModalResponseOK        = 1
	NSModalResponseCancel    = 0
	NSModalResponseStop      = -1000
	NSModalResponseAbort     = -1001
	NSAlertFirstButtonReturn = 1000

	// NSAlertStyle.
	NSAlertStyleWarning       = 0
	NSAlertStyleInformational = 1
	NSAlertStyleCritical      = 2

	// NSVariableStatusItemLength makes a status item as wide as its
	// button's content (a CGFloat).
	NSVariableStatusItemLength = -1.0

	// NSControlStateValue: a menu item's check mark.
	NSControlStateValueOff = 0
	NSControlStateValueOn  = 1
)
