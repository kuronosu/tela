package platform

// Cursor is a pointer cursor shape. The names and order are those of the
// CSS cursor property as listed by the wp_cursor_shape_v1 protocol (version
// 1), so the mapping to Wayland is direct: for every c in
// [CursorDefault, CursorZoomOut], the wp_cursor_shape_device_v1.shape value
// is uint32(c) + 1. [CursorNone] hides the cursor.
//
// The CSS value "auto" is not a Cursor: it is resolved by the layers above
// (text cursor over selectable text, default elsewhere).
type Cursor uint8

const (
	CursorDefault      Cursor = iota // default
	CursorContextMenu                // context-menu
	CursorHelp                       // help
	CursorPointer                    // pointer
	CursorProgress                   // progress
	CursorWait                       // wait
	CursorCell                       // cell
	CursorCrosshair                  // crosshair
	CursorText                       // text
	CursorVerticalText               // vertical-text
	CursorAlias                      // alias
	CursorCopy                       // copy
	CursorMove                       // move
	CursorNoDrop                     // no-drop
	CursorNotAllowed                 // not-allowed
	CursorGrab                       // grab
	CursorGrabbing                   // grabbing
	CursorEResize                    // e-resize
	CursorNResize                    // n-resize
	CursorNEResize                   // ne-resize
	CursorNWResize                   // nw-resize
	CursorSResize                    // s-resize
	CursorSEResize                   // se-resize
	CursorSWResize                   // sw-resize
	CursorWResize                    // w-resize
	CursorEWResize                   // ew-resize
	CursorNSResize                   // ns-resize
	CursorNESWResize                 // nesw-resize
	CursorNWSEResize                 // nwse-resize
	CursorColResize                  // col-resize
	CursorRowResize                  // row-resize
	CursorAllScroll                  // all-scroll
	CursorZoomIn                     // zoom-in
	CursorZoomOut                    // zoom-out
	CursorNone                       // none (hidden)

	cursorCount
)

var cursorNames = [cursorCount]string{
	"default", "context-menu", "help", "pointer", "progress", "wait", "cell",
	"crosshair", "text", "vertical-text", "alias", "copy", "move", "no-drop",
	"not-allowed", "grab", "grabbing", "e-resize", "n-resize", "ne-resize",
	"nw-resize", "s-resize", "se-resize", "sw-resize", "w-resize", "ew-resize",
	"ns-resize", "nesw-resize", "nwse-resize", "col-resize", "row-resize",
	"all-scroll", "zoom-in", "zoom-out", "none",
}

// String returns the CSS name of c ("not-allowed", "ew-resize", ...).
func (c Cursor) String() string {
	if c < cursorCount {
		return cursorNames[c]
	}
	return "Cursor(invalid)"
}

// Valid reports whether c is one of the defined cursors.
func (c Cursor) Valid() bool { return c < cursorCount }

// CursorFromName returns the cursor with the given CSS name. ok is false for
// unknown names (including "auto", which the layers above resolve).
func CursorFromName(name string) (c Cursor, ok bool) {
	for i, n := range cursorNames {
		if n == name {
			return Cursor(i), true
		}
	}
	return CursorDefault, false
}
