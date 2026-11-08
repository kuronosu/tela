package telatest

import (
	"fmt"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/kuronosu/tela"
	"github.com/kuronosu/tela/aria"
	"github.com/kuronosu/tela/h"
	"github.com/kuronosu/tela/internal/semantics"
	"github.com/kuronosu/tela/kit"
)

// The accessibility audit of kit (ADR-246): every component is mounted in
// representative states, its overlays open, and the semantics tree the OS
// accessibility bridges receive is checked with rules after axe-core's
// (aria-allowed-role, aria-command-name, aria-input-field-name,
// aria-toggle-field-name, aria-required-attr, aria-valid-attr-value,
// aria-dialog-name, aria-progressbar-name, aria-tooltip-name,
// focus-order-semantics), plus the fields of the bridges: set items have a
// position in their set and toasts are in a live region. The test is
// internal to telatest because the raw tree (semantics.Node, with the
// fields the aria snapshot does not print) has no public accessor.

// namedRoles are the roles that need a name (axe's command, input-field,
// toggle-field, dialog, progressbar, tooltip and image rules, and the
// widgets a focus lands on).
var namedRoles = []string{
	"button", "link", "checkbox", "switch", "radio", "menuitem", "menuitemcheckbox", "menuitemradio",
	"option", "tab", "treeitem", "slider", "spinbutton", "textbox", "searchbox", "combobox", "listbox",
	"dialog", "alertdialog", "progressbar", "meter", "tooltip", "img", "radiogroup", "tree", "grid",
}

// checkedRoles need AriaChecked (ARIA's required states).
var checkedRoles = []string{"checkbox", "switch", "radio", "menuitemcheckbox", "menuitemradio"}

// setItemRoles are the items of sets (semantics.FillSetPositions).
var setItemRoles = []string{"option", "menuitem", "menuitemcheckbox", "menuitemradio", "tab", "radio", "treeitem", "listitem"}

// audit returns the violations of the rules in the tree rooted at root.
func audit(root *semantics.Node) []string {
	ids := map[semantics.NodeID]*semantics.Node{}
	root.Walk(func(n *semantics.Node) bool {
		ids[n.ID] = n
		return true
	})
	var out []string
	bad := func(n *semantics.Node, format string, args ...any) {
		out = append(out, fmt.Sprintf("%s %q: %s", n.Role, n.Name, fmt.Sprintf(format, args...)))
	}
	root.Walk(func(n *semantics.Node) bool {
		if n.Has(semantics.StateHidden) {
			return false
		}
		if n.Kind != semantics.KindElement {
			return true
		}
		named := strings.TrimSpace(n.Name) != ""
		if n.Has(semantics.StateFocusable) {
			switch n.Role {
			case "generic", "none", "presentation":
				bad(n, "focusable without a role")
			}
			if !named && n.Role != "generic" && n.Role != "tabpanel" && n.Role != "group" {
				bad(n, "focusable without a name")
			}
		}
		if slices.Contains(namedRoles, n.Role) && !named {
			bad(n, "a %s needs a name", n.Role)
		}
		if slices.Contains(checkedRoles, n.Role) && n.Checked == semantics.ToggleUnset {
			bad(n, "no AriaChecked")
		}
		if n.Role == "combobox" && n.Expanded == semantics.ToggleUnset {
			bad(n, "a combobox needs AriaExpanded")
		}
		if (n.Role == "slider" || n.Role == "scrollbar") && !n.Range.HasNow {
			bad(n, "no AriaValueNow")
		}
		if n.Role == "tab" && n.Selected == semantics.ToggleUnset {
			bad(n, "a tab needs AriaSelected")
		}
		for _, ref := range []struct {
			name string
			id   semantics.NodeID
		}{{"AriaControls", n.Controls}, {"AriaActiveDescendant", n.ActiveDescendant}} {
			if ref.id == 0 {
				continue
			}
			if t := ids[ref.id]; t == nil {
				bad(n, "%s names a node that is not in the tree", ref.name)
			} else if t.Has(semantics.StateHidden) && n.Expanded == semantics.ToggleTrue {
				bad(n, "%s names a hidden node while expanded", ref.name)
			}
		}
		if n.Expanded == semantics.ToggleTrue && n.HasPopup != "" && n.Controls == 0 {
			bad(n, "an open popup trigger without AriaControls")
		}
		if slices.Contains(setItemRoles, n.Role) && (n.PosInSet == 0 || n.SetSize == 0) {
			bad(n, "a set item outside a set (PosInSet %d, SetSize %d)", n.PosInSet, n.SetSize)
		}
		if n.Role == "heading" && n.Level == 0 {
			bad(n, "a heading without a level")
		}
		return true
	})
	return out
}

// auditCase is one screen of the audit: a component, and what to do to
// reach the state to check.
type auditCase struct {
	name string
	root func() tela.Node
	// act brings the screen to its state (open an overlay...).
	act func(a *App)
	// check is a further check on the tree.
	check func(t *testing.T, root *semantics.Node)
}

func auditKit(t *testing.T, cases []auditCase) {
	t.Helper()
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			a := Mount(t, tela.C(c.root), Size(1000, 760))
			if c.act != nil {
				c.act(a)
			}
			root := a.tree.Semantics()
			for _, v := range audit(root) {
				t.Error(v)
			}
			if c.check != nil {
				c.check(t, root)
			}
			if t.Failed() || testing.Verbose() {
				t.Logf("aria snapshot:\n%s", a.AriaSnapshot())
			}
		})
	}
}

// nodeOf returns the first node with role and name, failing without one.
func nodeOf(t *testing.T, root *semantics.Node, role, name string) *semantics.Node {
	t.Helper()
	var found *semantics.Node
	root.Walk(func(n *semantics.Node) bool {
		if found == nil && n.Kind == semantics.KindElement && n.Role == role && n.Name == name && !n.Has(semantics.StateHidden) {
			found = n
		}
		return found == nil
	})
	if found == nil {
		t.Fatalf("no %s %q", role, name)
	}
	return found
}

var auditItems = []kit.MenuItem[string]{
	{Kind: kit.MenuLabel, Label: "My account"},
	{Kind: kit.MenuSeparator},
	{Key: "profile", Label: "Profile", Shortcut: "Shift+Cmd+P"},
	{Key: "keys", Label: "Keyboard shortcuts", Disabled: true},
	{Kind: kit.MenuCheckbox, Key: "bar", Label: "Status bar", DefaultChecked: true},
	{Kind: kit.MenuRadioGroup, Key: "pos", Label: "Position", DefaultValue: new("top"), Items: []kit.MenuItem[string]{
		{Key: "top", Label: "Top"}, {Key: "bottom", Label: "Bottom"}}},
	{Kind: kit.MenuSeparator},
	{Key: "logout", Label: "Log out", Variant: kit.Destructive},
}

func btn(label string) kit.Button { return kit.Button{Variant: kit.Outline, Children: h.Text(label)} }

func TestKitAccessibilityAudit(t *testing.T) {
	fruits := []kit.SelectItem[string]{{Value: "apple", Label: "Apple"}, {Value: "banana", Label: "Banana"}, {Value: "cherry", Label: "Cherry", Disabled: true}}
	auditKit(t, []auditCase{
		{name: "controls", root: func() tela.Node {
			return h.Div(h.Class("flex flex-col gap-4 p-4"),
				kit.Button{Children: h.Text("Save")},
				kit.Button{Size: kit.Icon, Icon: tela.Icon{SVG: `<svg viewBox="0 0 24 24"><path d="M6 9l6 6 6-6"/></svg>`}, AriaLabel: "More"},
				kit.Field{Children: h.Fragment(kit.Checkbox{DefaultChecked: true, ID: "terms"}, kit.Label{For: "terms", Children: h.Text("Accept terms")})},
				kit.Field{Children: h.Fragment(kit.Switch{ID: "air"}, kit.Label{For: "air", Children: h.Text("Airplane mode")})},
				kit.Checkbox{AriaLabel: "Bare"},
				kit.Toggle{AriaLabel: "Bold", Children: h.Text("B")},
				kit.ToggleGroup[string]{AriaLabel: "Align", Items: []kit.ToggleGroupItem[string]{{Value: "l", Label: "Left"}, {Value: "r", Label: "Right"}}},
				kit.ToggleGroup[string]{AriaLabel: "Style", Multiple: true, Items: []kit.ToggleGroupItem[string]{{Value: "b", Label: "Bold"}, {Value: "i", Label: "Italic"}}},
				kit.RadioGroup[string]{AriaLabel: "Plan", DefaultValue: new("pro"), Items: []kit.RadioItem[string]{{Value: "free", Label: "Free"}, {Value: "pro", Label: "Pro", Description: "For teams"}}},
				kit.Slider{AriaLabel: "Volume", DefaultValue: new(30.0), Max: 100},
				kit.Slider{AriaLabel: "Price", DefaultValues: []float64{20, 80}, Max: 100, ThumbLabels: []string{"Minimum", "Maximum"}},
				kit.Progress{Value: 40, AriaLabel: "Upload"},
				kit.Field{Children: h.Fragment(kit.Label{For: "email", Children: h.Text("Email")}, kit.Input{ID: "email", Type: "email", Placeholder: "you@example.com"})},
				kit.Input{Type: "password", AriaLabel: "Password"},
				kit.Textarea{AriaLabel: "Bio"},
				kit.Separator{Semantic: true},
				kit.Badge{Children: h.Text("New")},
				kit.Avatar{Alt: "Ada Lovelace", Fallback: "AL"},
				kit.Skeleton{Props: h.Class("h-4 w-20")},
				kit.Card{Title: h.Text("Card"), Description: h.Text("Text"), Children: h.Text("Body")},
				kit.FieldSet{Children: h.Text("Fields")},
			)
		}, check: func(t *testing.T, root *semantics.Node) {
			if n := nodeOf(t, root, "textbox", "Password"); !n.Has(semantics.StateProtected) {
				t.Error("the password input is not protected")
			}
			if n := nodeOf(t, root, "radio", "Pro"); n.PosInSet != 2 || n.SetSize != 2 {
				t.Errorf("radio Pro at %d of %d", n.PosInSet, n.SetSize)
			}
		}},
		{name: "tabs", root: func() tela.Node {
			return kit.Tabs[string]{AriaLabel: "Settings", Tabs: []kit.Tab[string]{
				{Value: "account", Label: "Account", Content: h.Text("Account body")},
				{Value: "password", Label: "Password", Content: h.Text("Password body")},
				{Value: "off", Label: "Off", Disabled: true, Content: h.Text("-")},
			}}
		}},
		{name: "select open", root: func() tela.Node {
			return kit.Select[string]{AriaLabel: "Fruit", Items: fruits, Placeholder: "Pick a fruit"}
		}, act: func(a *App) { a.Click(ByRole("button", "Pick a fruit Fruit")) }, check: func(t *testing.T, root *semantics.Node) {
			cb := nodeOf(t, root, "button", "Pick a fruit Fruit")
			if cb.Expanded != semantics.ToggleTrue || cb.Controls == 0 || cb.HasPopup != "listbox" {
				t.Errorf("open select trigger %+v", cb)
			}
		}},
		{name: "select closed with value", root: func() tela.Node {
			return kit.Select[string]{AriaLabel: "Fruit", Items: fruits, DefaultValue: new("banana")}
		}},
		{name: "dropdown menu open", root: func() tela.Node {
			return kit.DropdownMenu[string]{Trigger: btn("Open"), Items: auditItems}
		}, act: func(a *App) { a.Click(ByRole("button", "Open")) }, check: func(t *testing.T, root *semantics.Node) {
			if n := nodeOf(t, root, "menuitem", "Profile"); n.KeyShortcuts != "Shift+Meta+P" {
				t.Errorf("menu item Profile: KeyShortcuts %q for its Shortcut Shift+Cmd+P", n.KeyShortcuts)
			}
			if n := nodeOf(t, root, "menuitemradio", "Bottom"); n.PosInSet != 2 || n.SetSize != 2 {
				t.Errorf("menuitemradio Bottom at %d of %d", n.PosInSet, n.SetSize)
			}
			nodeOf(t, root, "group", "Position")
		}},
		{name: "context menu open", root: func() tela.Node {
			// The area is focusable for Shift+F10, so it needs a role and a
			// name, as the gallery's.
			return kit.ContextMenu[string]{AriaLabel: "Page", Class: "h-40 w-72", Children: h.Text("Right click here"), Items: auditItems,
				Props: tela.MergeProps(h.TabIndex(0), h.Role("region"), h.AriaLabel("Page area"), h.AriaKeyShortcuts("Shift+F10 ContextMenu"))}
		}, act: func(a *App) { a.Click(ByText("Right click here"), Right()) }, check: func(t *testing.T, root *semantics.Node) {
			if n := nodeOf(t, root, "region", "Page area"); !n.Can(semantics.ActionShowMenu) {
				t.Error("the area cannot show its menu")
			}
		}},
		{name: "menubar open", root: func() tela.Node {
			return kit.Menubar[string]{AriaLabel: "Application", Menus: []kit.MenubarMenu[string]{
				{Key: "file", Label: "File", Items: auditItems},
				{Key: "edit", Label: "Edit", Items: []kit.MenuItem[string]{{Key: "undo", Label: "Undo"}}},
			}}
		}, act: func(a *App) { a.Click(ByRole("menuitem", "File")) }},
		{name: "dialog open", root: func() tela.Node {
			return kit.Dialog{Trigger: btn("Edit profile"), Title: h.Text("Edit profile"), Description: h.Text("Make changes."),
				Children: kit.Input{AriaLabel: "Name"}, Footer: kit.Button{Children: h.Text("Save changes")}}
		}, act: func(a *App) { a.Click(ByRole("button", "Edit profile")) }, check: func(t *testing.T, root *semantics.Node) {
			if d := nodeOf(t, root, "dialog", "Edit profile"); !d.Modal || d.Description == "" {
				t.Errorf("dialog %+v", d)
			}
		}},
		{name: "alert dialog open", root: func() tela.Node {
			return kit.AlertDialog{Trigger: btn("Delete"), Title: h.Text("Are you sure?"), Description: h.Text("This cannot be undone.")}
		}, act: func(a *App) { a.Click(ByRole("button", "Delete")) }},
		{name: "popover open", root: func() tela.Node {
			return kit.Popover{Trigger: btn("Dimensions"), Children: kit.Input{AriaLabel: "Width"}}
		}, act: func(a *App) { a.Click(ByRole("button", "Dimensions")) }},
		{name: "tooltip open", root: func() tela.Node {
			return kit.Tooltip{Trigger: btn("Hover"), Content: h.Text("Add to library"), DefaultOpen: true}
		}, act: func(a *App) { a.Advance(time.Second) }, check: func(t *testing.T, root *semantics.Node) {
			if n := nodeOf(t, root, "button", "Hover"); n.Description != "Add to library" {
				t.Errorf("the trigger's description is %q", n.Description)
			}
		}},
		{name: "combobox open", root: func() tela.Node {
			fruits := []string{"apple", "apricot", "banana"}
			cb := aria.UseCombobox(aria.ComboboxOptions[string]{Items: fruits, TextValue: strings.ToUpper, AriaLabel: "Fruit"})
			return h.Div(
				h.Input(cb.InputProps), h.Button(cb.ButtonProps, h.Text("v")),
				h.Portal(h.If(cb.Open, h.Div(cb.PopoverProps, h.Div(cb.ListBox.Props,
					h.Range(len(cb.Keys), func(i int) tela.Node {
						k := cb.Keys[i]
						return h.Div(cb.ListBox.ItemProps(k), h.Key(k), h.Text(strings.ToUpper(k)))
					}))))))
		}, act: func(a *App) {
			a.Focus(ByRole("combobox", "Fruit"))
			a.Type("ap")
			a.Press("ArrowDown")
		}, check: func(t *testing.T, root *semantics.Node) {
			cb := nodeOf(t, root, "combobox", "Fruit")
			opt := nodeOf(t, root, "option", "APPLE")
			if cb.Expanded != semantics.ToggleTrue || cb.ActiveDescendant != opt.ID || cb.Controls == 0 ||
				cb.Selection == nil || *cb.Selection != (semantics.TextSelection{Anchor: 2, Focus: 2}) {
				t.Errorf("combobox %+v (selection %v)", cb, cb.Selection)
			}
			if opt.PosInSet != 1 || opt.SetSize != 2 {
				t.Errorf("option APPLE at %d of %d", opt.PosInSet, opt.SetSize)
			}
		}},
		{name: "toasts", root: func() tela.Node {
			return kit.Toaster{Children: tela.C(func() tela.Node {
				toast := kit.UseToast()
				return h.Div(
					kit.Button{Children: h.Text("Notify"), OnClick: func() {
						toast.Success("Event created", kit.ToastDescription("Sunday at 9:00"))
						toast.Error("Upload failed")
					}})
			})}
		}, act: func(a *App) { a.Click(ByRole("button", "Notify")) }, check: func(t *testing.T, root *semantics.Node) {
			region := nodeOf(t, root, "region", "Notifications")
			if region.Live == "" {
				t.Errorf("the toaster's region is not live (Live %q): a toast added to it would not be announced", region.Live)
			}
			for _, role := range []string{"status", "alert"} {
				var n *semantics.Node
				root.Walk(func(x *semantics.Node) bool {
					if n == nil && x.Role == role {
						n = x
					}
					return n == nil
				})
				if n == nil {
					t.Errorf("no %s toast", role)
				} else if n.LiveRoot == 0 {
					t.Errorf("%s toast %q is not in a live region", role, n.Name)
				}
			}
		}},
		{name: "table", root: func() tela.Node {
			type row struct{ name, role string }
			return kit.Table[row]{Rows: []row{{"Ada", "Admin"}, {"Linus", "Dev"}}, Columns: []kit.Column[row]{
				{ID: "name", Header: "Name", Sortable: true, Cell: func(r row) tela.Node { return h.Text(r.name) }, Sort: func(a, b row) int { return strings.Compare(a.name, b.name) }},
				{ID: "role", Header: "Role", Cell: func(r row) tela.Node { return h.Text(r.role) }},
			}}
		}},
		{name: "calendar", root: func() tela.Node {
			return kit.Calendar{DefaultValue: aria.Date{Year: 2026, Month: 11, Day: 8}, Today: aria.Date{Year: 2026, Month: 11, Day: 8}}
		}, check: func(t *testing.T, root *semantics.Node) {
			var selected []string
			root.Walk(func(n *semantics.Node) bool {
				if n.Role == "gridcell" && n.Selected == semantics.ToggleTrue {
					selected = append(selected, n.Name)
				}
				return true
			})
			if len(selected) != 1 || !strings.Contains(selected[0], "November 8, 2026") {
				t.Errorf("selected cells %q", selected)
			}
		}},
		{name: "range calendar", root: func() tela.Node {
			return kit.RangeCalendar{Today: aria.Date{Year: 2026, Month: 11, Day: 8}}
		}},
		{name: "date picker open", root: func() tela.Node {
			return kit.DatePicker{AriaLabel: "Birthday", Today: aria.Date{Year: 2026, Month: 11, Day: 8}}
		}, act: func(a *App) { a.Click(ByRole("button", "Birthday")) }},
		{name: "color picker", root: func() tela.Node {
			return kit.ColorPicker{AriaLabel: "Accent", Alpha: true, Swatches: []tela.Color{tela.RGB(255, 0, 0), tela.RGB(0, 0, 255)}}
		}},
		{name: "color input open", root: func() tela.Node {
			return kit.ColorInput{AriaLabel: "Background"}
		}, act: func(a *App) { a.Click(ByRole("button", "Background, #000000")) }},
		{name: "title bar", root: func() tela.Node {
			return kit.TitleBar{Title: h.Text("Notes")}
		}},
	})
}

// TestKitAccessibilityActions: the actions of the bridges operate kit's
// components as a user would: increment a slider through its arrow keys,
// expand a select, open a context menu from its area.
func TestKitAccessibilityActions(t *testing.T) {
	a := Mount(t, tela.C(func() tela.Node {
		return h.Div(h.Class("flex flex-col gap-4 p-4"),
			kit.Slider{AriaLabel: "Volume", DefaultValue: new(30.0), Max: 100},
			kit.Slider{AriaLabel: "Balance", DefaultValue: new(50.0), Max: 100, Direction: "rtl"},
			kit.Select[string]{AriaLabel: "Fruit", Items: []kit.SelectItem[string]{{Value: "apple", Label: "Apple"}, {Value: "kiwi", Label: "Kiwi"}}, DefaultValue: new("apple")},
			kit.ContextMenu[string]{AriaLabel: "Page", Class: "h-20 w-40", Children: h.Text("Area"),
				Props: tela.MergeProps(h.TabIndex(0), h.Role("region"), h.AriaLabel("Page area")),
				Items: []kit.MenuItem[string]{{Key: "back", Label: "Back"}, {Key: "reload", Label: "Reload"}}},
		)
	}), Size(600, 500))
	node := func(role, name string) *semantics.Node { return nodeOf(t, a.tree.Semantics(), role, name) }
	do := func(err error) {
		t.Helper()
		if err != nil {
			t.Fatal(err)
		}
		a.settle()
	}
	do(a.tree.IncrementNode(node("slider", "Volume").ID))
	if v := node("slider", "Volume").Value; v != "31" {
		t.Errorf("Volume after an increment: %q, want 31", v)
	}
	do(a.tree.IncrementNode(node("slider", "Balance").ID))
	if v := node("slider", "Balance").Value; v != "51" {
		t.Errorf("a right-to-left Balance after an increment: %q, want 51", v)
	}
	do(a.tree.DecrementNode(node("slider", "Balance").ID))
	do(a.tree.DecrementNode(node("slider", "Balance").ID))
	if v := node("slider", "Balance").Value; v != "49" {
		t.Errorf("Balance after two decrements: %q, want 49", v)
	}
	sel := node("button", "Apple Fruit")
	if !sel.Can(semantics.ActionExpand) {
		t.Fatalf("the select trigger cannot expand: %+v", sel)
	}
	do(a.tree.ExpandNode(sel.ID))
	nodeOf(t, a.tree.Semantics(), "listbox", "Fruit")
	do(a.tree.CollapseNode(node("button", "Apple Fruit").ID))
	a.Advance(time.Second)
	do(a.tree.ShowMenuNode(node("region", "Page area").ID))
	back := node("menuitem", "Back")
	if !back.Has(semantics.StateFocused) {
		t.Errorf("the context menu opened from its area does not focus its first item:\n%s", a.AriaSnapshot())
	}
}
