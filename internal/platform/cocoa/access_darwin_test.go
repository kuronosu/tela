//go:build darwin

package cocoa

import (
	"fmt"
	"image/color"
	"math"
	"os"
	"runtime"
	"strings"
	"testing"
	"time"

	"github.com/kuronosu/tela/internal/darwin"
	"github.com/kuronosu/tela/internal/platform"
)

// Tests of the NSAccessibility bridge on a real window: the tests call
// TelaView's and the elements' NSAccessibility methods through
// objc_msgSend, as AppKit does for an assistive technology (so the
// CGRect, CGPoint and NSRange arguments and results cross purego's
// callbacks as AppKit passes them), and read the notifications through
// accessTrace (AppKit sends them to the AX server, which an in-process
// observer cannot see without the accessibility permission). Everything
// observed is logged.

// axUnrecognized collects the messages the test did not send because the
// receiver does not respond to them: an unrecognized selector raises an
// Objective-C exception, which aborts the whole test binary. Each step
// reports them as errors (axReportUnrecognized).
var axUnrecognized []string

// axResponds reports whether obj responds to name (a nil obj answers
// nothing without being recorded: messages to nil return 0).
func axResponds(obj darwin.ID, name string) bool {
	if obj == 0 {
		return false
	}
	if darwin.SendBool(obj, darwin.Sel("respondsToSelector:"), darwin.Sel(name)) {
		return true
	}
	cls := darwin.GoString(darwin.Send(darwin.Send(obj, darwin.Sel("class")), darwin.Sel("description")))
	axUnrecognized = append(axUnrecognized, fmt.Sprintf("-[%s %s]", cls, name))
	return false
}

// axReportUnrecognized fails the test for every message skipped since the
// last report.
func axReportUnrecognized(t *testing.T, step string) {
	t.Helper()
	for _, m := range axUnrecognized {
		t.Errorf("%s: %s is not implemented (not sent)", step, m)
	}
	axUnrecognized = nil
}

// axSend sends the message name to obj when obj responds to it.
func axSend(obj darwin.ID, name string, args ...any) darwin.ID {
	if !axResponds(obj, name) {
		return 0
	}
	return darwin.Send(obj, darwin.Sel(name), args...)
}

func axGoString(obj darwin.ID, name string) string { return darwin.GoString(axSend(obj, name)) }

func axBool(obj darwin.ID, name string, args ...any) bool {
	return axResponds(obj, name) && darwin.SendBool(obj, darwin.Sel(name), args...)
}

func axInt(obj darwin.ID, name string, args ...any) int {
	if !axResponds(obj, name) {
		return -1
	}
	return darwin.SendInt(obj, darwin.Sel(name), args...)
}

func axFloat(obj darwin.ID, name string, args ...any) float64 {
	if !axResponds(obj, name) {
		return math.NaN()
	}
	return darwin.SendFloat(obj, darwin.Sel(name), args...)
}

func axRangeOf(obj darwin.ID, name string, args ...any) darwin.NSRange {
	if !axResponds(obj, name) {
		return darwin.NSRange{Location: darwin.NSNotFound}
	}
	return darwin.SendRange(obj, darwin.Sel(name), args...)
}

func axRect(obj darwin.ID, name string, args ...any) darwin.CGRect {
	if !axResponds(obj, name) {
		return darwin.CGRect{}
	}
	return darwin.SendRect(obj, darwin.Sel(name), args...)
}

// axProbe reports whether obj responds to name without recording it: for
// the optional name-based API.
func axProbe(obj darwin.ID, name string) bool {
	return obj != 0 && darwin.SendBool(obj, darwin.Sel("respondsToSelector:"), darwin.Sel(name))
}

// axDescribe returns a line about element e: role, subrole, label and
// value.
func axDescribe(e darwin.ID) string {
	if e == 0 {
		return "<nil>"
	}
	val := axSend(e, "accessibilityValue")
	v := "nil"
	if val != 0 {
		v = fmt.Sprintf("%q", darwin.GoString(axSend(val, "description")))
	}
	return fmt.Sprintf("%s/%s %q value=%s desc=%q", axGoString(e, "accessibilityRole"), axGoString(e, "accessibilitySubrole"),
		axGoString(e, "accessibilityLabel"), v, axGoString(e, "accessibilityRoleDescription"))
}

// axTestTree is the first tree the test pushes: a heading with its text,
// a layout div holding a button and a checkbox, a text field, a password
// field and a slider; the button has the focus.
func axTestTree() platform.AccessUpdate {
	root := platform.AccessNode{ID: 1, Kind: platform.AccessRoot, Children: []platform.AccessID{2, 4, 8, 9, 10}}
	heading := platform.AccessNode{ID: 2, Role: "heading", Name: "Settings", Level: 1, Children: []platform.AccessID{3},
		Bounds: platform.Rect{X: 10, Y: 10, W: 180, H: 24}}
	text := platform.AccessNode{ID: 3, Kind: platform.AccessText, Text: "Settings", Bounds: platform.Rect{X: 10, Y: 10, W: 80, H: 24}}
	div := platform.AccessNode{ID: 4, Role: "generic", Children: []platform.AccessID{5, 7}, Bounds: platform.Rect{X: 0, Y: 40, W: 200, H: 30}}
	button := platform.AccessNode{ID: 5, Role: "button", Name: "Save", Children: []platform.AccessID{6},
		States: platform.AccessFocusable | platform.AccessFocused,
		Actions: platform.AccessActionsOf(platform.AccessActionDefault, platform.AccessActionFocus,
			platform.AccessActionScrollIntoView, platform.AccessActionShowMenu),
		Bounds: platform.Rect{X: 10, Y: 40, W: 80, H: 30}}
	label := platform.AccessNode{ID: 6, Kind: platform.AccessText, Text: "Save", Bounds: platform.Rect{X: 20, Y: 45, W: 40, H: 20}}
	check := platform.AccessNode{ID: 7, Role: "checkbox", Name: "Notify", Checked: platform.ToggleTrue,
		Actions: platform.AccessActionsOf(platform.AccessActionDefault), Bounds: platform.Rect{X: 100, Y: 40, W: 90, H: 30}}
	field := platform.AccessNode{ID: 8, Role: "textbox", Name: "Email", Value: "a😀b", Placeholder: "you@example.com",
		HasSelection: true, Selection: platform.AccessSelection{Anchor: 1, Focus: 2},
		Actions: platform.AccessActionsOf(platform.AccessActionSetValue, platform.AccessActionSetSelection, platform.AccessActionFocus),
		States:  platform.AccessFocusable, Bounds: platform.Rect{X: 10, Y: 75, W: 180, H: 20}}
	password := platform.AccessNode{ID: 9, Role: "textbox", Name: "Password", Value: "••••",
		States: platform.AccessFocusable | platform.AccessProtected, Bounds: platform.Rect{X: 10, Y: 100, W: 180, H: 20}}
	slider := platform.AccessNode{ID: 10, Role: "slider", Name: "Volume", Value: "30",
		Range:   platform.AccessRange{Now: 30, HasNow: true, Min: 0, HasMin: true, Max: 100, HasMax: true},
		Actions: platform.AccessActionsOf(platform.AccessActionIncrement, platform.AccessActionDecrement),
		Bounds:  platform.Rect{X: 10, Y: 125, W: 180, H: 20}}
	return platform.AccessUpdate{Nodes: []platform.AccessNode{root, heading, text, div, button, label, check, field, password, slider},
		Root: 1, Focus: 5}
}

// posts returns the notification lines of an access trace (the hosts
// also trace the messages AppKit sends them).
func posts(trace []string) []string {
	var out []string
	for _, l := range trace {
		if strings.HasPrefix(l, "post ") {
			out = append(out, l)
		}
	}
	return out
}

func actionString(e platform.AccessActionEvent) string {
	s := fmt.Sprintf("%v %d", e.Action, e.Node)
	switch e.Action {
	case platform.AccessActionSetValue:
		s += fmt.Sprintf(" %q", e.Value)
	case platform.AccessActionSetSelection:
		s += fmt.Sprintf(" %d-%d", e.Selection.Anchor, e.Selection.Focus)
	}
	return s
}

// TestAccessibilityBridge activates the bridge as an AT does, pushes a
// tree, walks it through the NSAccessibility methods, performs actions
// and checks the AccessActionEvents, then pushes a second update and
// checks its notifications and the dead element of a removed node.
func TestAccessibilityBridge(t *testing.T) {
	var trace []string
	accessTrace = func(format string, args ...any) {
		l := fmt.Sprintf(format, args...)
		trace = append(trace, l)
		t.Logf("access: %s", l)
	}
	t.Cleanup(func() { accessTrace = nil })
	check := func(what string, got, want any) {
		t.Helper()
		if fmt.Sprint(got) != fmt.Sprint(want) {
			t.Errorf("%s = %v, want %v", what, got, want)
		} else {
			t.Logf("%s = %v", what, got)
		}
	}
	var buttonEl darwin.ID
	activations := func(s *session) int {
		n := 0
		for _, e := range s.events {
			if e, ok := e.(platform.AccessibilityEvent); ok {
				n++
				if !e.Active {
					t.Errorf("AccessibilityEvent{Active: false}: the bridge never deactivates")
				}
			}
		}
		return n
	}
	steps := []func(s *session){
		// 1. An AT asks the view for its children: nothing yet, and the
		// bridge activates (once).
		func(s *session) {
			if axAccess.class == 0 {
				t.Errorf("TelaAccessElement is not registered: NSAccessibilityElement or AppKit's functions are missing")
				return
			}
			t.Logf("bridge active before the test's first query: %v (true: something in the process asked already)", s.w.acc.active.Load())
			check("children before the first tree", len(darwin.ArrayItems(axSend(s.w.view, "accessibilityChildren"))), 0)
			axSend(s.w.view, "accessibilityFocusedUIElement")
			check("active", s.w.acc.active.Load(), true)
			axReportUnrecognized(t, "step 1")
		},
		// 2. The app pushes the tree; the test walks it.
		func(s *session) {
			check("AccessibilityEvents after the first queries", activations(s), 1)
			trace = nil
			s.w.SetAccessibility(axTestTree())
			check("notifications of the first tree", strings.Join(posts(trace), "; "), "post AXLayoutChanged")
			v := s.w.view
			kids := darwin.ArrayItems(axSend(v, "accessibilityChildren"))
			var lines []string
			for _, k := range kids {
				lines = append(lines, axDescribe(k))
			}
			t.Logf("view children:\n  %s", strings.Join(lines, "\n  "))
			if len(kids) != 6 {
				t.Errorf("the view has %d children, want 6 (heading, button, checkbox, field, password, slider)", len(kids))
				return
			}
			heading, button, checkbox, field, password, slider := kids[0], kids[1], kids[2], kids[3], kids[4], kids[5]
			buttonEl = darwin.Retain(button)
			roles := []string{}
			for _, k := range kids {
				roles = append(roles, axGoString(k, "accessibilityRole"))
			}
			check("roles", roles, []string{"AXHeading", "AXButton", "AXCheckBox", "AXTextField", "AXTextField", "AXSlider"})
			check("button label", axGoString(button, "accessibilityLabel"), "Save")
			check("button children (presentational)", len(darwin.ArrayItems(axSend(button, "accessibilityChildren"))), 0)
			check("button is an element", axBool(button, "isAccessibilityElement"), true)
			check("button enabled", axBool(button, "isAccessibilityEnabled"), true)
			check("button focused", axBool(button, "isAccessibilityFocused"), true)
			check("view's focused element is the button", axSend(v, "accessibilityFocusedUIElement") == button, true)
			check("button parent is the view", axSend(button, "accessibilityParent") == v, true)
			check("button window", axSend(button, "accessibilityWindow") == s.w.ns, true)
			check("heading level (value)", axInt(axSend(heading, "accessibilityValue"), "integerValue"), 1)
			hk := darwin.ArrayItems(axSend(heading, "accessibilityChildren"))
			if len(hk) != 1 {
				t.Errorf("the heading has %d children, want its static text", len(hk))
			} else {
				t.Logf("heading's child: %s", axDescribe(hk[0]))
				check("heading text role", axGoString(hk[0], "accessibilityRole"), "AXStaticText")
				check("heading text value", axGoString(hk[0], "accessibilityValue"), "Settings")
				check("text's parent is the heading", axSend(hk[0], "accessibilityParent") == heading, true)
			}
			check("checkbox value", axInt(axSend(checkbox, "accessibilityValue"), "integerValue"), 1)
			check("password subrole", axGoString(password, "accessibilitySubrole"), "AXSecureTextField")
			check("password value", axGoString(password, "accessibilityValue"), "••••")
			check("slider value", axFloat(axSend(slider, "accessibilityValue"), "doubleValue"), 30)
			check("slider min", axFloat(axSend(slider, "accessibilityMinValue"), "doubleValue"), 0)
			check("slider max", axFloat(axSend(slider, "accessibilityMaxValue"), "doubleValue"), 100)

			// Text, in UTF-16: "a😀b" is 4 units; the selection 1-2 in
			// code points is the emoji.
			check("field value", axGoString(field, "accessibilityValue"), "a😀b")
			check("field placeholder", axGoString(field, "accessibilityPlaceholderValue"), "you@example.com")
			check("field characters", axInt(field, "accessibilityNumberOfCharacters"), 4)
			check("field selected range", axRangeOf(field, "accessibilitySelectedTextRange"), darwin.NSRange{Location: 1, Length: 2})
			check("field selected text", axGoString(field, "accessibilitySelectedText"), "😀")
			check("field string for 3-1", darwin.GoString(axSend(field, "accessibilityStringForRange:", darwin.NSRange{Location: 3, Length: 1})), "b")
			check("field line for index 2", axInt(field, "accessibilityLineForIndex:", 2), 0)
			check("field range for line 0", axRangeOf(field, "accessibilityRangeForLine:", 0), darwin.NSRange{Length: 4})
			check("field visible range", axRangeOf(field, "accessibilityVisibleCharacterRange"), darwin.NSRange{Length: 4})
			check("field insertion line", axInt(field, "accessibilityInsertionPointLineNumber"), 0)

			// The method gate.
			allowed := func(e darwin.ID, name string) bool {
				return axBool(e, "isAccessibilitySelectorAllowed:", darwin.Sel(name))
			}
			check("press allowed on the button", allowed(button, "accessibilityPerformPress"), true)
			check("press allowed on the heading", allowed(heading, "accessibilityPerformPress"), false)
			check("setValue allowed on the button", allowed(button, "setAccessibilityValue:"), false)
			check("setValue allowed on the field", allowed(field, "setAccessibilityValue:"), true)
			check("role allowed on the button", allowed(button, "accessibilityRole"), true)
			names := []string{}
			for _, n := range darwin.ArrayItems(axSend(button, "accessibilityActionNames")) {
				names = append(names, darwin.GoString(n))
			}
			t.Logf("button action names: %v", names)
			if !strings.Contains(strings.Join(names, " "), axScrollToVisible) {
				t.Errorf("the button's action names %v lack %s", names, axScrollToVisible)
			}

			// The frame in screen coordinates: the content area's origin
			// plus the bounds, y flipped.
			frame := darwin.SendRect(s.w.ns, asel.frame)
			content := darwin.SendRect(s.w.ns, asel.contentRect, frame)
			got := axRect(button, "accessibilityFrame")
			want := darwin.CGRect{Origin: darwin.CGPoint{X: content.Origin.X + 10, Y: content.Origin.Y + content.Size.Height - 40 - 30},
				Size: darwin.CGSize{Width: 80, Height: 30}}
			t.Logf("window frame %v, content %v; button frame %v, want %v", frame, content, got, want)
			if math.Abs(got.Origin.X-want.Origin.X) > 0.5 || math.Abs(got.Origin.Y-want.Origin.Y) > 0.5 ||
				math.Abs(got.Size.Width-80) > 0.5 || math.Abs(got.Size.Height-30) > 0.5 {
				t.Errorf("button frame %v, want %v", got, want)
			}
			center := darwin.CGPoint{X: want.Origin.X + 40, Y: want.Origin.Y + 15}
			hit := axSend(v, "accessibilityHitTest:", center)
			check("view hit test at the button's center", axDescribe(hit), axDescribe(button))
			whit := axSend(s.w.ns, "accessibilityHitTest:", center)
			t.Logf("window hit test at %v: %s", center, axDescribe(whit))
			var wk []string
			found := false
			for _, k := range darwin.ArrayItems(axSend(s.w.ns, "accessibilityChildren")) {
				// Only the bridge's elements: AppKit's own (the title's
				// cell proxy) need not answer every method.
				cls := darwin.GoString(darwin.Send(darwin.Send(k, darwin.Sel("class")), darwin.Sel("description")))
				if !darwin.SendBool(k, darwin.Sel("isKindOfClass:"), axAccess.class) {
					wk = append(wk, cls)
					continue
				}
				wk = append(wk, axGoString(k, "accessibilityRole"))
				found = found || k == heading
			}
			t.Logf("window children: %v", wk)
			if !found {
				t.Errorf("the window's accessibilityChildren %v do not hold the heading: AppKit does not reach the view's children", wk)
			}

			// Actions: each queues an AccessActionEvent.
			check("press the button", axBool(button, "accessibilityPerformPress"), true)
			check("press the heading", axBool(heading, "accessibilityPerformPress"), false)
			check("increment the slider", axBool(slider, "accessibilityPerformIncrement"), true)
			check("decrement the button", axBool(button, "accessibilityPerformDecrement"), false)
			check("show the button's menu", axBool(button, "accessibilityPerformShowMenu"), true)
			hi := darwin.NSString("hi")
			axSend(field, "setAccessibilityValue:", hi)
			axSend(password, "setAccessibilityValue:", hi) // no set-value action: ignored
			darwin.Release(hi)
			axSend(field, "setAccessibilitySelectedTextRange:", darwin.NSRange{Location: 0, Length: 3})
			axSend(field, "setAccessibilityFocused:", true)
			axSend(button, "accessibilityPerformAction:", darwin.AutoString(axScrollToVisible))

			// The attribute API the AX server calls (accessibilityAttribute
			// Value: and the names), which AppKit maps onto the methods
			// above through their type encodings.
			// Which name-based methods exist, on the element, its superclass
			// and the window (logged; not all are expected).
			for _, m := range []string{"accessibilityAttributeNames", "accessibilityAttributeValue:",
				"accessibilityParameterizedAttributeNames", "accessibilityActionNames", "accessibilityPerformAction:",
				"accessibilityActionDescription:", "accessibilityIsIgnored"} {
				t.Logf("responds to %s: element %v, NSAccessibilityElement instances %v, window %v", m, axProbe(button, m),
					darwin.SendBool(darwin.ID(axAccess.super), asel.instancesRespond, darwin.Sel(m)), axProbe(s.w.ns, m))
			}
			attrAPI := axProbe(button, "accessibilityAttributeValue:") && axProbe(button, "accessibilityAttributeNames")
			attr := func(e darwin.ID, name string) darwin.ID {
				if !axProbe(e, "accessibilityAttributeValue:") {
					return 0
				}
				return darwin.Send(e, darwin.Sel("accessibilityAttributeValue:"), darwin.AutoString(name))
			}
			desc := func(o darwin.ID) string {
				if o == 0 {
					return "<nil>"
				}
				return darwin.GoString(axSend(o, "description"))
			}
			if !attrAPI {
				t.Logf("note: the element has no accessibilityAttributeValue: (the attribute API); its checks are skipped")
			} else {
				var attrNames []string
				for _, n := range darwin.ArrayItems(axSend(button, "accessibilityAttributeNames")) {
					attrNames = append(attrNames, darwin.GoString(n))
				}
				t.Logf("button attribute names: %v", attrNames)
				for _, name := range []string{"AXRole", "AXRoleDescription", "AXDescription", "AXEnabled", "AXFocused", "AXFrame", "AXParent", "AXWindow"} {
					t.Logf("button %s = %s", name, desc(attr(button, name)))
				}
				check("AXRole", darwin.GoString(attr(button, "AXRole")), "AXButton")
				check("AXDescription", darwin.GoString(attr(button, "AXDescription")), "Save")
				check("AXEnabled", axBool(attr(button, "AXEnabled"), "boolValue"), true)
				if f := attr(button, "AXFrame"); f != 0 {
					check("AXFrame", axRect(f, "rectValue"), got)
				} else {
					t.Errorf("AXFrame is nil")
				}
				check("AXChildren of the heading", len(darwin.ArrayItems(attr(heading, "AXChildren"))), 1)
				check("AXValue of the slider", axFloat(attr(slider, "AXValue"), "doubleValue"), 30)
				check("AXNumberOfCharacters of the field", axInt(attr(field, "AXNumberOfCharacters"), "integerValue"), 4)
				if r := attr(field, "AXSelectedTextRange"); r != 0 {
					check("AXSelectedTextRange of the field", axRangeOf(r, "rangeValue"), darwin.NSRange{Location: 1, Length: 2})
				} else {
					t.Errorf("AXSelectedTextRange is nil")
				}
				check("AXParent of the button is the window", attr(button, "AXParent") == s.w.ns, true)
				check("AXParent of the heading's text is the heading", len(hk) == 1 && attr(hk[0], "AXParent") == heading, true)
				if len(hk) == 1 {
					check("AXValue of the heading's text", darwin.GoString(attr(hk[0], "AXValue")), "Settings")
					check("AXRole of the heading's text", darwin.GoString(attr(hk[0], "AXRole")), "AXStaticText")
				}
				check("AXRole of the heading", darwin.GoString(attr(heading, "AXRole")), "AXHeading")
				check("AXSubrole of the password", darwin.GoString(attr(password, "AXSubrole")), "AXSecureTextField")
				if p := attr(button, "AXPosition"); p != 0 {
					t.Logf("button AXPosition %s AXSize %s", desc(p), desc(attr(button, "AXSize")))
				} else {
					t.Errorf("AXPosition is nil")
				}
				check("AXIsIgnored of the button", axBool(button, "accessibilityIsIgnored"), false)
				check("AXValue settable on the field", axBool(field, "accessibilityIsAttributeSettable:", darwin.AutoString("AXValue")), true)
				check("AXValue settable on the button", axBool(button, "accessibilityIsAttributeSettable:", darwin.AutoString("AXValue")), false)
				check("AXFocused settable on the button", axBool(button, "accessibilityIsAttributeSettable:", darwin.AutoString("AXFocused")), true)
				param := func(e darwin.ID, name string, p darwin.ID) darwin.ID {
					return axSend(e, "accessibilityAttributeValue:forParameter:", darwin.AutoString(name), p)
				}
				check("AXStringForRange 3-1", darwin.GoString(param(field, "AXStringForRange", darwin.NSValueRange(darwin.NSRange{Location: 3, Length: 1}))), "b")
				if as := param(field, "AXAttributedStringForRange", darwin.NSValueRange(darwin.NSRange{Location: 1, Length: 2})); as != 0 {
					check("AXAttributedStringForRange 1-2", axGoString(as, "string"), "😀")
				} else {
					t.Errorf("AXAttributedStringForRange is nil")
				}
				check("AXLineForIndex 2", axInt(param(field, "AXLineForIndex", darwin.NSNumberInt(2)), "integerValue"), 0)
				check("AXRangeForIndex 1", axRangeOf(param(field, "AXRangeForIndex", darwin.NSNumberInt(1)), "rangeValue"), darwin.NSRange{Location: 1, Length: 2})
				check("AXRangeForLine 0", axRangeOf(param(field, "AXRangeForLine", darwin.NSNumberInt(0)), "rangeValue"), darwin.NSRange{Length: 4})
				// Setting the field's selection through the attribute API.
				axSend(field, "accessibilitySetValue:forAttribute:", darwin.NSValueRange(darwin.NSRange{Location: 3, Length: 1}),
					darwin.AutoString("AXSelectedTextRange"))
				var fieldAttrs []string
				for _, n := range darwin.ArrayItems(axSend(field, "accessibilityAttributeNames")) {
					fieldAttrs = append(fieldAttrs, darwin.GoString(n))
				}
				t.Logf("field attribute names: %v", fieldAttrs)
				var params []string
				for _, n := range darwin.ArrayItems(axSend(field, "accessibilityParameterizedAttributeNames")) {
					params = append(params, darwin.GoString(n))
				}
				t.Logf("field parameterized attribute names: %v", params)
				wkids := attr(s.w.ns, "AXChildren")
				found = false
				for _, k := range darwin.ArrayItems(wkids) {
					found = found || k == heading
				}
				t.Logf("window AXChildren: %s", desc(wkids))
				if !found {
					t.Errorf("the window's AXChildren do not hold the heading")
				}
			}
			// AXPress through the attribute API reaches accessibilityPerformPress.
			axSend(button, "accessibilityPerformAction:", darwin.AutoString("AXPress"))
			axReportUnrecognized(t, "step 2")
		},
		// 3. The actions arrived; a second update.
		func(s *session) {
			var acts []string
			for _, e := range s.events {
				if e, ok := e.(platform.AccessActionEvent); ok {
					acts = append(acts, actionString(e))
				}
			}
			check("action events", strings.Join(acts, "; "),
				`default 5; increment 10; show-menu 5; set-value 8 "hi"; set-selection 8 0-2; focus 8; scroll-into-view 5; set-selection 8 2-3; default 5`)
			u := axTestTree()
			nodes := map[platform.AccessID]platform.AccessNode{}
			for _, n := range u.Nodes {
				nodes[n.ID] = n
			}
			heading, div, checkbox := nodes[2], nodes[4], nodes[7]
			heading.Name = "Preferences"
			div.Children = []platform.AccessID{7}
			checkbox.Checked = platform.ToggleFalse
			trace = nil
			s.w.SetAccessibility(platform.AccessUpdate{Nodes: []platform.AccessNode{heading, div, checkbox}, Root: 1, Focus: 8,
				Announcements: []platform.Announcement{{Text: "Saved", Assertive: true}}})
			check("notifications of the second update", strings.Join(posts(trace), "; "),
				`post AXUIElementDestroyed 5; post AXLayoutChanged; post AXTitleChanged 2; post AXValueChanged 7; `+
					`post AXFocusedUIElementChanged 8; post AXAnnouncementRequested "Saved" high`)
			kids := darwin.ArrayItems(axSend(s.w.view, "accessibilityChildren"))
			check("view children after the removal", len(kids), 5)
			if buttonEl != 0 {
				check("removed button is an element", axBool(buttonEl, "isAccessibilityElement"), false)
				check("removed button role", axGoString(buttonEl, "accessibilityRole"), "")
				check("removed button press", axBool(buttonEl, "accessibilityPerformPress"), false)
				darwin.Release(buttonEl)
				buttonEl = 0
			}
			if len(kids) > 0 {
				check("renamed heading", axGoString(kids[0], "accessibilityLabel"), "Preferences")
			}
			focused := axSend(s.w.view, "accessibilityFocusedUIElement")
			t.Logf("focused element after the update: %s", axDescribe(focused))
			check("focused element is the field", len(kids) == 5 && focused == kids[2], true)
			axReportUnrecognized(t, "step 3")
		},
	}
	s := runSteps(t, "tela accessibility", false, 300*time.Millisecond, steps...)
	if s == nil {
		t.Fatal("the window never showed")
	}
	check("AccessibilityEvents in the session", activations(s), 1)
}

// TestAccessibilityAXClient reads the window's tree through the
// AXUIElement API from another thread, as VoiceOver does from its own
// process. It needs the accessibility permission (AXIsProcessTrusted),
// which CI runners usually lack: it skips without it.
func TestAccessibilityAXClient(t *testing.T) {
	if err := darwin.LoadAXClient(); err != nil {
		t.Skipf("ApplicationServices: %v", err)
	}
	if !darwin.AXIsProcessTrusted() {
		t.Skip("the process is not trusted for accessibility (System Settings > Privacy & Security > Accessibility); skipping the AXUIElement client test")
	}
	// The accessibility messages the hosts and elements receive, as they
	// arrive (from the main thread; t.Logf is safe there).
	accessTrace = func(format string, args ...any) { t.Logf("access: "+format, args...) }
	t.Cleanup(func() { accessTrace = nil })
	results := make(chan []string, 1)
	pushed := make(chan struct{})
	const title = "tela ax client"
	client := func() {
		runtime.LockOSThread()
		defer runtime.UnlockOSThread()
		pool := darwin.PoolPush()
		defer darwin.PoolPop(pool)
		var out []string
		attr := func(el uintptr, name string) uintptr {
			s := darwin.NSString(name)
			defer darwin.Release(s)
			var v uintptr
			if err := darwin.AXUIElementCopyAttributeValue(el, uintptr(s), &v); err != 0 {
				out = append(out, fmt.Sprintf("%s: AXError %d", name, err))
				return 0
			}
			return v
		}
		app := darwin.AXUIElementCreateApplication(int32(os.Getpid()))
		defer darwin.CFRelease(app)
		var win uintptr
		if ws := attr(app, "AXWindows"); ws != 0 {
			for _, w := range darwin.ArrayItems(darwin.ID(ws)) {
				if tv := attr(uintptr(w), "AXTitle"); tv != 0 {
					if darwin.GoString(darwin.ID(tv)) == title {
						win = uintptr(darwin.Retain(w))
					}
					darwin.CFRelease(tv)
				}
			}
			darwin.CFRelease(ws)
		}
		if win == 0 {
			results <- append(out, "window not found")
			return
		}
		// str returns the description of a CF value ("" for none).
		str := func(v uintptr) string {
			if v == 0 {
				return ""
			}
			defer darwin.CFRelease(v)
			if darwin.SendBool(darwin.ID(v), darwin.Sel("isKindOfClass:"), darwin.GetClass("NSString")) {
				return darwin.GoString(darwin.ID(v))
			}
			return darwin.GoString(darwin.Send(darwin.ID(v), darwin.Sel("description")))
		}
		quiet := func(el uintptr, name string) uintptr {
			s := darwin.NSString(name)
			defer darwin.Release(s)
			var v uintptr
			if darwin.AXUIElementCopyAttributeValue(el, uintptr(s), &v) != 0 {
				return 0
			}
			return v
		}
		var save uintptr
		var walk func(el uintptr, depth int)
		walk = func(el uintptr, depth int) {
			kids := attr(el, "AXChildren")
			if depth == 0 {
				out = append(out, fmt.Sprintf("window AXChildren: %d items", len(darwin.ArrayItems(darwin.ID(kids)))))
			}
			for _, k := range darwin.ArrayItems(darwin.ID(kids)) {
				e := uintptr(k)
				role, sub := str(quiet(e, "AXRole")), str(quiet(e, "AXSubrole"))
				desc, title, value := str(quiet(e, "AXDescription")), str(quiet(e, "AXTitle")), str(quiet(e, "AXValue"))
				n := 0
				if c := quiet(e, "AXChildren"); c != 0 {
					n = len(darwin.ArrayItems(darwin.ID(c)))
					darwin.CFRelease(c)
				}
				out = append(out, fmt.Sprintf("%s%s/%s %q title=%q value=%q children=%d", strings.Repeat("  ", depth), role, sub, desc, title, value, n))
				if role == "AXButton" && desc == "Save" && save == 0 {
					save = uintptr(darwin.Retain(k))
				}
				if depth < 3 {
					walk(e, depth+1)
				}
			}
			if kids != 0 {
				darwin.CFRelease(kids)
			}
		}
		walk(win, 0) // activates the bridge
		select {
		case <-pushed:
		case <-time.After(5 * time.Second):
			out = append(out, "no tree pushed")
		}
		// The tree reaches the bridge in the pass after the activation:
		// walk again until the Save button shows up (a few seconds).
		for try := 1; try <= 8 && save == 0; try++ {
			time.Sleep(300 * time.Millisecond)
			out = append(out, fmt.Sprintf("--- walk %d after the tree", try))
			walk(win, 0)
		}
		if f := quiet(app, "AXFocusedUIElement"); f != 0 {
			out = append(out, fmt.Sprintf("focused: %s %q", str(quiet(f, "AXRole")), str(quiet(f, "AXDescription"))))
			darwin.CFRelease(f)
		} else {
			out = append(out, "focused: none")
		}
		if save != 0 {
			a := darwin.NSString("AXPress")
			out = append(out, fmt.Sprintf("AXPress on Save: AXError %d", darwin.AXUIElementPerformAction(save, uintptr(a))))
			darwin.Release(a)
			darwin.Release(darwin.ID(save))
		}
		darwin.Release(darwin.ID(win))
		results <- out
	}
	var got []string
	done := false
	poll := func(s *session) {
		if done {
			return
		}
		if s.w.acc.active.Load() && s.w.acc.tree.Load() == nil {
			s.w.SetAccessibility(axTestTree())
			close(pushed)
		}
		select {
		case got = <-results:
			done = true
		default:
		}
	}
	steps := []func(s *session){func(*session) { go client() }}
	for range 40 {
		steps = append(steps, poll)
	}
	s := runSteps(t, title, false, 300*time.Millisecond, steps...)
	if s == nil {
		t.Fatal("the window never showed")
	}
	if !done {
		t.Fatalf("the AX client did not finish")
	}
	t.Logf("AX client saw:\n  %s", strings.Join(got, "\n  "))
	if !strings.Contains(strings.Join(got, "\n"), `AXButton/ "Save"`) {
		t.Errorf("the AX client did not see the Save button")
	}
	pressed := false
	for _, e := range s.events {
		if e, ok := e.(platform.AccessActionEvent); ok {
			t.Logf("action event from the AX client: %s", actionString(e))
			pressed = pressed || (e.Node == 5 && e.Action == platform.AccessActionDefault)
		}
	}
	if !pressed {
		t.Errorf("AXPress from the AX client did not arrive as the default action of the Save button")
	}
}

// TestAccessibilityTwoWindows checks that each window's bridge activates
// on its own first query (contract v5: core keeps a bridge per window):
// the first window through its view, the second through its NSWindow.
func TestAccessibilityTwoWindows(t *testing.T) {
	accessTrace = func(format string, args ...any) { t.Logf("access: "+format, args...) }
	t.Cleanup(func() { accessTrace = nil })
	b := newBackend(t)
	events := map[*Window][]bool{}
	var got1, got2 [3]int // AccessibilityEvents of each window after each stage
	onMain(func() {
		p1, err1 := b.NewWindow(platform.WindowOptions{Title: "tela access one", Size: platform.Size{W: 160, H: 120}})
		p2, err2 := b.NewWindow(platform.WindowOptions{Title: "tela access two", Size: platform.Size{W: 160, H: 120}})
		if err1 != nil || err2 != nil {
			t.Errorf("NewWindow: %v, %v", err1, err2)
			return
		}
		w1, w2 := p1.(*Window), p2.(*Window)
		time.AfterFunc(20*time.Second, b.Quit) // a failure, not a hang
		stage := 0
		_ = b.Run(platform.HandlerFunc(func(ew platform.Window, e platform.Event) {
			switch e := e.(type) {
			case platform.FrameEvent:
				presentSolid(t, ew, color.RGBA{0x80, 0x80, 0x90, 0xff})
				if stage == 0 && w1.shown && w2.shown {
					stage = 1
					time.AfterFunc(150*time.Millisecond, b.Wake)
				}
			case platform.AccessibilityEvent:
				w := ew.(*Window)
				events[w] = append(events[w], e.Active)
				t.Logf("AccessibilityEvent{Active: %v} for %q", e.Active, w.opts.Title)
			case platform.WakeEvent:
				pool := darwin.PoolPush()
				defer darwin.PoolPop(pool)
				switch stage {
				case 1:
					got1[0], got2[0] = len(events[w1]), len(events[w2])
					t.Logf("window one's view children: %d", len(darwin.ArrayItems(axSend(w1.view, "accessibilityChildren"))))
					stage = 2
					time.AfterFunc(300*time.Millisecond, b.Wake)
				case 2:
					got1[1], got2[1] = len(events[w1]), len(events[w2])
					t.Logf("window two's children: %d", len(darwin.ArrayItems(axSend(w2.ns, "accessibilityChildren"))))
					axSend(w1.view, "accessibilityChildren") // a second query of window one
					stage = 3
					time.AfterFunc(300*time.Millisecond, b.Wake)
				case 3:
					got1[2], got2[2] = len(events[w1]), len(events[w2])
					axReportUnrecognized(t, "two windows")
					b.Quit()
				}
			}
		}))
	})
	t.Logf("AccessibilityEvents (one, two) before any query %d %d, after querying one %d %d, after querying two %d %d",
		got1[0], got2[0], got1[1], got2[1], got1[2], got2[2])
	if got1 != [3]int{0, 1, 1} || got2 != [3]int{0, 0, 1} {
		t.Errorf("activations of window one %v and two %v after each stage, want [0 1 1] and [0 0 1]", got1, got2)
	}
}
