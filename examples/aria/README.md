# aria

A "Command center" screen in the style of shadcn/ui, built **only from the behavior hooks of package [`aria`](../../aria), the elements of package `h` and Tailwind classes with the theme tokens**: no `kit` components and no hand-written key handling. It is the validation of the phase-8 criterion: a dialog, a menu and a combobox built only from `aria` hooks pass keyboard and aria-snapshot tests (`go test ./examples/aria`).

| Light | Dark |
|---|---|
| ![the Actions menu open](../../docs/images/aria.png) | ![the New deployment dialog open](../../docs/images/aria-dark.png) |

Every color is a theme token (`bg-background`, `text-muted-foreground`, `border-input`, `ring-ring/50`, `bg-chart-1`...), with the radius set to 10 px in `App.Theme`, so one set of classes serves both schemes (`--dark` renders the second). Tab to anything and the `focus-visible:` ring shows; click and it does not.

## What each part shows

| Part | Hooks | File |
|---|---|---|
| **Project combobox**: thirty projects, an icon per kind, filtered while typing | `UseCombobox` (`Items`, `aria.Contains`, `AriaLabel`, `ButtonProps` with `ButtonLabel`, `MatchWidth`, `AnchorRef`), `UseFloating` through its popover | `search.go` |
| **Bell with a tooltip** | `UseTooltipTrigger`, `UseFloating` with `Offset`, `Flip`, `Shift` and an `Arrow` | `bell.go` |
| **View** menu: checkbox items that show and hide the columns of the list | `UseMenuTrigger`, `UsePopover` (`Modal`), `UseMenu` (`SelectionMode: "multiple"`) | `menu.go` |
| **Actions** menu: icons, shortcuts, two sections with a separator, a disabled item and a destructive one | the same, `SelectionMode: "none"`, `Section` | `menu.go`, `main.go` |
| **New deployment** dialog with a form: a text field, a select, a switch, Cancel and a submit button | `UseOverlayTrigger`, `UseOverlay` (`Modal`, `Dismissable`), `UseDialog`, `UseSelect`, `UseSwitch`, `UseButton` (`Type: "submit"`), `Form`/`Label`/`Input` | `dialog.go` |
| **Tabs** | `UseTabs` | `workspace.go` |
| **People to notify**: a list box with a multiple selection | `UseListBox` (`SelectionMode: "multiple"`, a controlled `Selection`) | `notify.go` |
| **Entrances and exits** of every floating element | `Presence`, `Exit`, `starting:` and `transition` classes | everywhere |

The menu items, the combobox options and the tabs are plain `Div`s with the props of `ItemProps(key)` and `TabProps(key)`, styled by the states the hooks set: `focus:` (a menu's roving focus), `aria-selected:` and `aria-disabled:`, or Go booleans (`IsFocused`, `IsSelected`) for the combobox's virtual focus. The icons are a subset of Lucide in `icons/`: the root module cannot import `icons/lucide`, so `icons/gen.go` runs `tela icons gen` over 27 SVG files and Lucide's license (see `THIRD_PARTY.md`).

## Previews

`var Previews` has the main screen and every overlay open, each seeded through the fields of the `Page` struct component (no global): `1 Overview`, `2 Actions menu`, `3 View menu`, `4 Combobox`, `5 Tooltip`, `6 Dialog`, `7 Dialog select` and `8 Activity tab`.

```bash
CGO_ENABLED=0 go run ./cmd/tela snapshot --preview "6 Dialog" --dark --scale 2 -o dialog.png ./examples/aria
CGO_ENABLED=0 go run ./cmd/tela inspect --preview "2 Actions menu" ./examples/aria   # the aria snapshot
CGO_ENABLED=0 go run ./examples/aria                                                 # the window
```

## Tests

`CGO_ENABLED=0 go test ./examples/aria`. The keyboard tests use **only the keyboard** (Tab, Shift+Tab, the arrows, Home, End, Enter, Space, Escape, typing) and assert what a screen reader reads, with `ExpectAria` and the aria snapshot, and where the focus is:

| Test | Asserts |
|---|---|
| `TestTabOrder` | the tab order of the whole screen, and back |
| `TestActionsMenuKeyboard` | Enter, Space, ArrowDown and ArrowUp open the menu on its first or last item; the full snapshot of the open menu (sections, separator, a disabled item) with the rest of the window inert; arrows skip the disabled item; Home, End and typeahead; Escape and choosing give the focus back to the button |
| `TestViewMenuKeyboard` | a menu of `menuitemcheckbox`: Space toggles and the menu stays open; the list follows |
| `TestComboboxKeyboard` | typing opens and filters the listbox (its snapshot), the focus stays on the input while the arrows highlight an option (its `aria-activedescendant`, `telatest.ActiveDescendant`), Enter chooses, Escape and Tab put the text back, ArrowDown opens it with all thirty projects |
| `TestDialogKeyboard`, `TestDialogImplicitSubmissionAndCancel` | the full snapshot of the open modal dialog (the page is inert), the focus goes in, Tab goes round it and never leaves, the select inside it, Escape closes the select before the dialog, Space flips the switch, Enter submits (on Deploy and in the field), Escape and Cancel close, and the focus returns to the opening button every time |
| `TestTabsKeyboard`, `TestListBoxKeyboard`, `TestTooltipKeyboard` | arrows and Home/End on tabs; Space, Ctrl+A, Escape and typeahead on a multiple selection; the tooltip opens with the keyboard focus and Escape closes it |

`pointer_test.go` drives the same widgets with the mouse (the menu opens with the menu focused, the item under the pointer takes the focus, a press outside closes and a press on the scrim closes the dialog, the tooltip's delays on the frame clock). `golden_test.go` has the goldens in `testdata/`: every preview, light and dark, at scales 1 and 2, bit by bit, plus the focus ring and the highlighted option (regenerate with `TELA_UPDATE=1 CGO_ENABLED=0 go test -run Golden ./examples/aria` and look at them). `damage_test.go` checks every frame of the overlays against a full rendering, and `idle_test.go` that an open overlay asks for no frames.
