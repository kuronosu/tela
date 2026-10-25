package kit_test

import (
	"fmt"
	"testing"
	"time"

	"github.com/kuronosu/tela"
	"github.com/kuronosu/tela/h"
	"github.com/kuronosu/tela/kit"
	"github.com/kuronosu/tela/telatest"
)

// shadcn/ui's Dialog parts (new-york-v4/ui/dialog.tsx at kit.ShadcnCommit),
// verbatim.
const (
	shadcnDialogContent     = "fixed top-[50%] left-[50%] z-50 grid w-full max-w-[calc(100%-2rem)] translate-x-[-50%] translate-y-[-50%] gap-4 rounded-lg border bg-background p-6 shadow-lg duration-200 outline-none data-[state=closed]:animate-out data-[state=closed]:fade-out-0 data-[state=closed]:zoom-out-95 data-[state=open]:animate-in data-[state=open]:fade-in-0 data-[state=open]:zoom-in-95 sm:max-w-lg"
	shadcnDialogClose       = "absolute top-4 right-4 rounded-xs opacity-70 ring-offset-background transition-opacity hover:opacity-100 focus:ring-2 focus:ring-ring focus:ring-offset-2 focus:outline-hidden disabled:pointer-events-none data-[state=open]:bg-accent data-[state=open]:text-muted-foreground [&_svg]:pointer-events-none [&_svg]:shrink-0 [&_svg:not([class*='size-'])]:size-4"
	shadcnDialogHeader      = "flex flex-col gap-2 text-center sm:text-left"
	shadcnDialogFooter      = "flex flex-col-reverse gap-2 sm:flex-row sm:justify-end"
	shadcnDialogTitle       = "text-lg leading-none font-semibold"
	shadcnDialogDescription = "text-sm text-muted-foreground"
)

// dialogSpec describes a dialog cell.
type dialogSpec struct {
	title, description string
	body               *part
	footer             *part
	hideClose          bool
	class              string
}

// dialogCell is an open dialog's panel on both sides: kit.StaticDialog
// against the markup of shadcn's DialogContent, DialogHeader, DialogTitle,
// DialogDescription and DialogFooter, with the content placed in the flow
// (see staticClasses).
func dialogCell(name string, s dialogSpec) cell {
	d := kit.Dialog{HideClose: s.hideClose, Class: s.class, Props: h.TestID(name + "-panel")}
	var inner string
	var header string
	if s.title != "" {
		d.Title = h.Text(s.title)
		header += fmt.Sprintf(`<h2 data-slot="dialog-title" class="%s">%s</h2>`, shadcnDialogTitle, s.title)
	}
	if s.description != "" {
		d.Description = h.Text(s.description)
		header += fmt.Sprintf(`<p data-slot="dialog-description" class="%s">%s</p>`, shadcnDialogDescription, s.description)
	}
	if header != "" {
		inner += fmt.Sprintf(`<div data-slot="dialog-header" class="%s">%s</div>`, shadcnDialogHeader, header)
	}
	if s.body != nil {
		d.Children = s.body.c
		inner += s.body.html
	}
	if s.footer != nil {
		d.Footer = s.footer.c
		inner += fmt.Sprintf(`<div data-slot="dialog-footer" class="%s">%s</div>`, shadcnDialogFooter, s.footer.html)
	}
	if !s.hideClose {
		inner += fmt.Sprintf(`<button data-slot="dialog-close" class="%s">%s<span class="sr-only">Close</span></button>`,
			shadcnDialogClose, svgHTML(iconX))
		d.CloseIcon = iconX
	}
	return cell{
		name: name, width: 560, probes: []string{name + "-panel"}, tela: kit.StaticDialog(d),
		html: fmt.Sprintf(`<div role="dialog" data-probe="%s-panel" data-slot="dialog-content" class="%s">%s</div>`,
			name, cn("relative", staticClasses(shadcnDialogContent), s.class), inner),
	}
}

// profileForm is the body of shadcn's "Edit profile" dialog: two labeled
// inputs in a grid.
func profileForm() *part {
	field := func(label, value string) part {
		return part{
			c: h.Div(h.Class("grid gap-3"), h.Div(h.Class(kit.DefaultStyle().Label.Root), h.Text(label)), kit.Input{DefaultValue: value}),
			html: fmt.Sprintf(`<div class="grid gap-3"><label class="%s">%s</label><input value=%q class="%s"></div>`,
				shadcnLabel, label, value, shadcnInput),
		}
	}
	name, user := field("Name", "Pedro Duarte"), field("Username", "@peduarte")
	return &part{
		c:    h.Div(h.Class("grid gap-4"), name.c, user.c),
		html: `<div class="grid gap-4">` + name.html + user.html + `</div>`,
	}
}

func dialogSheet() *sheet {
	footer := parts(
		btn(kit.Button{Variant: kit.Outline, Children: h.Text("Cancel")}, "Cancel"),
		btn(kit.Button{Children: h.Text("Save changes")}, "Save changes"))
	text := &part{c: h.P(h.Text("Anyone with the link can view this document.")),
		html: `<p>Anyone with the link can view this document.</p>`}
	return &sheet{name: "dialog", cells: []cell{
		dialogCell("edit-profile", dialogSpec{title: "Edit profile", description: "Make changes to your profile here. Click save when you're done.",
			body: profileForm(), footer: &footer}),
		dialogCell("no-close", dialogSpec{title: "Share link", description: "Anyone with the link can view this document.", footer: &footer, hideClose: true}),
		dialogCell("title-only", dialogSpec{title: "Are you sure?", body: text}),
		dialogCell("content-only", dialogSpec{body: text, footer: &footer, hideClose: true}),
		dialogCell("class", dialogSpec{title: "Narrow", description: "A dialog with its own width.", class: "sm:max-w-sm"}),
	}}
}

func TestDialogGolden(t *testing.T)   { dialogSheet().golden(t) }
func TestChromiumDialog(t *testing.T) { dialogSheet().chromium(t) }

// editDialog is the dialog of the behavior tests: a button, a form with a
// text field, and two buttons in the footer.
func editDialog(d kit.Dialog) kit.Dialog {
	if d.Trigger == nil {
		d.Trigger = kit.Button{Variant: kit.Outline, Children: h.Text("Edit profile")}
	}
	d.Title = h.Text("Edit profile")
	d.Description = h.Text("Make changes to your profile here.")
	d.Children = h.Div(h.Class("grid gap-3"), kit.Label{For: "name", Children: h.Text("Name")}, kit.Input{Name: "name", Placeholder: "Your name", ID: "name"})
	d.Footer = h.Fragment(
		kit.Button{Variant: kit.Outline, Children: h.Text("Cancel")},
		kit.Button{Children: h.Text("Save changes")})
	return d
}

var (
	editButton  = telatest.ByRole("button", "Edit profile")
	profileDlg  = telatest.ByRole("dialog", "Edit profile")
	closeButton = telatest.ByRole("button", "Close")
)

func TestDialogOpenAndClose(t *testing.T) {
	page := pageUnder(editDialog(kit.Dialog{}))
	app := telatest.MountFunc(t, page, telatest.Theme(kit.Theme()))
	app.ExpectCount(telatest.ByRole("dialog", ""), 0)
	app.Expect(editButton, telatest.HasPopup("dialog"), telatest.Expanded(false))

	// A click opens it: the dialog is the only thing the aria tree has.
	app.Click(editButton)
	app.ExpectCount(profileDlg, 1)
	app.ExpectAria(`
		- dialog "Edit profile":
		  - heading "Edit profile" [level=2]
		  - paragraph: Make changes to your profile here.
		  - text: Name
		  - textbox "Name"
		  - button "Cancel"
		  - button "Save changes"
		  - button "Close"
	`)
}

func TestDialogIsModal(t *testing.T) {
	var saved int
	page := pageUnder(editDialog(kit.Dialog{}), kit.Button{Children: h.Text("Other"), OnClick: func() { saved++ }})
	app := telatest.MountFunc(t, page, telatest.Theme(kit.Theme()))
	app.Click(editButton)
	// The page behind is inert: it is not in the semantics tree and a press
	// on it reaches nothing.
	app.ExpectCount(telatest.ByRole("heading", "Project settings"), 0)
	app.ExpectCount(telatest.ByRole("button", "Other"), 0)
	if saved != 0 {
		t.Fatalf("saved %d", saved)
	}
}

func TestDialogFocus(t *testing.T) {
	page := pageUnder(editDialog(kit.Dialog{}))
	app := telatest.MountFunc(t, page, telatest.Theme(kit.Theme()))
	app.Focus(editButton)
	app.Press("Enter")
	app.ExpectCount(profileDlg, 1)

	// The first field has the focus, Tab goes round the dialog and never
	// leaves it.
	app.ExpectFocused(telatest.ByRole("textbox", "Name"))
	for _, name := range []string{"Cancel", "Save changes", "Close", ""} {
		app.Press("Tab")
		if name == "" {
			app.ExpectFocused(telatest.ByRole("textbox", "Name"))
			continue
		}
		app.ExpectFocused(telatest.ByRole("button", name))
	}
	app.Press("Shift+Tab")
	app.ExpectFocused(closeButton)

	// Escape closes it and the focus goes back to the trigger.
	app.Press("Escape")
	app.ExpectCount(telatest.ByRole("dialog", ""), 0)
	app.ExpectFocused(editButton)
	app.Expect(editButton, telatest.Expanded(false))
}

func TestDialogWaysToClose(t *testing.T) {
	page := pageUnder(editDialog(kit.Dialog{}))
	for _, tc := range []struct {
		name  string
		close func(*telatest.App)
	}{
		{"the close button", func(a *telatest.App) { a.Click(closeButton) }},
		{"the close button with the keyboard", func(a *telatest.App) { a.Focus(closeButton); a.Press("Space") }},
		{"Escape", func(a *telatest.App) { a.Press("Escape") }},
		{"a press on the scrim", func(a *telatest.App) { a.ClickAt(10, 10) }},
	} {
		t.Run(tc.name, func(t *testing.T) {
			app := telatest.MountFunc(t, page, telatest.Theme(kit.Theme()))
			app.Click(editButton)
			app.ExpectCount(profileDlg, 1)
			tc.close(app)
			app.ExpectCount(telatest.ByRole("dialog", ""), 0)
			app.ExpectFocused(editButton)
		})
	}

	// A press inside the dialog, on its text, does not close it.
	app := telatest.MountFunc(t, page, telatest.Theme(kit.Theme()))
	app.Click(editButton)
	app.Click(telatest.ByRole("heading", "Edit profile"))
	app.ExpectCount(profileDlg, 1)
}

func TestDialogFormInside(t *testing.T) {
	// State inside the dialog lives while it is open: it starts afresh the
	// next time.
	app := telatest.MountFunc(t, pageUnder(editDialog(kit.Dialog{})), telatest.Theme(kit.Theme()))
	app.Click(editButton)
	app.Type("Ana")
	app.ExpectValue(telatest.ByRole("textbox", "Name"), "Ana")
	app.Press("Escape")
	app.Click(editButton)
	app.ExpectValue(telatest.ByRole("textbox", "Name"), "")
}

// The close button shows by default (shadcn's showCloseButton); HideClose
// leaves it out; CloseLabel names it.
func TestDialogHideClose(t *testing.T) {
	app := telatest.MountFunc(t, pageUnder(editDialog(kit.Dialog{})), telatest.Theme(kit.Theme()))
	app.Click(editButton)
	app.ExpectCount(closeButton, 1)

	app = telatest.MountFunc(t, pageUnder(editDialog(kit.Dialog{HideClose: true})), telatest.Theme(kit.Theme()))
	app.Click(editButton)
	app.ExpectCount(closeButton, 0)

	app = telatest.MountFunc(t, pageUnder(editDialog(kit.Dialog{CloseLabel: "Cerrar"})), telatest.Theme(kit.Theme()))
	app.Click(editButton)
	app.ExpectCount(closeButton, 0)
	app.ExpectCount(telatest.ByRole("button", "Cerrar"), 1)
}

func TestDialogControlled(t *testing.T) {
	var asked []bool
	closed := false
	page := pageUnder(editDialog(kit.Dialog{Open: &closed, OnOpenChange: func(next bool) { asked = append(asked, next) }}))
	// A controlled dialog that ignores the request stays as it is.
	app := telatest.MountFunc(t, page, telatest.Theme(kit.Theme()))
	app.Click(editButton)
	app.ExpectCount(telatest.ByRole("dialog", ""), 0)
	if len(asked) != 1 || !asked[0] {
		t.Fatalf("OnOpenChange got %v, want [true]", asked)
	}
}

// controlledDialog is a dialog the parent opens and closes with its state.
type controlledDialog struct{ log *[]bool }

func (c controlledDialog) Build() tela.Node {
	open, setOpen := tela.UseState(false)
	return h.Div(h.Class("p-6"),
		kit.Button{Children: h.Text("Open from outside"), OnClick: func() { setOpen.Set(true) }},
		kit.Dialog{Open: &open, OnOpenChange: func(next bool) { *c.log = append(*c.log, next); setOpen.Set(next) },
			Title: h.Text("Controlled"), Description: h.Text("Opened by state.")})
}

func TestDialogControlledByState(t *testing.T) {
	var log []bool
	app := telatest.Mount(t, controlledDialog{log: &log}, telatest.Theme(kit.Theme()))
	app.Click(telatest.ByRole("button", "Open from outside"))
	app.ExpectCount(telatest.ByRole("dialog", "Controlled"), 1)
	if len(log) != 0 {
		t.Errorf("OnOpenChange called for a change the parent made: %v", log)
	}
	app.Press("Escape")
	app.ExpectCount(telatest.ByRole("dialog", ""), 0)
	if len(log) != 1 || log[0] {
		t.Errorf("OnOpenChange got %v, want [false]", log)
	}
	// With no trigger the focus goes back to what had it: the button.
	app.ExpectFocused(telatest.ByRole("button", "Open from outside"))
}

func TestDialogDefaultOpenNotifies(t *testing.T) {
	var log []bool
	app := telatest.MountFunc(t, pageUnder(editDialog(kit.Dialog{DefaultOpen: true, OnOpenChange: func(o bool) { log = append(log, o) }})),
		telatest.Theme(kit.Theme()))
	app.ExpectCount(profileDlg, 1)
	app.Press("Escape")
	app.ExpectCount(telatest.ByRole("dialog", ""), 0)
	app.Click(editButton)
	app.ExpectCount(profileDlg, 1)
	if fmt.Sprint(log) != "[false true]" {
		t.Errorf("OnOpenChange got %v, want [false true]", log)
	}
}

// profileButton is a trigger of the app's own: a component with the one
// method of kit.Trigger.
type profileButton struct {
	Label string
	props tela.Prop
}

func (b profileButton) WithTriggerProps(p tela.Prop) tela.Component { b.props = p; return b }
func (b profileButton) Build() tela.Node {
	return h.Button(h.Class("rounded-full bg-secondary px-3 py-1 text-sm"), b.props, h.Text(b.Label))
}

func TestTriggerSlotRule(t *testing.T) {
	// One rule for every trigger: a kit.Button, a component with
	// WithTriggerProps and a kit.TriggerFunc all open the dialog, and are
	// the element the screen reader finds.
	triggers := map[string]kit.Trigger{
		"kit.Button": kit.Button{Children: h.Text("Open")},
		"own type":   profileButton{Label: "Open"},
		"TriggerFunc": kit.TriggerFunc(func(p tela.Prop) tela.Node {
			return h.Button(p, h.Class("rounded-full bg-secondary px-3 py-1 text-sm"), h.Text("Open"))
		}),
	}
	for name, trig := range triggers {
		t.Run(name, func(t *testing.T) {
			app := telatest.MountFunc(t, func() tela.Node {
				return h.Div(h.Class("p-6"), kit.Dialog{Trigger: trig, Title: h.Text("Hello"), Description: h.Text("World")})
			}, telatest.Theme(kit.Theme()))
			open := telatest.ByRole("button", "Open")
			app.Expect(open, telatest.HasPopup("dialog"), telatest.Expanded(false))
			app.Focus(open)
			app.Press("Enter")
			app.ExpectCount(telatest.ByRole("dialog", "Hello"), 1)
			app.Press("Escape")
			app.ExpectFocused(open)
			app.Click(open)
			app.ExpectCount(telatest.ByRole("dialog", "Hello"), 1)
		})
	}
}

func TestTriggerKeepsItsProps(t *testing.T) {
	// The trigger's own handlers and props still work next to the overlay's.
	clicks := 0
	app := telatest.MountFunc(t, func() tela.Node {
		return h.Div(h.Class("p-6"), kit.Dialog{
			Trigger: kit.Button{Children: h.Text("Open"), OnClick: func() { clicks++ }, Props: h.TestID("trigger"), Class: "w-40"},
			Title:   h.Text("Hello")})
	}, telatest.Theme(kit.Theme()))
	if w := app.Bounds(telatest.ByTestID("trigger")).Width; w != 160 {
		t.Errorf("the trigger's Class was lost: width %v, want 160", w)
	}
	app.Click(telatest.ByTestID("trigger"))
	app.ExpectCount(telatest.ByRole("dialog", "Hello"), 1)
	if clicks != 1 {
		t.Errorf("the trigger's OnClick ran %d times, want 1", clicks)
	}
}

func TestDialogAriaLabelAndExit(t *testing.T) {
	app := telatest.MountFunc(t, pageUnder(kit.Dialog{
		Trigger:   kit.Button{Children: h.Text("Open")},
		AriaLabel: "Quick view", Children: h.Text("Content"),
	}), telatest.Theme(kit.Theme()))
	app.Click(telatest.ByRole("button", "Open"))
	app.ExpectCount(telatest.ByRole("dialog", "Quick view"), 1)
	app.Press("Escape")
	// It leaves with a transition: the panel is gone from the tree at once
	// (it is inert while it exits) and unmounted when the transition ends.
	app.ExpectCount(telatest.ByRole("dialog", ""), 0)
}

func TestDialogStyleProvider(t *testing.T) {
	s := kit.DefaultStyle()
	s.Dialog.Root += " ring-4"
	app := telatest.MountFunc(t, func() tela.Node {
		return h.Fragment(kit.StyleProvider{Style: &s, Children: tela.C(pageUnder(editDialog(kit.Dialog{DefaultOpen: true})))})
	}, telatest.Theme(kit.Theme()))
	app.ExpectCount(profileDlg, 1)
	if n := app.Count(telatest.ByClass("ring-4")); n != 1 {
		t.Errorf("the dialog has the provider's classes %d times, want 1", n)
	}
}

func TestDialogOpenGolden(t *testing.T) {
	page := pageUnder(editDialog(kit.Dialog{}))
	openGolden(t, "dialog", 800, 500, page, func(a *telatest.App) { a.Click(editButton) })
}

func TestDialogIdle(t *testing.T) {
	// No text field in it, so that no caret blinks.
	expectIdle(t, pageUnder(kit.Dialog{DefaultOpen: true, Title: h.Text("Delete"), Description: h.Text("This cannot be undone."),
		Footer: kit.Button{Children: h.Text("Delete")}}), nil)
}

// the entrance and exit run on the frame clock.
func TestDialogEntranceAndExit(t *testing.T) {
	app := telatest.MountFunc(t, pageUnder(editDialog(kit.Dialog{})), telatest.Theme(kit.Theme()), telatest.NoSettle())
	app.Click(editButton)
	app.Advance(20 * time.Millisecond)
	if n := app.Count(profileDlg); n != 1 {
		t.Fatalf("the dialog is not there just after opening (%d)", n)
	}
	app.Advance(time.Second)
	app.Press("Escape")
	app.Advance(50 * time.Millisecond)
	app.Advance(time.Second)
	app.ExpectCount(telatest.ByRole("dialog", ""), 0)
}
