//go:build darwin

package cocoa

import (
	"github.com/kuronosu/tela/internal/darwin"
)

// TelaAccessElement's attribute API: the older NSAccessibility informal
// protocol that the AX server calls for an AT (see "the attribute API" in
// access.go for why the element implements it). Each attribute is
// answered by sending the element its own NSAccessibility method (the
// axAttrs table), so both APIs give the same answers; values are boxed
// as the API expects (NSNumber for BOOL and integers, NSValue for
// geometry and ranges). Nothing here sends super: NSAccessibilityElement's
// versions read its own stored properties, which the element never sets.

// axSels caches the selectors of the attribute table (main thread).
var axSels = map[string]darwin.SEL{}

func axSelOf(name string) darwin.SEL {
	s, ok := axSels[name]
	if !ok {
		s = darwin.Sel(name)
		axSels[name] = s
	}
	return s
}

// responds reports whether obj responds to name: arguments from AT
// clients are checked before they are sent anything.
func responds(obj darwin.ID, name string) bool {
	return obj != 0 && darwin.SendBool(obj, axSelOf("respondsToSelector:"), axSelOf(name))
}

// attrValue returns the value of the attribute name of element self.
func attrValue(self darwin.ID, c axCall, name string) darwin.ID {
	a, ok := axFindAttr(axAttrs, name)
	if !ok || !axAttrApplies(c.v, c.n, a) {
		return 0
	}
	s := axSelOf(a.sel)
	switch a.kind {
	case attrObject:
		return darwin.Send(self, s)
	case attrBool:
		return darwin.NSBool(darwin.SendBool(self, s))
	case attrInt:
		return darwin.NSNumberInt(darwin.SendInt(self, s))
	case attrRange:
		return darwin.NSValueRange(darwin.SendRange(self, s))
	case attrRect:
		return darwin.NSValueRect(darwin.SendRect(self, s))
	case attrPosition:
		return darwin.NSValuePoint(darwin.SendRect(self, s).Origin)
	case attrSize:
		return darwin.NSValueSize(darwin.SendRect(self, s).Size)
	case attrParent:
		p := darwin.Send(self, s)
		if p == c.w.view {
			// The view is ignored: the AT's parent is the window.
			if u := darwin.AccessibilityUnignoredAncestor(p); u != 0 {
				return u
			}
			return c.w.ns
		}
		return p
	case attrOrientation:
		return axStr(axOrientationValue(c.n))
	}
	return 0
}

// paramValue returns the value of the parameterized attribute name of
// element self for param (an NSValue range or an NSNumber).
func paramValue(self darwin.ID, c axCall, name string, param darwin.ID) darwin.ID {
	a, ok := axFindAttr(axParamAttrs, name)
	if !ok {
		return 0
	}
	text, isText := axText(c.n)
	if !isText {
		return 0
	}
	s := axSelOf(a.sel)
	switch name {
	case "AXLineForIndex", "AXRangeForLine", "AXRangeForIndex":
		if !responds(param, "integerValue") {
			return 0
		}
		i := darwin.SendInt(param, axSelOf("integerValue"))
		switch name {
		case "AXLineForIndex":
			return darwin.NSNumberInt(darwin.SendInt(self, s, i))
		case "AXRangeForLine":
			return darwin.NSValueRange(darwin.SendRange(self, s, i))
		}
		loc, n, ok := u16CharRange(text, i)
		if !ok {
			return 0
		}
		return darwin.NSValueRange(darwin.NSRange{Location: uint(loc), Length: uint(n)})
	}
	if !responds(param, "rangeValue") {
		return 0
	}
	r := darwin.SendRange(param, axSelOf("rangeValue"))
	switch name {
	case "AXStringForRange":
		return darwin.Send(self, s, r)
	case "AXAttributedStringForRange":
		str := darwin.Send(self, s, r)
		if str == 0 {
			return 0
		}
		return darwin.NSAttributedStringAutoreleased(darwin.GoString(str))
	case "AXBoundsForRange":
		return darwin.NSValueRect(darwin.SendRect(self, s, r))
	}
	return 0
}

// setAttr sets the attribute name of element self to value through the
// attribute's setter.
func setAttr(self darwin.ID, c axCall, name string, value darwin.ID) {
	if !axSettable(c.v, c.n, name) {
		return
	}
	a, _ := axFindAttr(axAttrs, name)
	s := axSelOf(a.set)
	switch a.set {
	case axSelSetValue:
		darwin.Send(self, s, value)
	case axSelSetSelectedRange:
		if responds(value, "rangeValue") {
			darwin.Send(self, s, darwin.SendRange(value, axSelOf("rangeValue")))
		}
	default: // the BOOL setters
		if responds(value, "boolValue") {
			darwin.Send(self, s, darwin.SendBool(value, axSelOf("boolValue")))
		}
	}
}

// attributeMethods are TelaAccessElement's methods of the attribute API.
func attributeMethods() []darwin.Method {
	names := func(list func(c axCall) []string) func(self darwin.ID, _ darwin.SEL) darwin.ID {
		return func(self darwin.ID, _ darwin.SEL) darwin.ID {
			out := darwin.NewMutableArray()
			axQuery(self, func(c axCall) {
				for _, n := range list(c) {
					darwin.ArrayAdd(out, axStr(n))
				}
			})
			return out
		}
	}
	// arrayValue returns the attribute's value when it is an array.
	arrayValue := func(self darwin.ID, attr darwin.ID) []darwin.ID {
		var items []darwin.ID
		axQuery(self, func(c axCall) {
			if v := attrValue(self, c, darwin.GoString(attr)); v != 0 && responds(v, "objectAtIndex:") {
				items = darwin.ArrayItems(v)
			}
		})
		return items
	}
	return []darwin.Method{
		{Sel: darwin.Sel("accessibilityAttributeNames"), Fn: names(func(c axCall) []string { return axAttributeNames(c.v, c.n) })},
		{Sel: darwin.Sel("accessibilityParameterizedAttributeNames"), Fn: names(func(c axCall) []string { return axParamAttributeNames(c.n) })},
		{Sel: darwin.Sel("accessibilityAttributeValue:"), Fn: func(self darwin.ID, _ darwin.SEL, attr darwin.ID) darwin.ID {
			var res darwin.ID
			axQuery(self, func(c axCall) { res = attrValue(self, c, darwin.GoString(attr)) })
			return res
		}},
		{Sel: darwin.Sel("accessibilityAttributeValue:forParameter:"), Fn: func(self darwin.ID, _ darwin.SEL, attr, param darwin.ID) darwin.ID {
			var res darwin.ID
			axQuery(self, func(c axCall) { res = paramValue(self, c, darwin.GoString(attr), param) })
			return res
		}},
		{Sel: darwin.Sel("accessibilityIsAttributeSettable:"), Fn: func(self darwin.ID, _ darwin.SEL, attr darwin.ID) bool {
			res := false
			axQuery(self, func(c axCall) { res = axSettable(c.v, c.n, darwin.GoString(attr)) })
			return res
		}},
		{Sel: darwin.Sel("accessibilitySetValue:forAttribute:"), Fn: func(self darwin.ID, _ darwin.SEL, value, attr darwin.ID) {
			axQuery(self, func(c axCall) { setAttr(self, c, darwin.GoString(attr), value) })
		}},
		// A dead element is ignored.
		{Sel: darwin.Sel("accessibilityIsIgnored"), Fn: func(self darwin.ID, _ darwin.SEL) bool {
			ignored := true
			axQuery(self, func(axCall) { ignored = false })
			return ignored
		}},
		{Sel: darwin.Sel("accessibilityArrayAttributeCount:"), Fn: func(self darwin.ID, _ darwin.SEL, attr darwin.ID) uint {
			return uint(len(arrayValue(self, attr)))
		}},
		{Sel: darwin.Sel("accessibilityArrayAttributeValues:index:maxCount:"), Fn: func(self darwin.ID, _ darwin.SEL, attr darwin.ID, index, maxCount uint) darwin.ID {
			items := arrayValue(self, attr)
			start := min(int(min(index, 1<<31)), len(items))
			end := min(start+int(min(maxCount, 1<<31)), len(items))
			return darwin.NSArray(items[start:end]...)
		}},
		{Sel: darwin.Sel("accessibilityIndexOfChild:"), Fn: func(self darwin.ID, _ darwin.SEL, child darwin.ID) uint {
			idx := uint(darwin.NSNotFound)
			axQuery(self, func(c axCall) {
				for i, k := range darwin.ArrayItems(darwin.Send(self, axSelOf("accessibilityChildren"))) {
					if k == child {
						idx = uint(i)
						return
					}
				}
			})
			return idx
		}},
	}
}
