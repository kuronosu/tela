// Copyright 2022 The AccessKit Authors. All rights reserved.
// Licensed under the Apache License, Version 2.0 or the MIT license, at
// your option. Derived from Chromium's accessibility abstraction,
// Copyright 2021 The Chromium Authors (BSD-style license).
//
// The role to control type mapping, the pattern rules and the AriaProperties
// format follow AccessKit's platforms/windows/src/node.rs, adapted to Tela's
// ARIA roles and W3C Core-AAM 1.2 (THIRD_PARTY.md).

package win32

import (
	"slices"
	"strconv"
	"strings"

	"github.com/kuronosu/tela/internal/platform"
	"github.com/kuronosu/tela/internal/platform/internal/idmap"
)

// The UI Automation bridge (ADR-246): the OS-independent half, pure Go so
// that its tests run everywhere. uia_windows.go has the COM providers.
//
// What a node exposes is decided here from the flat platform.AccessNode:
// its control type and localized control type (Core-AAM 1.2's UIA column,
// with Chromium's and AccessKit's choices where they differ: dialogs are
// windows, paragraphs groups), its properties, and which control patterns
// it supports.

// UI Automation identifiers (UIAutomationClient.h, UIAutomationCoreApi.h).
const (
	uiaButtonControlTypeID      = 50000
	uiaCheckBoxControlTypeID    = 50002
	uiaComboBoxControlTypeID    = 50003
	uiaEditControlTypeID        = 50004
	uiaHyperlinkControlTypeID   = 50005
	uiaImageControlTypeID       = 50006
	uiaListItemControlTypeID    = 50007
	uiaListControlTypeID        = 50008
	uiaMenuControlTypeID        = 50009
	uiaMenuBarControlTypeID     = 50010
	uiaMenuItemControlTypeID    = 50011
	uiaProgressBarControlTypeID = 50012
	uiaRadioButtonControlTypeID = 50013
	uiaScrollBarControlTypeID   = 50014
	uiaSliderControlTypeID      = 50015
	uiaSpinnerControlTypeID     = 50016
	uiaStatusBarControlTypeID   = 50017
	uiaTabControlTypeID         = 50018
	uiaTabItemControlTypeID     = 50019
	uiaTextControlTypeID        = 50020
	uiaToolBarControlTypeID     = 50021
	uiaToolTipControlTypeID     = 50022
	uiaTreeControlTypeID        = 50023
	uiaTreeItemControlTypeID    = 50024
	uiaCustomControlTypeID      = 50025
	uiaGroupControlTypeID       = 50026
	uiaThumbControlTypeID       = 50027
	uiaDataGridControlTypeID    = 50028
	uiaDataItemControlTypeID    = 50029
	uiaDocumentControlTypeID    = 50030
	uiaWindowControlTypeID      = 50032
	uiaPaneControlTypeID        = 50033
	uiaHeaderItemControlTypeID  = 50035
	uiaTableControlTypeID       = 50036
	uiaSeparatorControlTypeID   = 50038

	uiaInvokePatternID         = 10000
	uiaSelectionPatternID      = 10001
	uiaValuePatternID          = 10002
	uiaRangeValuePatternID     = 10003
	uiaScrollPatternID         = 10004
	uiaExpandCollapsePatternID = 10005
	uiaSelectionItemPatternID  = 10010
	uiaTextPatternID           = 10014
	uiaTogglePatternID         = 10015
	uiaScrollItemPatternID     = 10017

	uiaRuntimeIDPropertyID               = 30000
	uiaBoundingRectanglePropertyID       = 30001
	uiaControlTypePropertyID             = 30003
	uiaLocalizedControlTypePropertyID    = 30004
	uiaNamePropertyID                    = 30005
	uiaAcceleratorKeyPropertyID          = 30006
	uiaHasKeyboardFocusPropertyID        = 30008
	uiaIsKeyboardFocusablePropertyID     = 30009
	uiaIsEnabledPropertyID               = 30010
	uiaAutomationIDPropertyID            = 30011
	uiaClassNamePropertyID               = 30012
	uiaHelpTextPropertyID                = 30013
	uiaIsControlElementPropertyID        = 30016
	uiaIsContentElementPropertyID        = 30017
	uiaIsPasswordPropertyID              = 30019
	uiaNativeWindowHandlePropertyID      = 30020
	uiaIsOffscreenPropertyID             = 30022
	uiaOrientationPropertyID             = 30023
	uiaFrameworkIDPropertyID             = 30024
	uiaIsRequiredForFormPropertyID       = 30025
	uiaItemStatusPropertyID              = 30026
	uiaValueValuePropertyID              = 30045
	uiaRangeValueValuePropertyID         = 30047
	uiaExpandCollapseStatePropertyID     = 30070
	uiaSelectionItemIsSelectedPropertyID = 30079
	uiaToggleStatePropertyID             = 30086
	uiaAriaRolePropertyID                = 30101
	uiaAriaPropertiesPropertyID          = 30102
	uiaIsDataValidForFormPropertyID      = 30103
	uiaControllerForPropertyID           = 30104
	uiaLiveSettingPropertyID             = 30135
	uiaPositionInSetPropertyID           = 30152
	uiaSizeOfSetPropertyID               = 30153
	uiaLevelPropertyID                   = 30154
	uiaLandmarkTypePropertyID            = 30157
	uiaLocalizedLandmarkTypePropertyID   = 30158
	uiaFullDescriptionPropertyID         = 30159
	uiaHeadingLevelPropertyID            = 30173
	uiaIsDialogPropertyID                = 30174

	uiaStructureChangedEventID            = 20002
	uiaMenuOpenedEventID                  = 20003
	uiaAutomationFocusChangedEventID      = 20005
	uiaMenuClosedEventID                  = 20007
	uiaElementAddedToSelectionEventID     = 20010
	uiaElementRemovedFromSelectionEventID = 20011
	uiaElementSelectedEventID             = 20012
	uiaTextSelectionChangedEventID        = 20014
	uiaTextChangedEventID                 = 20015
	uiaWindowOpenedEventID                = 20016
	uiaLiveRegionChangedEventID           = 20024

	uiaCustomLandmarkTypeID     = 80000
	uiaFormLandmarkTypeID       = 80001
	uiaMainLandmarkTypeID       = 80002
	uiaNavigationLandmarkTypeID = 80003
	uiaSearchLandmarkTypeID     = 80004
	uiaHeadingLevel1            = 80051

	// ToggleState, ExpandCollapseState, LiveSetting, OrientationType.
	uiaToggleOff            = 0
	uiaToggleOn             = 1
	uiaToggleIndeterminate  = 2
	uiaCollapsed            = 0
	uiaExpanded             = 1
	uiaLeafNode             = 3
	uiaLiveOff              = 0
	uiaLivePolite           = 1
	uiaLiveAssertive        = 2
	uiaOrientationNone      = 0
	uiaOrientationHorizonal = 1
	uiaOrientationVertical  = 2

	// NotificationKind_Other and the NotificationProcessing values.
	uiaNotificationKindOther             = 4
	uiaProcessingImportantMostRecent     = 1
	uiaProcessingAll                     = 2
	uiaStructureChildrenInvalidated      = 2
	uiaCaretPositionUnknown              = 0
	uiaCaretPositionEndOfLine            = 1
	uiaCaretPositionBeginningOfLine      = 2
	uiaScrollNoScroll                    = -1.0
	uiaSupportedTextSelectionNone        = 0
	uiaSupportedTextSelectionSingle      = 1
	uiaScrollLargeDecrement              = 0
	uiaScrollSmallDecrement              = 1
	uiaScrollNoAmount                    = 2
	uiaScrollLargeIncrement              = 3
	uiaScrollSmallIncrement              = 4
	uiaNavigateParent                    = 0
	uiaNavigateNextSibling               = 1
	uiaNavigatePreviousSibling           = 2
	uiaNavigateFirstChild                = 3
	uiaNavigateLastChild                 = 4
	uiaTextPatternRangeEndpointStart     = 0
	uiaTextPatternRangeEndpointEnd       = 1
	uiaProviderOptionsServerSideProvider = 2
)

// uiaFrameworkID is the FrameworkId of every element: how UIA clients and
// screen-reader scripts tell Tela's elements apart (Chromium says
// "Chrome", WPF "WPF").
const uiaFrameworkID = "Tela"

// uiaRole is the UIA mapping of an ARIA role.
type uiaRole struct {
	controlType int32
	// localized is the LocalizedControlType, "" for UIA's default of the
	// control type.
	localized string
	// landmark is the LandmarkType (0: none) and landmarkName its
	// LocalizedLandmarkType, for the custom ones.
	landmark     int32
	landmarkName string
}

// uiaRoles maps the ARIA roles of semantics (roles.go) after Core-AAM 1.2,
// with AccessKit's and Chromium's choices where they differ.
var uiaRoles = map[string]uiaRole{
	"alert":            {uiaGroupControlTypeID, "alert", 0, ""},
	"alertdialog":      {uiaWindowControlTypeID, "alert dialog", 0, ""},
	"application":      {uiaPaneControlTypeID, "application", 0, ""},
	"article":          {uiaGroupControlTypeID, "article", 0, ""},
	"banner":           {uiaGroupControlTypeID, "banner", uiaCustomLandmarkTypeID, "banner"},
	"blockquote":       {uiaGroupControlTypeID, "blockquote", 0, ""},
	"button":           {uiaButtonControlTypeID, "", 0, ""},
	"caption":          {uiaTextControlTypeID, "", 0, ""},
	"cell":             {uiaDataItemControlTypeID, "item", 0, ""},
	"checkbox":         {uiaCheckBoxControlTypeID, "", 0, ""},
	"code":             {uiaTextControlTypeID, "code", 0, ""},
	"columnheader":     {uiaDataItemControlTypeID, "column header", 0, ""},
	"combobox":         {uiaComboBoxControlTypeID, "", 0, ""},
	"complementary":    {uiaGroupControlTypeID, "complementary", uiaCustomLandmarkTypeID, "complementary"},
	"contentinfo":      {uiaGroupControlTypeID, "content information", uiaCustomLandmarkTypeID, "content information"},
	"definition":       {uiaGroupControlTypeID, "definition", 0, ""},
	"deletion":         {uiaTextControlTypeID, "deletion", 0, ""},
	"dialog":           {uiaWindowControlTypeID, "dialog", 0, ""},
	"directory":        {uiaListControlTypeID, "", 0, ""},
	"document":         {uiaDocumentControlTypeID, "", 0, ""},
	"emphasis":         {uiaTextControlTypeID, "emphasis", 0, ""},
	"feed":             {uiaGroupControlTypeID, "feed", 0, ""},
	"figure":           {uiaGroupControlTypeID, "figure", 0, ""},
	"form":             {uiaGroupControlTypeID, "form", uiaFormLandmarkTypeID, ""},
	"generic":          {uiaGroupControlTypeID, "", 0, ""},
	"grid":             {uiaDataGridControlTypeID, "", 0, ""},
	"gridcell":         {uiaDataItemControlTypeID, "item", 0, ""},
	"group":            {uiaGroupControlTypeID, "", 0, ""},
	"heading":          {uiaTextControlTypeID, "heading", 0, ""},
	"img":              {uiaImageControlTypeID, "", 0, ""},
	"insertion":        {uiaTextControlTypeID, "insertion", 0, ""},
	"link":             {uiaHyperlinkControlTypeID, "", 0, ""},
	"list":             {uiaListControlTypeID, "", 0, ""},
	"listbox":          {uiaListControlTypeID, "", 0, ""},
	"listitem":         {uiaListItemControlTypeID, "", 0, ""},
	"log":              {uiaGroupControlTypeID, "log", 0, ""},
	"main":             {uiaGroupControlTypeID, "main", uiaMainLandmarkTypeID, ""},
	"mark":             {uiaGroupControlTypeID, "", 0, ""},
	"marquee":          {uiaGroupControlTypeID, "marquee", 0, ""},
	"math":             {uiaGroupControlTypeID, "math", 0, ""},
	"menu":             {uiaMenuControlTypeID, "", 0, ""},
	"menubar":          {uiaMenuBarControlTypeID, "", 0, ""},
	"menuitem":         {uiaMenuItemControlTypeID, "", 0, ""},
	"menuitemcheckbox": {uiaCheckBoxControlTypeID, "", 0, ""},
	"menuitemradio":    {uiaRadioButtonControlTypeID, "", 0, ""},
	"meter":            {uiaProgressBarControlTypeID, "meter", 0, ""},
	"navigation":       {uiaGroupControlTypeID, "navigation", uiaNavigationLandmarkTypeID, ""},
	"note":             {uiaGroupControlTypeID, "note", 0, ""},
	"option":           {uiaListItemControlTypeID, "", 0, ""},
	"paragraph":        {uiaGroupControlTypeID, "", 0, ""},
	"progressbar":      {uiaProgressBarControlTypeID, "", 0, ""},
	"radio":            {uiaRadioButtonControlTypeID, "", 0, ""},
	"radiogroup":       {uiaListControlTypeID, "radio group", 0, ""},
	"region":           {uiaGroupControlTypeID, "region", uiaCustomLandmarkTypeID, "region"},
	"row":              {uiaDataItemControlTypeID, "row", 0, ""},
	"rowgroup":         {uiaGroupControlTypeID, "", 0, ""},
	"rowheader":        {uiaHeaderItemControlTypeID, "row header", 0, ""},
	"scrollbar":        {uiaScrollBarControlTypeID, "", 0, ""},
	"search":           {uiaGroupControlTypeID, "search", uiaSearchLandmarkTypeID, ""},
	"searchbox":        {uiaEditControlTypeID, "search box", 0, ""},
	"separator":        {uiaSeparatorControlTypeID, "", 0, ""},
	"slider":           {uiaSliderControlTypeID, "", 0, ""},
	"spinbutton":       {uiaSpinnerControlTypeID, "", 0, ""},
	"status":           {uiaStatusBarControlTypeID, "", 0, ""},
	"strong":           {uiaTextControlTypeID, "strong", 0, ""},
	"subscript":        {uiaTextControlTypeID, "", 0, ""},
	"superscript":      {uiaTextControlTypeID, "", 0, ""},
	"switch":           {uiaButtonControlTypeID, "toggle switch", 0, ""},
	"tab":              {uiaTabItemControlTypeID, "", 0, ""},
	"table":            {uiaTableControlTypeID, "", 0, ""},
	"tablist":          {uiaTabControlTypeID, "", 0, ""},
	"tabpanel":         {uiaPaneControlTypeID, "tab panel", 0, ""},
	"term":             {uiaTextControlTypeID, "term", 0, ""},
	"textbox":          {uiaEditControlTypeID, "", 0, ""},
	"time":             {uiaTextControlTypeID, "time", 0, ""},
	"timer":            {uiaGroupControlTypeID, "timer", 0, ""},
	"toolbar":          {uiaToolBarControlTypeID, "", 0, ""},
	"tooltip":          {uiaToolTipControlTypeID, "", 0, ""},
	"tree":             {uiaTreeControlTypeID, "", 0, ""},
	"treegrid":         {uiaDataGridControlTypeID, "", 0, ""},
	"treeitem":         {uiaTreeItemControlTypeID, "", 0, ""},
}

// uiaRoleOf returns the UIA mapping of node n: the root is the window's
// content (a pane, the window itself coming from the host provider), a
// text is a Text control, an element maps by role (unknown roles are
// groups). A form or region without a name is no landmark (Core-AAM),
// and a focusable separator is a splitter's thumb.
func uiaRoleOf(n *platform.AccessNode) uiaRole {
	switch n.Kind {
	case platform.AccessRoot:
		return uiaRole{controlType: uiaPaneControlTypeID}
	case platform.AccessText:
		return uiaRole{controlType: uiaTextControlTypeID}
	}
	r, ok := uiaRoles[n.Role]
	if !ok {
		return uiaRole{controlType: uiaGroupControlTypeID}
	}
	switch n.Role {
	case "form", "region":
		if n.Name == "" {
			r.landmark, r.landmarkName = 0, ""
		}
	case "separator":
		if n.Has(platform.AccessFocusable) {
			r.controlType = uiaThumbControlTypeID
		}
	}
	return r
}

// uiaLeafRoles are the roles whose children are presentational (WAI-ARIA
// "Children Presentational: True"): their texts make their name and are
// not exposed as elements of their own, as Chromium does on Windows.
var uiaLeafRoles = map[string]bool{
	"button": true, "checkbox": true, "img": true, "math": true, "menuitemcheckbox": true,
	"menuitemradio": true, "meter": true, "progressbar": true, "radio": true, "scrollbar": true,
	"separator": true, "slider": true, "switch": true, "tab": true,
}

// uiaSelectionContainerRoles are the roles that get the Selection pattern:
// containers of selectable items.
var uiaSelectionContainerRoles = map[string]bool{
	"listbox": true, "tablist": true, "tree": true, "treegrid": true, "grid": true, "radiogroup": true,
}

// --- patterns ---

// uiaIsTextField reports whether n is an editable or selectable text
// field: what gets the Value and Text patterns over its Value.
func uiaIsTextField(n *platform.AccessNode) bool {
	return n.Kind == platform.AccessElement && (n.HasSelection || n.Role == "textbox" || n.Role == "searchbox")
}

// uiaHasText reports whether n supports the Text pattern: a text field
// (its Value) or a text node (its Text).
func uiaHasText(n *platform.AccessNode) bool {
	return n.Kind == platform.AccessText || uiaIsTextField(n)
}

// uiaText returns the text the Text pattern of n reads.
func uiaText(n *platform.AccessNode) string {
	if n.Kind == platform.AccessText {
		return n.Text
	}
	return n.Value
}

// uiaHasToggle reports whether n gets the Toggle pattern: aria-checked or
// aria-pressed is set, except on the roles whose check is a selection
// (radio, menuitemradio: SelectionItem instead, as AccessKit does).
func uiaHasToggle(n *platform.AccessNode) bool {
	if n.Kind != platform.AccessElement || uiaHasSelectionItem(n) {
		return false
	}
	return n.Checked != platform.ToggleUnset || n.Pressed != platform.ToggleUnset
}

// uiaToggleState returns the ToggleState of a node with the Toggle
// pattern.
func uiaToggleState(n *platform.AccessNode) int32 {
	t := n.Checked
	if t == platform.ToggleUnset {
		t = n.Pressed
	}
	switch t {
	case platform.ToggleTrue:
		return uiaToggleOn
	case platform.ToggleMixed:
		return uiaToggleIndeterminate
	}
	return uiaToggleOff
}

// uiaHasSelectionItem reports whether n gets the SelectionItem pattern
// (AccessKit's rule after Core-AAM): radios with aria-checked true or
// false, options, list items, tabs and tree items with aria-selected set,
// and grid cells.
func uiaHasSelectionItem(n *platform.AccessNode) bool {
	if n.Kind != platform.AccessElement {
		return false
	}
	switch n.Role {
	case "radio", "menuitemradio":
		return n.Checked == platform.ToggleTrue || n.Checked == platform.ToggleFalse
	case "option", "listitem", "tab", "treeitem", "row":
		return n.Selected != platform.ToggleUnset
	case "gridcell":
		return true
	}
	return false
}

// uiaIsSelected returns SelectionItem.IsSelected.
func uiaIsSelected(n *platform.AccessNode) bool {
	switch n.Role {
	case "radio", "menuitemradio":
		return n.Checked == platform.ToggleTrue
	}
	return n.Selected == platform.ToggleTrue
}

// uiaHasSelection reports whether n gets the Selection pattern.
func uiaHasSelection(n *platform.AccessNode) bool {
	return n.Kind == platform.AccessElement && uiaSelectionContainerRoles[n.Role]
}

// uiaHasInvoke reports whether n gets the Invoke pattern: it has a
// default action and no pattern of its own covers it (Toggle,
// SelectionItem, a text field's focus click, ExpandCollapse).
func uiaHasInvoke(n *platform.AccessNode) bool {
	return n.Kind == platform.AccessElement && n.Actions.Has(platform.AccessActionDefault) &&
		!uiaHasToggle(n) && !uiaHasSelectionItem(n) && !uiaIsTextField(n) && n.Role != "combobox"
}

// uiaHasValue reports whether n gets the Value pattern: a text field, a
// combobox, a link (its URL), or a node with a value string (a range
// widget's aria-valuetext).
func uiaHasValue(n *platform.AccessNode) bool {
	if n.Kind != platform.AccessElement {
		return false
	}
	return uiaIsTextField(n) || n.Role == "combobox" || (n.Role == "link" && n.URL != "") || n.Value != ""
}

// uiaValue returns Value.Value of n.
func uiaValue(n *platform.AccessNode) string {
	if n.Role == "link" && n.Value == "" {
		return n.URL
	}
	return n.Value
}

// uiaValueReadOnly returns Value.IsReadOnly.
func uiaValueReadOnly(n *platform.AccessNode) bool {
	return n.Has(platform.AccessReadOnly) || n.Has(platform.AccessDisabled) || !n.Actions.Has(platform.AccessActionSetValue)
}

// uiaHasRangeValue reports whether n gets the RangeValue pattern.
func uiaHasRangeValue(n *platform.AccessNode) bool {
	return n.Kind == platform.AccessElement && n.Range.HasNow
}

// uiaRangeReadOnly returns RangeValue.IsReadOnly: the node cannot step.
func uiaRangeReadOnly(n *platform.AccessNode) bool {
	return n.Has(platform.AccessDisabled) || n.Has(platform.AccessReadOnly) ||
		!(n.Actions.Has(platform.AccessActionIncrement) || n.Actions.Has(platform.AccessActionDecrement))
}

// uiaHasExpandCollapse reports whether n gets the ExpandCollapse pattern:
// aria-expanded is set, or it has a popup it can open.
func uiaHasExpandCollapse(n *platform.AccessNode) bool {
	return n.Kind == platform.AccessElement && (n.Expanded != platform.ToggleUnset ||
		(n.HasPopup != "" && (n.Actions.Has(platform.AccessActionExpand) || n.Actions.Has(platform.AccessActionDefault))))
}

// uiaExpandState returns ExpandCollapse.ExpandCollapseState.
func uiaExpandState(n *platform.AccessNode) int32 {
	switch n.Expanded {
	case platform.ToggleTrue:
		return uiaExpanded
	case platform.ToggleFalse, platform.ToggleMixed:
		return uiaCollapsed
	}
	if n.HasPopup != "" {
		return uiaCollapsed
	}
	return uiaLeafNode
}

// uiaHasScrollItem and uiaHasScroll report the ScrollItem and Scroll
// patterns.
func uiaHasScrollItem(n *platform.AccessNode) bool {
	return n.Actions.Has(platform.AccessActionScrollIntoView)
}

func uiaHasScroll(n *platform.AccessNode) bool {
	return n.Has(platform.AccessScrollable) && n.Actions.Has(platform.AccessActionScrollBy)
}

// uiaSupports reports whether n supports the control pattern with UIA ID
// pattern.
func uiaSupports(n *platform.AccessNode, pattern int32) bool {
	switch pattern {
	case uiaInvokePatternID:
		return uiaHasInvoke(n)
	case uiaTogglePatternID:
		return uiaHasToggle(n)
	case uiaValuePatternID:
		return uiaHasValue(n)
	case uiaRangeValuePatternID:
		return uiaHasRangeValue(n)
	case uiaExpandCollapsePatternID:
		return uiaHasExpandCollapse(n)
	case uiaSelectionItemPatternID:
		return uiaHasSelectionItem(n)
	case uiaSelectionPatternID:
		return uiaHasSelection(n)
	case uiaScrollItemPatternID:
		return uiaHasScrollItem(n)
	case uiaScrollPatternID:
		return uiaHasScroll(n)
	case uiaTextPatternID:
		return uiaHasText(n)
	}
	return false
}

// --- properties ---

// uiaValueKind is the type of a uiaVal.
type uiaValueKind uint8

const (
	uiaEmpty uiaValueKind = iota // VT_EMPTY: not supported, UIA's default
	uiaBool
	uiaInt
	uiaFloat
	uiaString
)

// uiaVal is a property value before it becomes a VARIANT.
type uiaVal struct {
	kind uiaValueKind
	b    bool
	i    int32
	f    float64
	s    string
}

func uiaB(b bool) uiaVal    { return uiaVal{kind: uiaBool, b: b} }
func uiaI(i int32) uiaVal   { return uiaVal{kind: uiaInt, i: i} }
func uiaF(f float64) uiaVal { return uiaVal{kind: uiaFloat, f: f} }
func uiaS(s string) uiaVal  { return uiaVal{kind: uiaString, s: s} }
func uiaSOpt(s string) uiaVal { // a string, empty when ""
	if s == "" {
		return uiaVal{}
	}
	return uiaS(s)
}

// uiaName returns the Name of n: its accessible name, a text's text with
// white space collapsed.
func uiaName(n *platform.AccessNode) string {
	if n.Kind == platform.AccessText {
		return strings.Join(strings.Fields(n.Text), " ")
	}
	return n.Name
}

// uiaProperty returns the value of property id of node n in snapshot s
// (whose root's bounds are the window's content). It covers the
// properties that need no COM objects or window: the providers add the
// rest (ControllerFor, the root's name).
func uiaProperty(s *uiaSnapshot, n *platform.AccessNode, id int32) uiaVal {
	switch id {
	case uiaControlTypePropertyID:
		return uiaI(uiaRoleOf(n).controlType)
	case uiaLocalizedControlTypePropertyID:
		return uiaSOpt(uiaRoleOf(n).localized)
	case uiaNamePropertyID:
		return uiaSOpt(uiaName(n))
	case uiaHelpTextPropertyID:
		if n.Placeholder != "" {
			return uiaS(n.Placeholder)
		}
		return uiaSOpt(n.Description)
	case uiaFullDescriptionPropertyID:
		return uiaSOpt(n.Description)
	case uiaAcceleratorKeyPropertyID:
		return uiaSOpt(uiaAcceleratorKey(n.KeyShortcuts))
	case uiaIsKeyboardFocusablePropertyID:
		return uiaB(n.Has(platform.AccessFocusable))
	case uiaHasKeyboardFocusPropertyID:
		return uiaB(s.focus == n.ID)
	case uiaIsEnabledPropertyID:
		return uiaB(!n.Has(platform.AccessDisabled))
	case uiaIsPasswordPropertyID:
		return uiaB(n.Has(platform.AccessProtected))
	case uiaIsOffscreenPropertyID:
		return uiaB(s.offscreen(n))
	case uiaIsControlElementPropertyID, uiaIsContentElementPropertyID:
		return uiaB(true)
	case uiaFrameworkIDPropertyID:
		return uiaS(uiaFrameworkID)
	case uiaAriaRolePropertyID:
		if n.Kind == platform.AccessElement {
			return uiaS(n.Role)
		}
	case uiaAriaPropertiesPropertyID:
		return uiaSOpt(uiaAriaProperties(n))
	case uiaPositionInSetPropertyID:
		if n.PosInSet > 0 {
			return uiaI(int32(n.PosInSet))
		}
	case uiaSizeOfSetPropertyID:
		if n.SetSize > 0 {
			return uiaI(int32(n.SetSize))
		}
	case uiaLevelPropertyID:
		if n.Level > 0 {
			return uiaI(int32(n.Level))
		}
	case uiaHeadingLevelPropertyID:
		if n.Role == "heading" && n.Level >= 1 && n.Level <= 9 {
			return uiaI(uiaHeadingLevel1 + int32(n.Level) - 1)
		}
	case uiaLiveSettingPropertyID:
		switch n.Live {
		case "polite":
			return uiaI(uiaLivePolite)
		case "assertive":
			return uiaI(uiaLiveAssertive)
		}
		return uiaI(uiaLiveOff)
	case uiaIsRequiredForFormPropertyID:
		return uiaB(n.Has(platform.AccessRequired))
	case uiaIsDataValidForFormPropertyID:
		return uiaB(!n.Has(platform.AccessInvalid))
	case uiaItemStatusPropertyID:
		return uiaSOpt(n.Sort)
	case uiaOrientationPropertyID:
		switch n.Orientation {
		case "horizontal":
			return uiaI(uiaOrientationHorizonal)
		case "vertical":
			return uiaI(uiaOrientationVertical)
		}
	case uiaLandmarkTypePropertyID:
		if l := uiaRoleOf(n).landmark; l != 0 {
			return uiaI(l)
		}
	case uiaLocalizedLandmarkTypePropertyID:
		return uiaSOpt(uiaRoleOf(n).landmarkName)
	case uiaIsDialogPropertyID:
		return uiaB(n.Role == "dialog" || n.Role == "alertdialog")
	// The patterns' properties, which UIA may also ask as properties.
	case uiaToggleStatePropertyID:
		if uiaHasToggle(n) {
			return uiaI(uiaToggleState(n))
		}
	case uiaExpandCollapseStatePropertyID:
		if uiaHasExpandCollapse(n) {
			return uiaI(uiaExpandState(n))
		}
	case uiaValueValuePropertyID:
		if uiaHasValue(n) {
			return uiaS(uiaValue(n))
		}
	case uiaRangeValueValuePropertyID:
		if uiaHasRangeValue(n) {
			return uiaF(n.Range.Now)
		}
	case uiaSelectionItemIsSelectedPropertyID:
		if uiaHasSelectionItem(n) {
			return uiaB(uiaIsSelected(n))
		}
	}
	return uiaVal{}
}

// uiaAcceleratorKey converts aria-keyshortcuts ("Control+S Meta+S") to
// the AcceleratorKey format of Windows ("Ctrl+S, Win+S").
func uiaAcceleratorKey(ks string) string {
	if ks == "" {
		return ""
	}
	alts := strings.Fields(ks)
	for i, a := range alts {
		keys := strings.Split(a, "+")
		for j, k := range keys {
			switch k {
			case "Control":
				keys[j] = "Ctrl"
			case "Meta":
				keys[j] = "Win"
			}
		}
		alts[i] = strings.Join(keys, "+")
	}
	return strings.Join(alts, ", ")
}

// uiaAriaProperties returns the AriaProperties string of n
// ("name=value;..."; '=', ';' and '\' escaped with '\'), with the ARIA
// properties that have no UIA property of their own (Core-AAM).
func uiaAriaProperties(n *platform.AccessNode) string {
	if n.Kind != platform.AccessElement {
		return ""
	}
	var b strings.Builder
	add := func(name, value string) {
		if b.Len() > 0 {
			b.WriteByte(';')
		}
		b.WriteString(name)
		b.WriteByte('=')
		for _, r := range value {
			if r == '=' || r == ';' || r == '\\' {
				b.WriteByte('\\')
			}
			b.WriteRune(r)
		}
	}
	if n.Has(platform.AccessAtomic) {
		add("atomic", "true")
	}
	if n.AutoComplete != "" {
		add("autocomplete", n.AutoComplete)
	}
	if n.Has(platform.AccessBusy) {
		add("busy", "true")
	}
	if n.Checked != platform.ToggleUnset {
		add("checked", n.Checked.String())
	}
	if n.Current != "" {
		add("current", n.Current)
	}
	if n.Expanded != platform.ToggleUnset {
		add("expanded", n.Expanded.String())
	}
	if n.HasPopup != "" {
		add("haspopup", n.HasPopup)
	}
	if n.Level > 0 {
		add("level", strconv.Itoa(n.Level))
	}
	if n.Has(platform.AccessModal) {
		add("modal", "true")
	}
	if n.Has(platform.AccessMultiline) {
		add("multiline", "true")
	}
	if n.Has(platform.AccessMultiSelectable) {
		add("multiselectable", "true")
	}
	if n.PosInSet > 0 {
		add("posinset", strconv.Itoa(n.PosInSet))
	}
	if n.Pressed != platform.ToggleUnset {
		add("pressed", n.Pressed.String())
	}
	if n.Has(platform.AccessReadOnly) {
		add("readonly", "true")
	}
	if n.Has(platform.AccessRequired) {
		add("required", "true")
	}
	if n.Selected != platform.ToggleUnset {
		add("selected", n.Selected.String())
	}
	if n.SetSize != 0 {
		add("setsize", strconv.Itoa(n.SetSize))
	}
	if n.Sort != "" {
		add("sort", n.Sort)
	}
	return b.String()
}

// --- the exposed tree ---

// uiaSnapshot is the tree UIA sees, made from one platform.AccessTree on
// the UI thread and then read by any thread: the nodes that are not
// ignored (AccessNode.Ignored), with the ignored ones' children lifted to
// their nearest exposed ancestor, and no children under the roles whose
// children are presentational. It is immutable. Successive snapshots share
// what did not change (a persistent map, ADR-277): updateUIASnapshot makes
// the next one from an update's changes.
type uiaSnapshot struct {
	tree *platform.AccessTree
	// ents holds an entry for every node of the tree.
	ents idmap.Map[uiaEntry]
	// focus is the exposed node that has the focus: the tree's focus, or
	// its nearest exposed ancestor; the root when no element has it.
	focus platform.AccessID
	// lost are the nodes exposed in the snapshot this one was made from
	// and not in this one; full reports a snapshot built from the whole
	// tree, which has none (lostSince compares in full).
	lost []platform.AccessID
	full bool
}

// uiaEntry is what a snapshot knows of a node.
type uiaEntry struct {
	// exposed: the node is exposed; up is then its exposed parent (0 for
	// the root), pos its index among up's kids, kids its own exposed
	// children and leaf whether its role makes its children
	// presentational (uiaLeafRoles), which hides its descendants.
	exposed bool
	up      platform.AccessID
	pos     int
	kids    []platform.AccessID
	leaf    bool
	// anc is the nearest exposed ancestor of every node but the root,
	// exposed or not.
	anc platform.AccessID
}

// uiaIsLeaf reports whether n's exposed descendants are hidden.
func uiaIsLeaf(n *platform.AccessNode) bool {
	return n.Kind == platform.AccessElement && uiaLeafRoles[n.Role]
}

// entry returns the entry of node id.
func (s *uiaSnapshot) entry(id platform.AccessID) (uiaEntry, bool) {
	if s == nil {
		return uiaEntry{}, false
	}
	return s.ents.Get(uint64(id))
}

// exposed reports whether node id is exposed.
func (s *uiaSnapshot) exposed(id platform.AccessID) bool {
	e, _ := s.entry(id)
	return e.exposed
}

// up returns the exposed parent of exposed node id (0 for the root), and
// whether it is exposed.
func (s *uiaSnapshot) up(id platform.AccessID) (platform.AccessID, bool) {
	e, _ := s.entry(id)
	return e.up, e.exposed
}

// kids returns the exposed children of exposed node id.
func (s *uiaSnapshot) kids(id platform.AccessID) []platform.AccessID {
	e, _ := s.entry(id)
	return e.kids
}

// anc returns the nearest exposed ancestor of node id (0 for the root).
func (s *uiaSnapshot) anc(id platform.AccessID) platform.AccessID {
	e, _ := s.entry(id)
	return e.anc
}

// newUIASnapshot returns the exposed tree of t (nil: the empty tree, a
// window whose first update has not come), built from the whole tree.
func newUIASnapshot(t *platform.AccessTree) *uiaSnapshot {
	b := &uiaBuild{t: t, m: &idmap.Builder[uiaEntry]{}}
	root := t.Root()
	s := &uiaSnapshot{tree: t, full: true}
	if root != 0 {
		rn, _ := t.Node(root)
		b.set(root, uiaEntry{exposed: true})
		b.region(root, rn)
	}
	s.ents = b.m.Map()
	s.focus = s.focusOf()
	return s
}

// lostSince returns the nodes exposed in old (the snapshot cur was made
// from) that cur does not expose, in ID order.
func (cur *uiaSnapshot) lostSince(old *uiaSnapshot) []platform.AccessID {
	if !cur.full {
		return cur.lost
	}
	var lost []platform.AccessID
	if old != nil {
		old.ents.Range(func(k uint64, e uiaEntry) bool {
			if e.exposed && !cur.exposed(platform.AccessID(k)) {
				lost = append(lost, platform.AccessID(k))
			}
			return true
		})
	}
	slices.Sort(lost)
	return lost
}

// focusOf returns the snapshot's focus (see uiaSnapshot.focus).
func (s *uiaSnapshot) focusOf() platform.AccessID {
	f := s.tree.Focus()
	if f == 0 || s.exposed(f) {
		return f
	}
	if a := s.anc(f); a != 0 {
		return a
	}
	return s.tree.Root()
}

// uiaBuild makes a snapshot's entries.
type uiaBuild struct {
	t *platform.AccessTree
	m *idmap.Builder[uiaEntry]
	// old is the snapshot an update starts from (nil for a full build);
	// seeds are the exposed nodes whose children it must find again, done
	// those it found again; lost collects the nodes no longer exposed.
	old   *uiaSnapshot
	seeds map[platform.AccessID]bool
	done  map[platform.AccessID]bool
	lost  map[platform.AccessID]bool
}

// set records the entry of node id.
func (b *uiaBuild) set(id platform.AccessID, e uiaEntry) {
	if b.lost != nil {
		if o, ok := b.m.Get(uint64(id)); ok && o.exposed && !e.exposed {
			b.lost[id] = true
		} else if e.exposed {
			delete(b.lost, id)
		}
	}
	b.m.Set(uint64(id), e)
}

// region finds the exposed children of exposed node k (node n), through
// its ignored descendants, or hides its descendants when it is a leaf.
func (b *uiaBuild) region(k platform.AccessID, n *platform.AccessNode) {
	if b.done != nil {
		b.done[k] = true
	}
	e, _ := b.m.Get(uint64(k))
	e.leaf = k != b.t.Root() && uiaIsLeaf(n)
	if e.leaf {
		e.kids = nil
		b.set(k, e)
		b.hide(n, k)
		return
	}
	var kids []platform.AccessID
	var walk func(n *platform.AccessNode)
	walk = func(n *platform.AccessNode) {
		for _, c := range n.Children {
			cn, ok := b.t.Node(c)
			if !ok {
				continue
			}
			if cn.Ignored() {
				b.set(c, uiaEntry{anc: k})
				walk(cn)
				continue
			}
			b.child(c, cn, k, len(kids))
			kids = append(kids, c)
		}
	}
	walk(n)
	e.kids = kids
	b.set(k, e)
}

// child makes c (node cn) the exposed child at pos of k: in an update, an
// exposed node that keeps its role class and is not a seed keeps its
// children; the others find theirs (region).
func (b *uiaBuild) child(c platform.AccessID, cn *platform.AccessNode, k platform.AccessID, pos int) {
	o, had := b.m.Get(uint64(c))
	e := uiaEntry{exposed: true, up: k, pos: pos, anc: k, leaf: uiaIsLeaf(cn)}
	if b.old != nil && had && o.exposed && o.leaf == e.leaf && !b.seeds[c] {
		e.kids = o.kids
		b.set(c, e)
		return
	}
	b.set(c, e)
	b.region(c, cn)
}

// hide records the descendants of n as hidden, below exposed node k.
func (b *uiaBuild) hide(n *platform.AccessNode, k platform.AccessID) {
	for _, c := range n.Children {
		cn, ok := b.t.Node(c)
		if !ok {
			continue
		}
		b.set(c, uiaEntry{anc: k})
		b.hide(cn, k)
	}
}

// updateUIASnapshot returns the snapshot of t made from old, the snapshot
// of the tree an update turned into t, which held the nodes changed and
// removed the nodes removed: only the children of the exposed nodes they
// reach are found again. old's tree must have t's root.
func updateUIASnapshot(old *uiaSnapshot, t *platform.AccessTree, changed, removed []platform.AccessID) *uiaSnapshot {
	b := &uiaBuild{t: t, m: idmap.NewBuilder(old.ents), old: old,
		seeds: map[platform.AccessID]bool{}, done: map[platform.AccessID]bool{}, lost: map[platform.AccessID]bool{}}
	// The seeds: a changed node that is not ignored (its children may
	// differ) and the nearest ancestor of each changed node that is not
	// ignored (the changed node may be its exposed child, or lift its
	// children into it).
	for _, id := range changed {
		n, ok := t.Node(id)
		if !ok {
			continue
		}
		if !n.Ignored() {
			b.seeds[id] = true
		}
		for p, _ := t.Parent(id); p != 0; p, _ = t.Parent(p) {
			if pn, _ := t.Node(p); !pn.Ignored() {
				b.seeds[p] = true
				break
			}
		}
	}
	for _, id := range removed {
		if o, ok := b.m.Get(uint64(id)); ok && o.exposed {
			b.lost[id] = true
		}
		b.m.Delete(uint64(id))
	}
	order := make([]platform.AccessID, 0, len(b.seeds))
	depth := make(map[platform.AccessID]int, len(b.seeds))
	for id := range b.seeds {
		order = append(order, id)
		d := 0
		for p, _ := t.Parent(id); p != 0; p, _ = t.Parent(p) {
			d++
		}
		depth[id] = d
	}
	// Ancestors first: a seed found again inside another's children is
	// done with it.
	slices.SortFunc(order, func(a, c platform.AccessID) int {
		if d := depth[a] - depth[c]; d != 0 {
			return d
		}
		return int(a) - int(c)
	})
	root := t.Root()
	for _, id := range order {
		if b.done[id] {
			continue
		}
		n, _ := t.Node(id)
		if id == root {
			b.region(root, n)
			continue
		}
		// Hidden below a leaf: the nodes below the topmost leaf ancestor
		// are hidden, their exposed ancestor is that leaf.
		top := platform.AccessID(0)
		for p, _ := t.Parent(id); p != 0; p, _ = t.Parent(p) {
			if pn, _ := t.Node(p); uiaIsLeaf(pn) {
				top = p
			}
		}
		if top != 0 {
			b.set(id, uiaEntry{anc: top})
			b.hide(n, top)
			continue
		}
		// Exposed where it was (its exposed parent is no seed, so its
		// place did not change): its children are found again.
		e, _ := b.m.Get(uint64(id))
		if !e.exposed {
			continue // only a seed's region exposes a node anew
		}
		b.region(id, n)
	}
	s := &uiaSnapshot{tree: t, ents: b.m.Map()}
	for id := range b.lost {
		s.lost = append(s.lost, id)
	}
	slices.Sort(s.lost)
	s.focus = s.focusOf()
	return s
}

// node returns the exposed node id, or false when the snapshot does not
// expose it.
func (s *uiaSnapshot) node(id platform.AccessID) (*platform.AccessNode, bool) {
	if s == nil || !s.exposed(id) {
		return nil, false
	}
	return s.tree.Node(id)
}

// root returns the ID of the root, 0 for the empty tree.
func (s *uiaSnapshot) root() platform.AccessID { return s.tree.Root() }

// navigate returns the node in direction dir (NavigateDirection) from
// exposed node id, or 0.
func (s *uiaSnapshot) navigate(id platform.AccessID, dir int32) platform.AccessID {
	switch dir {
	case uiaNavigateParent:
		p, _ := s.up(id)
		return p
	case uiaNavigateFirstChild:
		if k := s.kids(id); len(k) > 0 {
			return k[0]
		}
	case uiaNavigateLastChild:
		if k := s.kids(id); len(k) > 0 {
			return k[len(k)-1]
		}
	case uiaNavigateNextSibling, uiaNavigatePreviousSibling:
		e, _ := s.entry(id)
		if !e.exposed || e.up == 0 {
			return 0
		}
		k, i := s.kids(e.up), e.pos
		if dir == uiaNavigateNextSibling {
			i++
		} else {
			i--
		}
		if i >= 0 && i < len(k) {
			return k[i]
		}
	}
	return 0
}

// at returns the deepest exposed node whose bounds hold the point (x, y)
// in logical window coordinates, preferring later siblings (painted on
// top), or the root.
func (s *uiaSnapshot) at(x, y float32) platform.AccessID {
	id := s.root()
	if id == 0 {
		return 0
	}
	for {
		kids := s.kids(id)
		next := platform.AccessID(0)
		for i := len(kids) - 1; i >= 0; i-- {
			n, _ := s.tree.Node(kids[i])
			if b := n.Bounds; b.W > 0 && b.H > 0 && x >= b.X && x < b.X+b.W && y >= b.Y && y < b.Y+b.H {
				next = kids[i]
				break
			}
		}
		if next == 0 {
			return id
		}
		id = next
	}
}

// offscreen reports IsOffscreen: n has no area or lies outside the
// window's content (the root's bounds).
func (s *uiaSnapshot) offscreen(n *platform.AccessNode) bool {
	if n.Kind == platform.AccessRoot {
		return false
	}
	r, ok := s.tree.Node(s.root())
	if !ok {
		return true
	}
	b, w := n.Bounds, r.Bounds
	if b.W <= 0 || b.H <= 0 {
		return true
	}
	return b.X >= w.X+w.W || b.Y >= w.Y+w.H || b.X+b.W <= w.X || b.Y+b.H <= w.Y
}

// selectionContainer returns the nearest exposed ancestor of id with the
// Selection pattern, or 0.
func (s *uiaSnapshot) selectionContainer(id platform.AccessID) platform.AccessID {
	for p, _ := s.up(id); p != 0; p, _ = s.up(p) {
		if n, ok := s.tree.Node(p); ok && uiaHasSelection(n) {
			return p
		}
	}
	return 0
}

// selected returns the selected items of container id: its exposed
// descendants with the SelectionItem pattern that are selected, not
// looking inside nested selection containers.
func (s *uiaSnapshot) selected(id platform.AccessID) []platform.AccessID {
	var out []platform.AccessID
	var walk func(id platform.AccessID)
	walk = func(id platform.AccessID) {
		for _, c := range s.kids(id) {
			n, _ := s.tree.Node(c)
			if uiaHasSelectionItem(n) && uiaIsSelected(n) {
				out = append(out, c)
			}
			if !uiaHasSelection(n) {
				walk(c)
			}
		}
	}
	walk(id)
	return out
}

// --- events ---

// uiaEventKind is the kind of a uiaEvent.
type uiaEventKind uint8

const (
	uiaEventAutomation   uiaEventKind = iota // UiaRaiseAutomationEvent(node, id)
	uiaEventProperty                         // UiaRaiseAutomationPropertyChangedEvent(node, id, old, new)
	uiaEventStructure                        // UiaRaiseStructureChangedEvent(node, ChildrenInvalidated)
	uiaEventNotification                     // UiaRaiseNotificationEvent(root, Other, processing, text)
)

// uiaEvent is an event to raise after an update.
type uiaEvent struct {
	kind     uiaEventKind
	node     platform.AccessID
	id       int32
	old, new uiaVal
	text     string
}

// uiaEvents returns the events that tell UIA clients about the change
// from snapshot old (nil before the first update) to cur, made by update
// u, in the order they are raised: structure, properties, selection and
// text, then the focus, then the announcements. Like AccessKit's, the
// first update after the activation raises only the focus.
func uiaEvents(old, cur *uiaSnapshot, u *platform.AccessUpdate) []uiaEvent {
	var out []uiaEvent
	if old != nil && old.root() != 0 {
		out = uiaChanges(old, cur, u, out)
	}
	if cur.root() != 0 && (old == nil || old.root() == 0 || old.focus != cur.focus) {
		out = append(out, uiaEvent{kind: uiaEventAutomation, node: cur.focus, id: uiaAutomationFocusChangedEventID})
	}
	for _, a := range u.Announcements {
		p := int32(uiaProcessingAll)
		if a.Assertive {
			p = uiaProcessingImportantMostRecent
		}
		out = append(out, uiaEvent{kind: uiaEventNotification, node: cur.root(), id: p, text: a.Text})
	}
	return out
}

// uiaChanges appends the events of the nodes update u changed.
func uiaChanges(old, cur *uiaSnapshot, u *platform.AccessUpdate, out []uiaEvent) []uiaEvent {
	// Parents whose exposed children changed: a changed node, or the
	// exposed parent of one (an ignored node's children are its).
	invalid := map[platform.AccessID]bool{}
	check := func(id platform.AccessID) {
		if invalid[id] {
			return
		}
		if !cur.exposed(id) {
			return
		}
		if !old.exposed(id) {
			return // new: its parent's list changed
		}
		if !slices.Equal(old.kids(id), cur.kids(id)) {
			invalid[id] = true
			out = append(out, uiaEvent{kind: uiaEventStructure, node: id})
		}
	}
	for i := range u.Nodes {
		id := u.Nodes[i].ID
		check(id)
		if p, ok := cur.up(id); ok && p != 0 {
			check(p)
		} else if p := cur.anc(id); !ok && p != 0 {
			check(p) // an ignored node: its children are its exposed ancestor's
		}
	}
	for i := range u.Nodes {
		id := u.Nodes[i].ID
		n, ok := cur.node(id)
		if !ok {
			continue
		}
		o, was := old.node(id)
		if !was {
			// Newly exposed.
			switch {
			case n.Role == "menu":
				out = append(out, uiaEvent{kind: uiaEventAutomation, node: id, id: uiaMenuOpenedEventID})
			case n.Role == "dialog" || n.Role == "alertdialog":
				out = append(out, uiaEvent{kind: uiaEventAutomation, node: id, id: uiaWindowOpenedEventID})
			}
			if uiaHasSelectionItem(n) && uiaIsSelected(n) {
				out = uiaSelectionEvents(cur, id, true, out)
			}
			continue
		}
		out = uiaNodeChanges(cur, o, n, out)
	}
	// Menus that went away.
	for _, id := range cur.lostSince(old) {
		if n, _ := old.tree.Node(id); n != nil && n.Role == "menu" {
			out = append(out, uiaEvent{kind: uiaEventAutomation, node: id, id: uiaMenuClosedEventID})
		}
	}
	return out
}

// uiaNodeChanges appends the property and pattern events of a node that
// stayed exposed, from o to n.
func uiaNodeChanges(cur *uiaSnapshot, o, n *platform.AccessNode, out []uiaEvent) []uiaEvent {
	id := n.ID
	prop := func(pid int32, ov, nv uiaVal) {
		if ov != nv {
			out = append(out, uiaEvent{kind: uiaEventProperty, node: id, id: pid, old: ov, new: nv})
		}
	}
	prop(uiaNamePropertyID, uiaS(uiaName(o)), uiaS(uiaName(n)))
	prop(uiaFullDescriptionPropertyID, uiaS(o.Description), uiaS(n.Description))
	prop(uiaHelpTextPropertyID, uiaProperty(cur, o, uiaHelpTextPropertyID), uiaProperty(cur, n, uiaHelpTextPropertyID))
	prop(uiaIsEnabledPropertyID, uiaB(!o.Has(platform.AccessDisabled)), uiaB(!n.Has(platform.AccessDisabled)))
	prop(uiaIsKeyboardFocusablePropertyID, uiaB(o.Has(platform.AccessFocusable)), uiaB(n.Has(platform.AccessFocusable)))
	prop(uiaIsDataValidForFormPropertyID, uiaB(!o.Has(platform.AccessInvalid)), uiaB(!n.Has(platform.AccessInvalid)))
	prop(uiaIsRequiredForFormPropertyID, uiaB(o.Has(platform.AccessRequired)), uiaB(n.Has(platform.AccessRequired)))
	if uiaHasToggle(o) && uiaHasToggle(n) {
		prop(uiaToggleStatePropertyID, uiaI(uiaToggleState(o)), uiaI(uiaToggleState(n)))
	}
	if uiaHasExpandCollapse(o) && uiaHasExpandCollapse(n) {
		prop(uiaExpandCollapseStatePropertyID, uiaI(uiaExpandState(o)), uiaI(uiaExpandState(n)))
	}
	if uiaHasValue(o) && uiaHasValue(n) {
		prop(uiaValueValuePropertyID, uiaS(uiaValue(o)), uiaS(uiaValue(n)))
	}
	if uiaHasRangeValue(o) && uiaHasRangeValue(n) {
		prop(uiaRangeValueValuePropertyID, uiaF(o.Range.Now), uiaF(n.Range.Now))
	}
	if uiaHasSelectionItem(n) {
		was := uiaHasSelectionItem(o) && uiaIsSelected(o)
		if is := uiaIsSelected(n); is != was {
			out = uiaSelectionEvents(cur, id, is, out)
		}
	}
	if uiaHasText(o) && uiaHasText(n) {
		if uiaText(o) != uiaText(n) {
			out = append(out, uiaEvent{kind: uiaEventAutomation, node: id, id: uiaTextChangedEventID})
		}
		if o.Selection != n.Selection || o.HasSelection != n.HasSelection {
			out = append(out, uiaEvent{kind: uiaEventAutomation, node: id, id: uiaTextSelectionChangedEventID})
		}
	}
	// A live region whose own name changed is read again (AccessKit's
	// rule); the content of the others comes as core's announcements.
	if n.Live != "" && n.Name != "" && (o.Name != n.Name || o.Live != n.Live) {
		out = append(out, uiaEvent{kind: uiaEventAutomation, node: id, id: uiaLiveRegionChangedEventID})
	}
	return out
}

// uiaSelectionEvents appends the events of item id becoming selected or
// not: ElementSelected (or AddedToSelection in a multi-selectable
// container) or RemovedFromSelection, and the IsSelected change.
func uiaSelectionEvents(cur *uiaSnapshot, id platform.AccessID, selected bool, out []uiaEvent) []uiaEvent {
	multi := false
	if c := cur.selectionContainer(id); c != 0 {
		cn, _ := cur.tree.Node(c)
		multi = cn.Has(platform.AccessMultiSelectable)
	}
	switch {
	case selected && multi:
		out = append(out, uiaEvent{kind: uiaEventAutomation, node: id, id: uiaElementAddedToSelectionEventID})
	case selected:
		out = append(out, uiaEvent{kind: uiaEventAutomation, node: id, id: uiaElementSelectedEventID})
	case multi:
		out = append(out, uiaEvent{kind: uiaEventAutomation, node: id, id: uiaElementRemovedFromSelectionEventID})
	}
	return append(out, uiaEvent{kind: uiaEventProperty, node: id, id: uiaSelectionItemIsSelectedPropertyID,
		old: uiaB(!selected), new: uiaB(selected)})
}
