# Recipe: accessibility

Questions: how to name an icon button or a custom control; which role and aria props to set; keyboard access and focus rings; access keys (mnemonics) in menus; high contrast and reduced motion; how to check a screen for contrast, cut-off text, small targets and missing names (`tela check`, `tela verify`, `ExpectAccessible`).

Every element has a place in a semantics tree: a role, an accessible name, states and a value, computed as browsers do. The tree is what `tela inspect` and the `snapshot` tool print, what `telatest.ByRole` queries, and what the operating system's screen readers read. Prefer `kit` components (roles, keys and focus are done) and real elements: `Button` is a focusable button, `H1`..`H6` give headings, `Input` a textbox named by its `Label` (or `kit.Field`), `A` with `Href` a link, `Img` an image named by `Alt` (`Alt("")` marks a decorative one). `Div` and `Span` have no role and their text merges into the parent's.

**Name everything you can act on.** A control named only by an icon needs `AriaLabel("Close")` (`kit.Button{Size: kit.Icon, AriaLabel: "More"}`); a group is named by its legend (`kit.FieldSet`) or `AriaLabel`; `AriaLabelledBy(refs...)` names an element with the text of others (attached with `Ref`); `AriaDescription` adds a hint. A `Div` that acts like a control takes `Role("switch")` and the states: `AriaChecked`, `AriaPressed`, `AriaExpanded`, `AriaSelected`, `AriaDisabled`, `AriaInvalid`, `AriaRequired`, `AriaReadOnly`, `AriaBusy`, `AriaCurrent("page")`, `AriaSort`, `AriaLevel`, `AriaHasPopup`, `AriaControls`, `AriaHidden` (out of the tree, not off the screen). Ranges: `AriaValueNow/Min/Max/Text` (`...Fn` forms read a `tela.Value` when the tree is built, no rebuild), `AriaPosInSet`/`AriaSetSize` for a virtual list, `AriaRowCount`/`AriaRowIndex` for a table. Live regions: `AriaLive("polite")`, `AriaAtomic`; `Role("status")` and `Role("alert")` imply them. `AriaKeyShortcuts("Control+S")` only describes keys. An unknown role or token panics in dev mode with the closest valid one, and `tela lint` finds it statically.

**Keyboard.** A `Button` takes Enter and Space. Any other element needs `TabIndex(0)` and `OnKeyDown`; give it a visible focus: `outline-none focus-visible:ring-2 focus-visible:ring-ring` (`focus-visible:` is keyboard focus only). A custom target must be at least 24x24 px (`target-size`, WCAG 2.5.8).

**Access keys (mnemonics) in menus (Windows and Linux; not macOS).** `kit.MenuItem.AccessKey` and `kit.MenubarMenu.AccessKey` take a rune (`'F'`): the first matching character of the label is underlined while Alt is held or the keyboard is in the bar, and Alt+key opens a menu, the key alone chooses an item. A `Menubar` that is the window's bar (`AppMenu: true`, `ActivationKeys: true`, or one drawn in place of a native bar) gives every item without one a key by the Win32 bar's rule (the first letter no earlier item took; a Latin letter appended to a label written in Chinese, Japanese or Korean); a native bar gets the keys you set. Items with the same key cycle the focus. `aria.AccessKeys` and `aria.ValidAccessKey` are the rule for your own menus.

```go
package menus

import (
	"github.com/kuronosu/tela"
	"github.com/kuronosu/tela/kit"
)

func AppBar() tela.Node {
	return kit.Menubar[string]{
		AriaLabel: "Application", ActivationKeys: true, // F10, a lone Alt and access keys
		Menus: []kit.MenubarMenu[string]{
			{Key: "file", Label: "File", AccessKey: 'F', Items: []kit.MenuItem[string]{
				{Key: "new", Label: "New", AccessKey: 'N', Shortcut: "CmdOrCtrl+N"},
				{Key: "open", Label: "Open…", AccessKey: 'O', Shortcut: "CmdOrCtrl+O"},
				{Kind: kit.MenuSeparator},
				{Key: "quit", Label: "Exit", AccessKey: 'x'},
			}},
			{Key: "help", Label: "Help", AccessKey: 'H', Items: []kit.MenuItem[string]{{Key: "about", Label: "About"}}},
		},
		OnSelect: func(menu, item string) {},
	}
}
```

**System settings.** `contrast-more:` is the variant for the system's high contrast (`tela.UseHighContrast()` in code): give borders and focus a stronger look there. Text scales with the system's text scale (`rem` units grow, px do not; `tela.UseTextScale()`), so leave room: no fixed heights on text, `min-h-*` instead of `h-*`. `motion-reduce:` and `tela.UseReducedMotion()` follow reduced motion. Try all of them live with `tela mcp`'s `prefer` (`reducedMotion`, `highContrast`, `textScale`, `locale`, `direction: "rtl"`, `dark`).

**Check before you finish (ADR-306, ADR-312).** Run `tela check ./myapp` (the `check` tool of `tela mcp`): it mounts the screen in light and dark and reports, with the source line of each, text cut off or overlapping other text, targets over targets, low contrast (4.5:1; 3:1 for large text), targets under 24x24, controls the keyboard cannot operate, focus that shows nothing, and missing names and states. `tela verify` does it for every preview at 360, 768 and 1280 px, at twice the text size, right to left and in another language (`--widths`, `--text-scale`, `--lang`, `--rules`, `--disable`, `-json`). Exit status 1 means findings; a finding reads `file:line: rule (impact): message`, then `element:`, `fix:` and `in:` (which previews and variants). Fix the cause, not the check: accept one exception only in a test, with `DisableRules` (recipe testing). The color check judges text over a flat color; it does not judge text over an image, a gradient or a blur, or disabled text.

```go
package menus

import (
	"testing"

	"github.com/kuronosu/tela"
	"github.com/kuronosu/tela/kit"
	"github.com/kuronosu/tela/telatest"
)

func TestMenuAccessible(t *testing.T) {
	app := telatest.MountFunc(t, func() tela.Node { return AppBar() }, telatest.Size(640, 200), telatest.Theme(kit.Theme()))
	app.ExpectAccessible()                                             // names, roles, targets, contrast, key handlers
	app.ExpectAccessible(telatest.WithTags("layout"))                  // cut-off or overlapping text and targets
	app.ExpectAccessible(telatest.DisableRules("focus-visible"))
}
```
