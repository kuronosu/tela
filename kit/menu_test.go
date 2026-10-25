package kit_test

import (
	"fmt"
	"html"
	"strings"
	"testing"
	"time"

	"github.com/kuronosu/tela"
	"github.com/kuronosu/tela/internal/core"
	"github.com/kuronosu/tela/internal/platform"
	"github.com/kuronosu/tela/internal/platform/headless"
	"github.com/kuronosu/tela/internal/semantics"
	"github.com/kuronosu/tela/kit"
)

// What the tests of DropdownMenu, Menubar and ContextMenu share: shadcn's
// class strings (new-york-v4/ui/dropdown-menu.tsx, menubar.tsx and
// context-menu.tsx at kit.ShadcnCommit, verbatim) and the markup Radix
// renders for the items of a menu.

const (
	shadcnMenuItem         = "relative flex cursor-default items-center gap-2 rounded-sm px-2 py-1.5 text-sm outline-hidden select-none focus:bg-accent focus:text-accent-foreground data-[disabled]:pointer-events-none data-[disabled]:opacity-50 data-[inset]:pl-8 data-[variant=destructive]:text-destructive data-[variant=destructive]:focus:bg-destructive/10 data-[variant=destructive]:focus:text-destructive dark:data-[variant=destructive]:focus:bg-destructive/20 [&_svg]:pointer-events-none [&_svg]:shrink-0 [&_svg:not([class*='size-'])]:size-4 [&_svg:not([class*='text-'])]:text-muted-foreground data-[variant=destructive]:*:[svg]:text-destructive!"
	shadcnMenuIndicatorRow = "relative flex cursor-default items-center gap-2 %s py-1.5 pr-2 pl-8 text-sm outline-hidden select-none focus:bg-accent focus:text-accent-foreground data-[disabled]:pointer-events-none data-[disabled]:opacity-50 [&_svg]:pointer-events-none [&_svg]:shrink-0 [&_svg:not([class*='size-'])]:size-4"
	shadcnMenuIndicator    = "pointer-events-none absolute left-2 flex size-3.5 items-center justify-center"
	shadcnMenuSeparator    = "-mx-1 my-1 h-px bg-border"
	shadcnMenuShortcut     = "ml-auto text-xs tracking-widest text-muted-foreground"
	shadcnMenuPopup        = "data-[side=bottom]:slide-in-from-top-2 data-[side=left]:slide-in-from-right-2 data-[side=right]:slide-in-from-left-2 data-[side=top]:slide-in-from-bottom-2 data-[state=closed]:animate-out data-[state=closed]:fade-out-0 data-[state=closed]:zoom-out-95 data-[state=open]:animate-in data-[state=open]:fade-in-0 data-[state=open]:zoom-in-95"
)

// menuFlavor is what differs between shadcn's three menus.
type menuFlavor struct {
	slot    string // "dropdown-menu", "menubar", "context-menu"
	content string
	// rounded is the radius class of checkbox and radio items.
	rounded string
	label   string
}

var (
	dropdownFlavor = menuFlavor{
		slot:    "dropdown-menu",
		content: "z-50 max-h-(--radix-dropdown-menu-content-available-height) min-w-[8rem] origin-(--radix-dropdown-menu-content-transform-origin) overflow-x-hidden overflow-y-auto rounded-md border bg-popover p-1 text-popover-foreground shadow-md " + shadcnMenuPopup,
		rounded: "rounded-sm",
		label:   "px-2 py-1.5 text-sm font-medium data-[inset]:pl-8",
	}
	contextFlavor = menuFlavor{
		slot:    "context-menu",
		content: "z-50 max-h-(--radix-context-menu-content-available-height) min-w-[8rem] origin-(--radix-context-menu-content-transform-origin) overflow-x-hidden overflow-y-auto rounded-md border bg-popover p-1 text-popover-foreground shadow-md " + shadcnMenuPopup,
		rounded: "rounded-sm",
		label:   "px-2 py-1.5 text-sm font-medium text-foreground data-[inset]:pl-8",
	}
	menubarFlavor = menuFlavor{
		slot:    "menubar",
		content: "z-50 min-w-[12rem] origin-(--radix-menubar-content-transform-origin) overflow-hidden rounded-md border bg-popover p-1 text-popover-foreground shadow-md " + shadcnMenuPopup,
		rounded: "rounded-xs",
		label:   "px-2 py-1.5 text-sm font-medium data-[inset]:pl-8",
	}
)

// menuContentClasses is the content's classes for a static cell: the
// animation states and what reads a Radix variable are left out, as the
// screenshots show the settled menu.
func menuContentClasses(class string) string {
	var out []string
	for _, c := range strings.Fields(staticClasses(class)) {
		if strings.HasPrefix(c, "max-h-(--radix") {
			continue
		}
		out = append(out, c)
	}
	return strings.Join(out, " ")
}

// menuHTML is the markup of an open menu's content: the panel (marked with
// probe) and its items, as Radix and shadcn render them.
func menuHTML(f menuFlavor, items []kit.MenuItem[string], panelClass, probe string) string {
	var sb strings.Builder
	fmt.Fprintf(&sb, `<div role="menu" aria-orientation="vertical" data-state="open" data-probe=%q data-slot="%s-content" class=%q>`,
		probe, f.slot, cn(menuContentClasses(f.content), panelClass))
	menuEntriesHTML(&sb, f, items, nil)
	sb.WriteString(`</div>`)
	return sb.String()
}

func menuEntriesHTML(sb *strings.Builder, f menuFlavor, items []kit.MenuItem[string], radio *kit.MenuItem[string]) {
	for _, it := range items {
		extra := ""
		if it.Disabled || (radio != nil && radio.Disabled) {
			extra = ` data-disabled="" aria-disabled="true"`
		}
		switch {
		case radio != nil:
			on := radio.DefaultValue == it.Key
			fmt.Fprintf(sb, `<div role="menuitemradio" aria-checked=%t data-state=%q data-slot="%s-radio-item"%s class=%q><span class=%q>`,
				on, checkedState(on), f.slot, extra, cn(fmt.Sprintf(shadcnMenuIndicatorRow, f.rounded), it.Class), shadcnMenuIndicator)
			if on {
				sb.WriteString(`<span data-state="checked"><svg class="lucide size-2 fill-current" xmlns="http://www.w3.org/2000/svg" width="24" height="24" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round"><circle cx="12" cy="12" r="10"></circle></svg></span>`)
			}
			fmt.Fprintf(sb, `</span>%s</div>`, html.EscapeString(it.Label))
			continue
		case it.Kind == kit.MenuCheckbox:
			on := it.DefaultChecked
			fmt.Fprintf(sb, `<div role="menuitemcheckbox" aria-checked=%t data-state=%q data-slot="%s-checkbox-item"%s class=%q><span class=%q>`,
				on, checkedState(on), f.slot, extra, cn(fmt.Sprintf(shadcnMenuIndicatorRow, f.rounded), it.Class), shadcnMenuIndicator)
			if on {
				sb.WriteString(`<span data-state="checked"><svg class="lucide size-4" xmlns="http://www.w3.org/2000/svg" width="24" height="24" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round"><path d="M20 6 9 17l-5-5"></path></svg></span>`)
			}
			fmt.Fprintf(sb, `</span>%s`, html.EscapeString(it.Label))
			if it.Shortcut != "" {
				fmt.Fprintf(sb, `<span data-slot="%s-shortcut" class=%q>%s</span>`, f.slot, shadcnMenuShortcut, html.EscapeString(it.Shortcut))
			}
			sb.WriteString(`</div>`)
			continue
		}
		switch it.Kind {
		case kit.MenuPlain:
			variant := "default"
			if it.Variant == kit.Destructive {
				variant = "destructive"
			}
			if it.Inset {
				extra += ` data-inset="true"`
			}
			fmt.Fprintf(sb, `<div role="menuitem" data-variant=%q data-slot="%s-item"%s class=%q>`, variant, f.slot, extra, cn(shadcnMenuItem, it.Class))
			if it.Icon != (tela.Icon{}) {
				sb.WriteString(svgHTML(it.Icon))
			}
			sb.WriteString(html.EscapeString(it.Label))
			if it.Shortcut != "" {
				fmt.Fprintf(sb, `<span data-slot="%s-shortcut" class=%q>%s</span>`, f.slot, shadcnMenuShortcut, html.EscapeString(it.Shortcut))
			}
			sb.WriteString(`</div>`)
		case kit.MenuLabel:
			if it.Inset {
				extra = ` data-inset="true"`
			}
			fmt.Fprintf(sb, `<div data-slot="%s-label"%s class=%q>%s</div>`, f.slot, extra, cn(f.label, it.Class), html.EscapeString(it.Label))
		case kit.MenuSeparator:
			fmt.Fprintf(sb, `<div role="separator" aria-orientation="horizontal" data-slot="%s-separator" class=%q></div>`, f.slot, cn(shadcnMenuSeparator, it.Class))
		case kit.MenuGroup:
			fmt.Fprintf(sb, `<div role="group" data-slot="%s-group">`, f.slot)
			menuEntriesHTML(sb, f, it.Items, nil)
			sb.WriteString(`</div>`)
		case kit.MenuRadioGroup:
			g := it
			fmt.Fprintf(sb, `<div role="group" data-slot="%s-radio-group">`, f.slot)
			menuEntriesHTML(sb, f, it.Items, &g)
			sb.WriteString(`</div>`)
		}
	}
}

func checkedState(on bool) string {
	if on {
		return "checked"
	}
	return "unchecked"
}

// menuCell is a static menu panel on both sides: a cell whose Tela side is
// the panel built by the kit's own renderer and whose Chromium side is the
// markup of shadcn's.
func menuCell(name string, f menuFlavor, panel tela.Component, items []kit.MenuItem[string], panelClass string) cell {
	return cell{
		name: name, width: 300, probes: []string{name + "-panel"}, tela: panel,
		html: menuHTML(f, items, panelClass, name+"-panel"),
	}
}

// accountItems is shadcn's dropdown menu demo.
func accountItems() []kit.MenuItem[string] {
	return []kit.MenuItem[string]{
		{Kind: kit.MenuLabel, Label: "My Account"},
		{Kind: kit.MenuSeparator},
		{Kind: kit.MenuGroup, Items: []kit.MenuItem[string]{
			{Key: "profile", Label: "Profile", Shortcut: "⇧⌘P"},
			{Key: "billing", Label: "Billing", Shortcut: "⌘B"},
			{Key: "settings", Label: "Settings", Shortcut: "⌘S"},
			{Key: "keyboard", Label: "Keyboard shortcuts", Shortcut: "⌘K"},
		}},
		{Kind: kit.MenuSeparator},
		{Kind: kit.MenuGroup, Items: []kit.MenuItem[string]{
			{Key: "team", Label: "Team"},
			{Key: "invite", Label: "Invite users"},
			{Key: "new-team", Label: "New Team", Shortcut: "⌘+T"},
		}},
		{Kind: kit.MenuSeparator},
		{Key: "github", Label: "GitHub"},
		{Key: "support", Label: "Support"},
		{Key: "api", Label: "API", Disabled: true},
		{Kind: kit.MenuSeparator},
		{Key: "logout", Label: "Log out", Shortcut: "⇧⌘Q"},
	}
}

// kindsItems has a checkbox item in each state, a radio group and an inset
// label.
func kindsItems() []kit.MenuItem[string] {
	return []kit.MenuItem[string]{
		{Kind: kit.MenuLabel, Label: "Appearance"},
		{Kind: kit.MenuSeparator},
		{Kind: kit.MenuCheckbox, Key: "status", Label: "Status Bar", DefaultChecked: true},
		{Kind: kit.MenuCheckbox, Key: "activity", Label: "Activity Bar"},
		{Kind: kit.MenuCheckbox, Key: "panel", Label: "Panel", Disabled: true, DefaultChecked: true},
		{Kind: kit.MenuCheckbox, Key: "minimap", Label: "Minimap", Shortcut: "⌘M"},
		{Kind: kit.MenuSeparator},
		{Kind: kit.MenuLabel, Label: "Theme", Inset: true},
		{Kind: kit.MenuRadioGroup, Key: "theme", DefaultValue: "dark", Items: []kit.MenuItem[string]{
			{Key: "light", Label: "Light"},
			{Key: "dark", Label: "Dark"},
			{Key: "system", Label: "System", Disabled: true},
		}},
	}
}

// iconItems has icons, the destructive variant, inset and disabled items and
// an item that shows the focus classes.
func iconItems() []kit.MenuItem[string] {
	return []kit.MenuItem[string]{
		{Key: "mail", Label: "Email", Icon: iconMail, Shortcut: "⌘E"},
		{Key: "focused", Label: "Focused item", Icon: iconArrowRight, Class: "bg-accent text-accent-foreground"},
		{Key: "inset", Label: "Inset item", Inset: true},
		{Key: "off", Label: "Disabled", Icon: iconMail, Disabled: true},
		{Kind: kit.MenuSeparator},
		{Key: "delete", Label: "Delete account", Icon: iconCircleAlert, Variant: kit.Destructive, Shortcut: "⌫"},
		{Key: "delete-focused", Label: "Delete (focused)", Variant: kit.Destructive, Class: "bg-destructive/10 text-destructive"},
		{Key: "plain", Label: "Plain", Class: "font-semibold"},
	}
}

// menuRaw is a headless window for what telatest has no action for yet: a
// right click, which makes the contextmenu event.
type menuRaw struct {
	t    *testing.T
	b    *headless.Backend
	host *core.Host
	win  *headless.Window
	tree *core.Tree
}

func mountMenuRaw(t *testing.T, page func() tela.Node, w, hgt float32) *menuRaw {
	t.Helper()
	b := headless.New(headless.Options{Size: platform.Size{W: w, H: hgt}})
	theme := kit.Theme()
	host, err := core.NewHost(b, core.Options{
		Config: core.Config{Dev: true, IDPrefix: "tt-", Theme: (*core.Theme)(&theme)},
		Root:   core.NodeOf(tela.C(page)),
	})
	if err != nil {
		t.Fatal(err)
	}
	a := &menuRaw{t: t, b: b, host: host, win: host.Window().(*headless.Window), tree: host.Tree()}
	a.win.SetFocused(true)
	a.settle()
	return a
}

func (a *menuRaw) settle() {
	a.t.Helper()
	for range 50 {
		a.b.Drain(a.host)
		if !a.b.Pending() && !a.tree.Dirty() && !a.host.NeedsFrame() {
			return
		}
		a.b.Advance(a.b.FrameInterval())
	}
}

// advance moves the clock forward by d, a frame at a time: the menu's
// entrance is a transition.
func (a *menuRaw) advance(d time.Duration) {
	a.t.Helper()
	for end := a.b.Now() + d; a.b.Now() < end; {
		a.b.Advance(a.b.FrameInterval())
		a.b.Wake()
		a.b.Drain(a.host)
	}
	a.settle()
}

func (a *menuRaw) event(e platform.PointerEvent) {
	a.t.Helper()
	a.win.Inject(e)
	a.settle()
}

func (a *menuRaw) move(x, y float32) {
	a.t.Helper()
	a.event(platform.PointerEvent{Action: platform.PointerMove, Kind: platform.PointerMouse, ID: 1, Pos: platform.Point{X: x, Y: y}})
}

func (a *menuRaw) rightClick(x, y float32) {
	a.t.Helper()
	a.move(x, y)
	a.event(platform.PointerEvent{Action: platform.PointerDown, Kind: platform.PointerMouse, ID: 1,
		Pos: platform.Point{X: x, Y: y}, Button: platform.ButtonSecondary, Buttons: platform.ButtonSecondary.Mask()})
	a.event(platform.PointerEvent{Action: platform.PointerUp, Kind: platform.PointerMouse, ID: 1,
		Pos: platform.Point{X: x, Y: y}, Button: platform.ButtonSecondary})
	a.advance(300 * time.Millisecond)
}

func (a *menuRaw) leftClick(x, y float32) {
	a.t.Helper()
	a.move(x, y)
	a.event(platform.PointerEvent{Action: platform.PointerDown, Kind: platform.PointerMouse, ID: 1,
		Pos: platform.Point{X: x, Y: y}, Button: platform.ButtonPrimary, Buttons: platform.ButtonPrimary.Mask()})
	a.event(platform.PointerEvent{Action: platform.PointerUp, Kind: platform.PointerMouse, ID: 1,
		Pos: platform.Point{X: x, Y: y}, Button: platform.ButtonPrimary})
}

// snapshot is the aria snapshot of the window.
func (a *menuRaw) snapshot() string { return a.tree.AriaSnapshot() }

// find returns the node of the window's semantics tree with the role and
// name, or nil.
func (a *menuRaw) find(role, name string) *semantics.Node {
	var walk func(n *semantics.Node) *semantics.Node
	walk = func(n *semantics.Node) *semantics.Node {
		if n.Role == role && n.Name == name {
			return n
		}
		for _, c := range n.Children {
			if f := walk(c); f != nil {
				return f
			}
		}
		return nil
	}
	return walk(a.tree.Semantics())
}
