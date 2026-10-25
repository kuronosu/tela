package kit_test

import (
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/kuronosu/tela"
	"github.com/kuronosu/tela/h"
	"github.com/kuronosu/tela/kit"
	"github.com/kuronosu/tela/telatest"
)

// shadcn/ui's SelectTrigger, SelectContent, SelectLabel, SelectItem and
// SelectSeparator (new-york-v4/ui/select.tsx at kit.ShadcnCommit), verbatim.
const (
	shadcnSelectTrigger = "flex w-fit items-center justify-between gap-2 rounded-md border border-input bg-transparent px-3 py-2 text-sm whitespace-nowrap shadow-xs transition-[color,box-shadow] outline-none focus-visible:border-ring focus-visible:ring-[3px] focus-visible:ring-ring/50 disabled:cursor-not-allowed disabled:opacity-50 aria-invalid:border-destructive aria-invalid:ring-destructive/20 data-[placeholder]:text-muted-foreground data-[size=default]:h-9 data-[size=sm]:h-8 *:data-[slot=select-value]:line-clamp-1 *:data-[slot=select-value]:flex *:data-[slot=select-value]:items-center *:data-[slot=select-value]:gap-2 dark:bg-input/30 dark:hover:bg-input/50 dark:aria-invalid:ring-destructive/40 [&_svg]:pointer-events-none [&_svg]:shrink-0 [&_svg:not([class*='size-'])]:size-4 [&_svg:not([class*='text-'])]:text-muted-foreground"

	shadcnSelectContent = "relative z-50 max-h-(--radix-select-content-available-height) min-w-[8rem] origin-(--radix-select-content-transform-origin) overflow-x-hidden overflow-y-auto rounded-md border bg-popover text-popover-foreground shadow-md data-[side=bottom]:slide-in-from-top-2 data-[side=left]:slide-in-from-right-2 data-[side=right]:slide-in-from-left-2 data-[side=top]:slide-in-from-bottom-2 data-[state=closed]:animate-out data-[state=closed]:fade-out-0 data-[state=closed]:zoom-out-95 data-[state=open]:animate-in data-[state=open]:fade-in-0 data-[state=open]:zoom-in-95"
	// Popper position: the viewport is p-1 with its trigger-sized variables.
	shadcnSelectViewport = "p-1 h-[var(--radix-select-trigger-height)] w-full min-w-[var(--radix-select-trigger-width)] scroll-my-1"
	shadcnSelectLabel    = "px-2 py-1.5 text-xs text-muted-foreground"
	shadcnSelectItem     = "relative flex w-full cursor-default items-center gap-2 rounded-sm py-1.5 pr-8 pl-2 text-sm outline-hidden select-none focus:bg-accent focus:text-accent-foreground data-[disabled]:pointer-events-none data-[disabled]:opacity-50 [&_svg]:pointer-events-none [&_svg]:shrink-0 [&_svg:not([class*='size-'])]:size-4 [&_svg:not([class*='text-'])]:text-muted-foreground *:[span]:last:flex *:[span]:last:items-center *:[span]:last:gap-2"
	shadcnSelectSep      = "pointer-events-none -mx-1 my-1 h-px bg-border"
)

var fruitItems = []kit.SelectItem[string]{
	{Value: "apple", Label: "Apple"},
	{Value: "banana", Label: "Banana"},
	{Value: "blueberry", Label: "Blueberry"},
	{Value: "grapes", Label: "Grapes", Disabled: true},
	{Value: "pineapple", Label: "Pineapple"},
}

// selectBox gives an open select the room its list takes, since the list
// is a portal and takes none.
type selectBox struct {
	height int
	sel    kit.Select[string]
}

func (b selectBox) Build() tela.Node {
	return h.Div(h.Class(fmt.Sprintf("h-[%dpx]", b.height)), b.sel)
}

// selectHTML is the markup shadcn's Select renders: the trigger, and for
// an open select the content in a box under it (Radix puts it in a portal
// at the same place).
func selectHTML(s kit.Select[string], probe string, value string, open, focused bool) string {
	var sb strings.Builder
	sb.WriteString(`<div class="relative w-fit">`)
	size := "default"
	if s.Size == kit.Sm {
		size = "sm"
	}
	attrs := fmt.Sprintf(` data-probe=%q`, probe)
	if value == "" {
		attrs += ` data-placeholder=""`
	}
	if s.Disabled {
		attrs += " disabled"
	}
	if s.Invalid {
		attrs += ` aria-invalid="true"`
	}
	state := "closed"
	if open {
		state = "open"
	}
	ic := ""
	for _, it := range s.Items {
		if it.Value == s.DefaultValue && it.Icon != (tela.Icon{}) {
			ic = svgHTML(it.Icon)
		}
	}
	shown := value
	if shown == "" {
		shown = s.Placeholder
	}
	fmt.Fprintf(&sb, `<button type="button" role="combobox" aria-expanded=%t aria-autocomplete="none" dir="ltr" data-state=%q%s data-slot="select-trigger" data-size=%q class=%q>`,
		open, state, attrs, size, cn(shadcnSelectTrigger, s.Class))
	fmt.Fprintf(&sb, `<span data-slot="select-value" style="pointer-events: none;">%s%s</span>%s</button>`, ic, shown, chevronDownHTML())
	if open {
		sb.WriteString(`<div class="absolute top-full left-0 mt-1 min-w-full">`)
		fmt.Fprintf(&sb, `<div data-side="bottom" data-align="center" data-state="open" role="listbox" data-slot="select-content" class=%q>`, shadcnSelectContent)
		fmt.Fprintf(&sb, `<div role="presentation" class=%q>`, shadcnSelectViewport)
		group, inGroup := "", false
		closeGroup := func() {
			if inGroup {
				sb.WriteString(`</div>`)
			}
			inGroup = false
		}
		for i, it := range s.Items {
			if i > 0 && it.Group != s.Items[i-1].Group {
				closeGroup()
				fmt.Fprintf(&sb, `<div data-slot="select-separator" aria-hidden="true" class=%q></div>`, shadcnSelectSep)
			}
			if it.Group != "" && (!inGroup || it.Group != group) {
				closeGroup()
				group, inGroup = it.Group, true
				fmt.Fprintf(&sb, `<div role="group"><div data-slot="select-label" class=%q>%s</div>`, shadcnSelectLabel, it.Group)
			}
			extra, check := "", ""
			if it.Disabled {
				extra = ` data-disabled="" aria-disabled="true"`
			}
			cls := shadcnSelectItem
			if it.Label == value {
				check = `<span><svg class="lucide lucide-check size-4" xmlns="http://www.w3.org/2000/svg" width="24" height="24" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round"><path d="M20 6 9 17l-5-5"></path></svg></span>`
				// The chosen option has the focus when the list opens, and
				// its focus: classes apply. Only one element of a window has
				// the focus, so a sheet marks one list as the focused one.
				if focused {
					cls = cn(cls, "bg-accent text-accent-foreground")
				}
			}
			iconHTML := ""
			if it.Icon != (tela.Icon{}) {
				iconHTML = svgHTML(it.Icon)
			}
			fmt.Fprintf(&sb, `<div role="option" data-slot="select-item"%s class=%q><span data-slot="select-item-indicator" class="absolute right-2 flex size-3.5 items-center justify-center">%s</span><span>%s%s</span></div>`,
				extra, cls, check, iconHTML, it.Label)
		}
		closeGroup()
		sb.WriteString(`</div></div></div>`)
	}
	sb.WriteString(`</div>`)
	return sb.String()
}

func chevronDownHTML() string {
	return `<svg class="lucide lucide-chevron-down size-4 opacity-50" xmlns="http://www.w3.org/2000/svg" width="24" height="24" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round"><path d="m6 9 6 6 6-6"></path></svg>`
}

// selectCell is a select on both sides. A cell with an open list is
// given room under the trigger.
//
// Every open list of a sheet takes the focus when it opens, and a window
// has one focus: the first open select of the page keeps it (the "open"
// cell of the light panel), so only that list shows the chosen option
// highlighted, and the dark panel's copy of it has no highlight where
// Chromium's has (a few levels over one row).
func selectCell(name string, s kit.Select[string], height int) cell {
	s.Props = h.TestID(name + "-trigger")
	if s.Class == "" {
		s.Class = "w-[180px]"
	}
	value := ""
	for _, it := range s.Items {
		if it.Value == s.DefaultValue && s.DefaultValue != "" {
			value = it.Label
		}
	}
	html := selectHTML(s, name+"-trigger", value, s.DefaultOpen, name == "open")
	var c tela.Component = s
	if height > 0 {
		c = selectBox{height: height, sel: s}
		html = fmt.Sprintf(`<div class="h-[%dpx]">%s</div>`, height, html)
	}
	return cell{name: name, width: 200, probes: []string{name + "-trigger"}, tela: c, html: html}
}

var groupedItems = []kit.SelectItem[string]{
	{Value: "apple", Label: "Apple", Group: "Fruits"},
	{Value: "banana", Label: "Banana", Group: "Fruits"},
	{Value: "carrot", Label: "Carrot", Group: "Vegetables"},
	{Value: "broccoli", Label: "Broccoli", Group: "Vegetables", Disabled: true},
	{Value: "bread", Label: "Bread"},
}

func selectSheet() *sheet {
	icons := []kit.SelectItem[string]{
		{Value: "mail", Label: "Mail", Icon: iconMail},
		{Value: "done", Label: "Done", Icon: iconCheck},
		{Value: "alert", Label: "Alert", Icon: iconCircleAlert},
	}
	return &sheet{name: "select", slot: 220, cells: []cell{
		selectCell("placeholder", kit.Select[string]{Items: fruitItems, Placeholder: "Select a fruit"}, 0),
		selectCell("value", kit.Select[string]{Items: fruitItems, DefaultValue: "banana", Placeholder: "Select a fruit"}, 0),
		selectCell("small", kit.Select[string]{Items: fruitItems, DefaultValue: "banana", Size: kit.Sm}, 0),
		selectCell("disabled", kit.Select[string]{Items: fruitItems, Placeholder: "Select a fruit", Disabled: true}, 0),
		selectCell("invalid", kit.Select[string]{Items: fruitItems, Placeholder: "Select a fruit", Invalid: true}, 0),
		selectCell("icon-value", kit.Select[string]{Items: icons, DefaultValue: "done"}, 0),
		selectCell("open", kit.Select[string]{Items: fruitItems, DefaultValue: "banana", DefaultOpen: true}, 220),
		selectCell("open-groups", kit.Select[string]{Items: groupedItems, DefaultValue: "carrot", DefaultOpen: true}, 290),
		selectCell("open-icons", kit.Select[string]{Items: icons, DefaultValue: "mail", DefaultOpen: true}, 150),
	}}
}

func TestSelectGolden(t *testing.T)   { selectSheet().golden(t) }
func TestChromiumSelect(t *testing.T) { selectSheet().chromium(t) }

var _ tela.Component = kit.Select[string]{}

func TestSelectKeyboardAndPointer(t *testing.T) {
	var seen []string
	app := telatest.Mount(t, kit.Select[string]{Items: fruitItems, Placeholder: "Select a fruit", AriaLabel: "Fruit",
		OnValueChange: func(k string) { seen = append(seen, k) }})
	trigger := telatest.ByRole("button", "Select a fruit Fruit")
	app.Expect(trigger, telatest.Expanded(false))
	// The pointer opens the list and choosing closes it.
	app.Click(trigger)
	app.Expect(trigger, telatest.Expanded(true))
	app.ExpectCount(telatest.ByRole("option", ""), 5)
	app.Click(telatest.ByRole("option", "Blueberry"))
	app.Expect(telatest.ByRole("button", "Blueberry Fruit"), telatest.Expanded(false))
	app.ExpectNoText("Pineapple")
	// The keyboard: ArrowDown opens on the chosen option, the next one skips
	// the disabled Grapes, Enter chooses.
	app.Focus(telatest.ByRole("button", "Blueberry Fruit"))
	app.Press("ArrowDown")
	app.Expect(telatest.ByRole("option", "Blueberry"), telatest.Focused(true))
	app.Press("ArrowDown")
	app.Expect(telatest.ByRole("option", "Pineapple"), telatest.Focused(true))
	app.Press("Enter")
	app.Expect(telatest.ByRole("button", "Pineapple Fruit"), telatest.Focused(true))
	// Escape closes without choosing.
	app.Press("Enter")
	app.Press("ArrowUp")
	app.Press("Escape")
	app.Expect(telatest.ByRole("button", "Pineapple Fruit"), telatest.Expanded(false))
	// Typing on the closed trigger chooses.
	app.Type("ban")
	app.Expect(telatest.ByRole("button", "Banana Fruit"), telatest.Expanded(false))
	if got := strings.Join(seen, ","); got != "blueberry,pineapple,banana" {
		t.Errorf("OnValueChange saw %s, want blueberry,pineapple,banana", got)
	}
}

func TestSelectDisabledItemsAreNotChosen(t *testing.T) {
	app := telatest.Mount(t, kit.Select[string]{Items: fruitItems, Placeholder: "Pick", DefaultOpen: true})
	app.Expect(telatest.ByRole("option", "Grapes"), telatest.Disabled(true))
	// The keyboard skips it.
	app.Press("End")
	app.Expect(telatest.ByRole("option", "Pineapple"), telatest.Focused(true))
	app.Press("ArrowUp")
	app.Expect(telatest.ByRole("option", "Blueberry"), telatest.Focused(true))
	app.Click(telatest.ByRole("option", "Apple"))
	app.ExpectText("Apple")
}

// controlledSelect keeps the chosen fruit in its state and refuses grapes
// and pineapple.
type controlledSelect struct{ log *[]string }

func (c controlledSelect) Build() tela.Node {
	v, setV := tela.UseState("apple")
	items := append([]kit.SelectItem[string](nil), fruitItems...)
	items[3].Disabled = false
	return h.Div(
		kit.Select[string]{Items: items, Value: &v, AriaLabel: "Fruit", OnValueChange: func(k string) {
			*c.log = append(*c.log, k)
			if k != "pineapple" {
				setV.Set(k)
			}
		}},
		kit.Button{OnClick: func() { setV.Set("blueberry") }, Children: h.Text("Blueberry please")},
		kit.Button{OnClick: func() { setV.Set("") }, Children: h.Text("Clear")},
	)
}

func TestSelectControlled(t *testing.T) {
	var log []string
	app := telatest.Mount(t, controlledSelect{&log})
	app.Expect(telatest.ByRole("button", "Apple Fruit"), telatest.Expanded(false))
	app.Click(telatest.ByRole("button", "Apple Fruit"))
	app.Click(telatest.ByRole("option", "Pineapple")) // refused
	app.Expect(telatest.ByRole("button", "Apple Fruit"), telatest.Expanded(false))
	app.Click(telatest.ByRole("button", "Apple Fruit"))
	app.Click(telatest.ByRole("option", "Grapes"))
	app.Expect(telatest.ByRole("button", "Grapes Fruit"), telatest.Expanded(false))
	app.Click(telatest.ByRole("button", "Blueberry please"))
	app.Expect(telatest.ByRole("button", "Blueberry Fruit"), telatest.Expanded(false))
	// A value no item has chooses nothing.
	app.Click(telatest.ByRole("button", "Clear"))
	app.Expect(telatest.ByRole("button", "Fruit"), telatest.Expanded(false))
	if got := strings.Join(log, ","); got != "pineapple,grapes" {
		t.Errorf("OnValueChange saw %s, want pineapple,grapes", got)
	}
}

func TestSelectOpenState(t *testing.T) {
	var opens []bool
	open := false
	app := telatest.MountFunc(t, func() tela.Node {
		o, setO := tela.UseState(open)
		return h.Div(kit.Select[string]{Items: fruitItems, Placeholder: "Pick", Open: &o,
			OnOpenChange: func(v bool) { opens = append(opens, v); setO.Set(v) }})
	})
	app.Click(telatest.ByRole("button", "Pick"))
	app.Click(telatest.ByRole("option", "Apple"))
	if fmt.Sprint(opens) != "[true false]" {
		t.Errorf("OnOpenChange saw %v, want [true false]", opens)
	}
}

func TestSelectGroupsAndSeparators(t *testing.T) {
	snap, err := telatest.AriaSnapshot(kit.Select[string]{Items: groupedItems, DefaultOpen: true, Placeholder: "Pick"})
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{`group "Fruits"`, `group "Vegetables"`, `option "Apple"`, `option "Broccoli" [disabled]`, `option "Bread"`} {
		if !strings.Contains(snap, want) {
			t.Errorf("the aria snapshot lacks %q:\n%s", want, snap)
		}
	}
}

func TestSelectInAField(t *testing.T) {
	app := telatest.Mount(t, kit.Field{Label: "Favorite fruit", Description: "Pick one.", Error: "Required.",
		Children: kit.Select[string]{Items: fruitItems, Placeholder: "Select a fruit"}})
	trigger := telatest.ByRole("button", "Select a fruit Favorite fruit")
	app.Expect(trigger, telatest.Invalid(true))
	// The label points to the trigger: a click on it reaches the trigger.
	app.Click(telatest.ByText("Favorite fruit", telatest.Exact()))
	app.Expect(trigger, telatest.Expanded(true))
}

func TestSelectUndefinedSizePanics(t *testing.T) {
	_, err := telatest.Render(kit.Select[string]{Items: fruitItems, Size: "huge"})
	if err == nil || !strings.Contains(err.Error(), `Select.Size "huge" is not defined`) {
		t.Errorf("error %v, want the undefined size reported", err)
	}
}

func TestSelectOpenListRequestsNoFrames(t *testing.T) {
	app := telatest.Mount(t, kit.Select[string]{Items: fruitItems, Placeholder: "Pick", DefaultOpen: true}, telatest.NoSettle())
	app.Advance(time.Second)
	before := app.Stats()
	app.Advance(3 * time.Second)
	if after := app.Stats(); after.Frames != before.Frames {
		t.Errorf("an open select drew %d frames while idle, want 0", after.Frames-before.Frames)
	}
}

func ExampleSelect() {
	t := &exampleT{}
	defer t.close()
	var chosen string
	page := kit.Select[string]{
		Placeholder: "Select a fruit",
		AriaLabel:   "Fruit",
		Items: []kit.SelectItem[string]{
			{Value: "apple", Label: "Apple"},
			{Value: "banana", Label: "Banana"},
		},
		OnValueChange: func(v string) { chosen = v },
	}
	app := telatest.Mount(t, page)
	app.Click(telatest.ByRole("button", "Select a fruit Fruit"))
	app.Click(telatest.ByRole("option", "Banana"))
	fmt.Println("chose", chosen)
	// Output: chose banana
}
