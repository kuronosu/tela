package telatest

import (
	"fmt"
	"math"
	"slices"
	"strings"

	"github.com/kuronosu/tela/internal/gfx"
	"github.com/kuronosu/tela/internal/semantics"
)

// The rules that read the semantics tree (ADR-306): the accessible-name
// rules of axe-core by the kind of widget, the required states, the
// references between elements, and the fields the OS bridges need.

// roleGroup maps the roles that need an accessible name to the rule that
// says so; the groups are axe's (aria-command-name: button, link,
// menuitem; aria-input-field-name; aria-toggle-field-name), with the
// roles Tela's widgets use (tabs and options are commands, a tablist
// is a group).
var roleGroup = map[string]string{
	"button": "aria-command-name", "link": "aria-command-name", "menuitem": "aria-command-name",
	"tab": "aria-command-name", "option": "aria-command-name",
	"textbox": "aria-input-field-name", "searchbox": "aria-input-field-name", "combobox": "aria-input-field-name",
	"listbox": "aria-input-field-name", "slider": "aria-input-field-name", "spinbutton": "aria-input-field-name",
	"tree": "aria-input-field-name", "grid": "aria-input-field-name", "radiogroup": "aria-input-field-name",
	"checkbox": "aria-toggle-field-name", "switch": "aria-toggle-field-name", "radio": "aria-toggle-field-name",
	"menuitemcheckbox": "aria-toggle-field-name", "menuitemradio": "aria-toggle-field-name",
	"dialog": "aria-dialog-name", "alertdialog": "aria-dialog-name",
	"progressbar": "aria-progressbar-name", "meter": "aria-meter-name", "tooltip": "aria-tooltip-name",
	"treeitem": "aria-treeitem-name", "img": "role-img-alt",
}

// setItemRoles are the items of sets (semantics.FillSetPositions).
var setItemRoles = []string{"option", "menuitem", "menuitemcheckbox", "menuitemradio", "tab", "radio", "treeitem", "listitem"}

// checkedRoles need AriaChecked.
var checkedRoles = []string{"checkbox", "switch", "radio", "menuitemcheckbox", "menuitemradio"}

// nameRule is the rule for an accessible name of one group.
func nameRule(id, desc, impact string, tags ...string) *ruleDef {
	r := &ruleDef{Rule: Rule{ID: id, Description: desc, Impact: impact, Default: true,
		Tags: append([]string{"aria", "wcag2a", "wcag412"}, tags...)}}
	r.node = func(c *auditCtx, n *semantics.Node) {
		if roleGroup[n.Role] != id || strings.TrimSpace(n.Name) != "" {
			return
		}
		article := "a"
		if strings.ContainsRune("aeiou", rune(n.Role[0])) {
			article = "an"
		}
		fix := `give it text content, h.AriaLabel("...") or h.AriaLabelledBy(...)`
		switch n.Role {
		case "img":
			fix = `describe it with h.Alt("...") or h.AriaLabel("..."), or hide it from assistive technologies with h.AriaHidden(true) when it is decoration`
		case "textbox", "searchbox", "combobox", "spinbutton":
			fix = `label it with a kit.Label For its ID, h.AriaLabel("...") or h.AriaLabelledBy(...); a Placeholder is not a name`
		case "dialog", "alertdialog":
			fix = `give it a title: h.AriaLabelledBy(title) of its heading, or h.AriaLabel("...")`
		}
		c.report(n, fmt.Sprintf("%s %s needs an accessible name", article, n.Role), fix)
	}
	return r
}

func init() {
	register(
		nameRule("aria-command-name", "buttons, links, menu items, tabs and options have an accessible name", "critical"),
		nameRule("aria-input-field-name", "text fields, comboboxes, listboxes, sliders and trees have an accessible name", "critical"),
		nameRule("aria-toggle-field-name", "checkboxes, switches and radios have an accessible name", "critical"),
		nameRule("aria-dialog-name", "dialogs have an accessible name", "serious"),
		nameRule("aria-progressbar-name", "progress bars have an accessible name", "serious"),
		nameRule("aria-meter-name", "meters have an accessible name", "serious"),
		nameRule("aria-tooltip-name", "tooltips have an accessible name", "serious"),
		nameRule("aria-treeitem-name", "tree items have an accessible name", "serious"),
		nameRule("role-img-alt", "images have a text alternative", "critical", "wcag111"),

		&ruleDef{Rule: Rule{ID: "focusable-name", Description: "a focusable element that no other rule covers has an accessible name", Impact: "serious",
			Tags: []string{"aria", "wcag2a", "wcag412", "tela"}, Default: true},
			node: func(c *auditCtx, n *semantics.Node) {
				if !n.Has(semantics.StateFocusable) || roleGroup[n.Role] != "" || strings.TrimSpace(n.Name) != "" {
					return
				}
				switch n.Role {
				case "generic", "none", "presentation", "tabpanel", "group":
					return // focus-order-semantics, or a container that takes the focus to scroll
				}
				c.report(n, fmt.Sprintf("a focusable %s has no accessible name", n.Role), `give it h.AriaLabel("...") or h.AriaLabelledBy(...)`)
			}},

		&ruleDef{Rule: Rule{ID: "focus-order-semantics", Description: "a focusable element has a role, so that assistive technologies announce what it is", Impact: "minor",
			Tags: []string{"aria", "best-practice", "wcag412"}, Default: true},
			node: func(c *auditCtx, n *semantics.Node) {
				if !n.Has(semantics.StateFocusable) {
					return
				}
				switch n.Role {
				case "generic", "none", "presentation":
					c.report(n, "a focusable element without a role", `give it h.Role("...") for what it is (button, region, listbox...), or remove its h.TabIndex`)
				}
			}},

		&ruleDef{Rule: Rule{ID: "aria-required-attr", Description: "an element has the states and values its role requires: checked, expanded, value, selected, a heading's level", Impact: "critical",
			Tags: []string{"aria", "wcag2a", "wcag412"}, Default: true},
			node: func(c *auditCtx, n *semantics.Node) {
				switch {
				case slices.Contains(checkedRoles, n.Role) && n.Checked == semantics.ToggleUnset:
					c.report(n, fmt.Sprintf("a %s without a checked state", n.Role), "set h.AriaChecked(...) from the value it shows")
				case n.Role == "combobox" && n.Expanded == semantics.ToggleUnset:
					c.report(n, "a combobox without an expanded state", "set h.AriaExpanded(...) to whether its list is open")
				case (n.Role == "slider" || n.Role == "scrollbar") && !n.Range.HasNow:
					c.report(n, fmt.Sprintf("a %s without a value", n.Role), "set h.AriaValueNow(...), with AriaValueMin and AriaValueMax")
				case n.Role == "tab" && n.Selected == semantics.ToggleUnset:
					c.report(n, "a tab without a selected state", "set h.AriaSelected(...) to whether it is the open tab")
				case n.Role == "heading" && n.Level == 0:
					c.report(n, "a heading without a level", "use h.H1 to h.H6, or set h.AriaLevel(...)")
				}
			}},

		&ruleDef{Rule: Rule{ID: "aria-valid-attr-value", Description: "AriaControls and AriaActiveDescendant name elements that are in the tree, and an open popup says what it controls", Impact: "critical",
			Tags: []string{"aria", "wcag2a", "wcag412"}, Default: true},
			node: func(c *auditCtx, n *semantics.Node) {
				for _, ref := range []struct {
					name string
					id   semantics.NodeID
				}{{"AriaControls", n.Controls}, {"AriaActiveDescendant", n.ActiveDescendant}} {
					if ref.id == 0 {
						continue
					}
					if t := c.byID[ref.id]; t == nil {
						c.report(n, ref.name+" names an element that is not in the tree", "point it at an element that is rendered (an ElementRef whose element is mounted), or drop the prop")
					} else if t.Has(semantics.StateHidden) && n.Expanded == semantics.ToggleTrue {
						c.report(n, ref.name+" names a hidden element while expanded", "render the popup while the trigger is expanded, or do not mark the trigger expanded")
					}
				}
				if n.Expanded == semantics.ToggleTrue && n.HasPopup != "" && n.Controls == 0 {
					c.report(n, "an open popup trigger without AriaControls", "set h.AriaControls(...) to the popup's ElementRef")
				}
			}},

		&ruleDef{Rule: Rule{ID: "set-position", Description: "an item of a set (option, menu item, tab, radio, tree item) is inside its set, so that assistive technologies can say \"2 of 5\"", Impact: "moderate",
			Tags: []string{"aria", "tela"}, Default: true},
			node: func(c *auditCtx, n *semantics.Node) {
				if slices.Contains(setItemRoles, n.Role) && (n.PosInSet == 0 || n.SetSize == 0) {
					c.report(n, fmt.Sprintf("a %s outside a set (position %d, size %d)", n.Role, n.PosInSet, n.SetSize),
						"put it inside its container (listbox, menu, tablist, radiogroup, tree, list), or set h.AriaPosInSet(...) and h.AriaSetSize(...)")
				}
			}},

		&ruleDef{Rule: Rule{ID: "target-size", Description: "a pointer target is at least 24x24 CSS pixels, or has 24 pixels of space around it (WCAG 2.2 success criterion 2.5.8)", Impact: "serious",
			Tags: []string{"wcag22aa", "wcag258"}, Default: true},
			all: checkTargetSize},
	)
}

// targetRoles are the roles of the elements a pointer acts on.
var targetRoles = []string{"button", "link", "checkbox", "switch", "radio", "tab", "menuitem", "menuitemcheckbox", "menuitemradio",
	"option", "treeitem", "slider", "spinbutton", "combobox", "textbox", "searchbox"}

// minTarget is the size WCAG 2.5.8 asks for, in CSS pixels, and the
// diameter of the circle of the spacing exception.
const minTarget = 24

// checkTargetSize implements WCAG 2.2's 2.5.8 as axe's target-size rule
// does: a target smaller than 24x24 passes when a 24 px circle centered
// on its box meets no other target (an undersized one counts as its own
// circle); targets inline in a sentence and inactive ones are exempt, and
// so are a target and the targets inside or around it, which share their
// box. The equivalent-target exception is not applied.
func checkTargetSize(c *auditCtx) {
	type target struct {
		n *semantics.Node
		r gfx.Rect
	}
	var all []target
	c.root.Walk(func(n *semantics.Node) bool {
		if n.Has(semantics.StateHidden) || c.excluded[n.ID] {
			return false
		}
		if n.Kind == semantics.KindElement && slices.Contains(targetRoles, n.Role) && !n.Bounds.Empty() &&
			!n.Inline && !c.disabled(n) {
			all = append(all, target{n, n.Bounds})
		}
		return true
	})
	related := func(a, b *semantics.Node) bool {
		for p := a; p != nil; p = c.parent[p] {
			if p == b {
				return true
			}
		}
		for p := b; p != nil; p = c.parent[p] {
			if p == a {
				return true
			}
		}
		return false
	}
	// The same box of a 1 px tolerance for rounding.
	small := func(r gfx.Rect) bool { return r.Width() < minTarget-0.5 || r.Height() < minTarget-0.5 }
	for _, t := range all {
		if !small(t.r) {
			continue
		}
		ctr := t.r.Center()
		var near *target
		var nearGap float32 = math.MaxFloat32
		for i := range all {
			o := &all[i]
			if o.n == t.n || related(o.n, t.n) {
				continue
			}
			var gap float32 // how far the other target is, for the message
			if small(o.r) {
				oc := o.r.Center()
				d := float32(math.Hypot(float64(ctr.X-oc.X), float64(ctr.Y-oc.Y)))
				if d >= minTarget {
					continue
				}
				gap = d
			} else {
				// The circle meets the other box when the distance from
				// its center to the box is under the radius.
				dx := max(o.r.X0-ctr.X, 0, ctr.X-o.r.X1)
				dy := max(o.r.Y0-ctr.Y, 0, ctr.Y-o.r.Y1)
				d := float32(math.Hypot(float64(dx), float64(dy)))
				if d >= minTarget/2 {
					continue
				}
				gap = d
			}
			if gap < nearGap {
				near, nearGap = o, gap
			}
		}
		if near == nil {
			continue
		}
		c.report(t.n,
			fmt.Sprintf("a %gx%g target with another target too close (%s, %.0f px from its center); WCAG 2.2 asks for 24x24 or a 24 px circle of space around it",
				round1(t.r.Width()), round1(t.r.Height()), c.describe(near.n), nearGap),
			"make it at least 24x24 (min-h-6 min-w-6, or padding), or space the targets apart (gap-2)")
	}
}

func round1(v float32) float32 { return float32(math.Round(float64(v)*10) / 10) }
