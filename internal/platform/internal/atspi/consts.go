//go:build linux

package atspi

import "github.com/godbus/dbus/v5"

// D-Bus names of AT-SPI2 (at-spi2-core's xml/ and atspi-constants.h).
const (
	// The session bus service that knows the accessibility bus.
	a11yBusName  = "org.a11y.Bus"
	a11yBusPath  = dbus.ObjectPath("/org/a11y/bus")
	a11yBusIface = "org.a11y.Bus"
	statusIface  = "org.a11y.Status"

	// The registry on the accessibility bus.
	registryName      = "org.a11y.atspi.Registry"
	registryPath      = dbus.ObjectPath("/org/a11y/atspi/registry")
	registryIface     = "org.a11y.atspi.Registry"
	socketIface       = "org.a11y.atspi.Socket"
	desktopRootPath   = dbus.ObjectPath("/org/a11y/atspi/accessible/root")
	accessiblePrefix  = "/org/a11y/atspi/accessible/"
	appRootPath       = dbus.ObjectPath("/org/a11y/atspi/accessible/root")
	nullPath          = dbus.ObjectPath("/org/a11y/atspi/null")
	cachePath         = dbus.ObjectPath("/org/a11y/atspi/cache")
	propsIface        = "org.freedesktop.DBus.Properties"
	introspectIface   = "org.freedesktop.DBus.Introspectable"
	busIface          = "org.freedesktop.DBus"
	ifaceAccessible   = "org.a11y.atspi.Accessible"
	ifaceApplication  = "org.a11y.atspi.Application"
	ifaceComponent    = "org.a11y.atspi.Component"
	ifaceAction       = "org.a11y.atspi.Action"
	ifaceValue        = "org.a11y.atspi.Value"
	ifaceText         = "org.a11y.atspi.Text"
	ifaceEditableText = "org.a11y.atspi.EditableText"
	ifaceSelection    = "org.a11y.atspi.Selection"
	ifaceCache        = "org.a11y.atspi.Cache"
	eventObjectIface  = "org.a11y.atspi.Event.Object"
	eventWindowIface  = "org.a11y.atspi.Event.Window"

	errUnknownObject = "org.freedesktop.DBus.Error.UnknownObject"
	errInvalidArgs   = "org.freedesktop.DBus.Error.InvalidArgs"
	errUnknownProp   = "org.freedesktop.DBus.Error.UnknownProperty"
	errUnknownMethod = "org.freedesktop.DBus.Error.UnknownMethod"
)

// AtspiRole values used here (atspi-constants.h).
const (
	roleInvalid          uint32 = 0
	roleAlert            uint32 = 2
	roleCheckBox         uint32 = 7
	roleCheckMenuItem    uint32 = 8
	roleColumnHeader     uint32 = 10
	roleComboBox         uint32 = 11
	roleDialog           uint32 = 16
	roleFrame            uint32 = 23
	roleImage            uint32 = 27
	roleLabel            uint32 = 29
	roleList             uint32 = 31
	roleListItem         uint32 = 32
	roleMenu             uint32 = 33
	roleMenuBar          uint32 = 34
	roleMenuItem         uint32 = 35
	rolePageTab          uint32 = 37
	rolePageTabList      uint32 = 38
	rolePanel            uint32 = 39
	rolePasswordText     uint32 = 40
	roleProgressBar      uint32 = 42
	rolePushButton       uint32 = 43
	roleRadioButton      uint32 = 44
	roleRadioMenuItem    uint32 = 45
	roleRowHeader        uint32 = 47
	roleScrollBar        uint32 = 48
	roleScrollPane       uint32 = 49
	roleSeparator        uint32 = 50
	roleSlider           uint32 = 51
	roleSpinButton       uint32 = 52
	roleStatusBar        uint32 = 54
	roleTable            uint32 = 55
	roleTableCell        uint32 = 56
	roleToggleButton     uint32 = 62
	roleToolBar          uint32 = 63
	roleToolTip          uint32 = 64
	roleTree             uint32 = 65
	roleTreeTable        uint32 = 66
	roleUnknown          uint32 = 67
	roleParagraph        uint32 = 73
	roleApplication      uint32 = 75
	roleEmbedded         uint32 = 78
	roleEntry            uint32 = 79
	roleCaption          uint32 = 81
	roleDocumentFrame    uint32 = 82
	roleHeading          uint32 = 83
	roleSection          uint32 = 85
	roleForm             uint32 = 87
	roleLink             uint32 = 88
	roleTableRow         uint32 = 90
	roleTreeItem         uint32 = 91
	roleComment          uint32 = 97
	roleListBox          uint32 = 98
	roleNotification     uint32 = 101
	roleLevelBar         uint32 = 103
	roleBlockQuote       uint32 = 105
	roleDefinition       uint32 = 108
	roleArticle          uint32 = 109
	roleLandmark         uint32 = 110
	roleLog              uint32 = 111
	roleMarquee          uint32 = 112
	roleMath             uint32 = 113
	roleTimer            uint32 = 115
	roleStatic           uint32 = 116
	roleSubscript        uint32 = 119
	roleSuperscript      uint32 = 120
	roleDescriptionTerm  uint32 = 122
	roleDescriptionValue uint32 = 123
	roleContentDeletion  uint32 = 125
	roleContentInsertion uint32 = 126
	roleMark             uint32 = 127
	rolePushButtonMenu   uint32 = 129
	roleSwitch           uint32 = 130
	roleLastDefined      uint32 = 131
)

// roleNames are atspi_role_get_name's names, by AtspiRole.
var roleNames = [roleLastDefined]string{
	"invalid", "accelerator label", "alert", "animation", "arrow", "calendar", "canvas", "check box",
	"check menu item", "color chooser", "column header", "combo box", "date editor", "desktop icon",
	"desktop frame", "dial", "dialog", "directory pane", "drawing area", "file chooser", "filler",
	"focus traversable", "font chooser", "frame", "glass pane", "html container", "icon", "image",
	"internal frame", "label", "layered pane", "list", "list item", "menu", "menu bar", "menu item",
	"option pane", "page tab", "page tab list", "panel", "password text", "popup menu", "progress bar",
	"button", "radio button", "radio menu item", "root pane", "row header", "scroll bar", "scroll pane",
	"separator", "slider", "spin button", "split pane", "status bar", "table", "table cell",
	"table column header", "table row header", "tearoff menu item", "terminal", "text", "toggle button",
	"tool bar", "tool tip", "tree", "tree table", "unknown", "viewport", "window", "extended", "header",
	"footer", "paragraph", "ruler", "application", "autocomplete", "editbar", "embedded", "entry", "chart",
	"caption", "document frame", "heading", "page", "section", "redundant object", "form", "link",
	"input method window", "table row", "tree item", "document spreadsheet", "document presentation",
	"document text", "document web", "document email", "comment", "list box", "grouping", "image map",
	"notification", "info bar", "level bar", "title bar", "block quote", "audio", "video", "definition",
	"article", "landmark", "log", "marquee", "math", "rating", "timer", "static", "math fraction",
	"math root", "subscript", "superscript", "description list", "description term", "description value",
	"footnote", "content deletion", "content insertion", "mark", "suggestion", "push button menu", "switch",
}

// roleName returns the AT-SPI name of role r.
func roleName(r uint32) string {
	if r < roleLastDefined {
		return roleNames[r]
	}
	return "unknown"
}

// AtspiStateType values (atspi-constants.h), bits of a stateSet.
const (
	stateActive                 = 1
	stateBusy                   = 3
	stateChecked                = 4
	stateCollapsed              = 5
	stateDefunct                = 6
	stateEditable               = 7
	stateEnabled                = 8
	stateExpandable             = 9
	stateExpanded               = 10
	stateFocusable              = 11
	stateFocused                = 12
	stateHorizontal             = 14
	stateModal                  = 16
	stateMultiLine              = 17
	stateMultiselectable        = 18
	statePressed                = 20
	stateResizable              = 21
	stateSelectable             = 22
	stateSelected               = 23
	stateSensitive              = 24
	stateShowing                = 25
	stateSingleLine             = 26
	stateVertical               = 29
	stateVisible                = 30
	stateIndeterminate          = 32
	stateRequired               = 33
	stateInvalidEntry           = 36
	stateSupportsAutocompletion = 37
	stateSelectableText         = 38
	stateCheckable              = 41
	stateHasPopup               = 42
	stateReadOnly               = 43
	stateLastDefined            = 44
)

// stateNames are the names of the states in object:state-changed events
// (the nicks of AtspiStateType), by state.
var stateNames = [stateLastDefined]string{
	"invalid", "active", "armed", "busy", "checked", "collapsed", "defunct", "editable", "enabled",
	"expandable", "expanded", "focusable", "focused", "has-tooltip", "horizontal", "iconified", "modal",
	"multi-line", "multiselectable", "opaque", "pressed", "resizable", "selectable", "selected",
	"sensitive", "showing", "single-line", "stale", "transient", "vertical", "visible",
	"manages-descendants", "indeterminate", "required", "truncated", "animated", "invalid-entry",
	"supports-autocompletion", "selectable-text", "is-default", "visited", "checkable", "has-popup",
	"read-only",
}

// AtspiRelationType values.
const (
	relationControllerFor = 3
	relationControlledBy  = 4
)

// AtspiCoordType values.
const (
	coordScreen = 0
	coordWindow = 1
	coordParent = 2
)

// AtspiComponentLayer values.
const (
	layerWidget = 3
	layerWindow = 7
)

// AtspiLive values: the politeness of an Announcement.
const (
	livePolite    = 1
	liveAssertive = 2
)
