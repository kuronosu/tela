package kit_test

import (
	"fmt"
	"time"

	"github.com/kuronosu/tela"
	"github.com/kuronosu/tela/h"
	"github.com/kuronosu/tela/kit"
	"github.com/kuronosu/tela/telatest"
)

func ExampleDialog() {
	t := &exampleT{}
	defer t.close()
	page := func() tela.Node {
		return h.Div(h.Class("p-6"), kit.Dialog{
			Trigger:     kit.Button{Variant: kit.Outline, Children: h.Text("Edit profile")},
			Title:       h.Text("Edit profile"),
			Description: h.Text("Make changes to your profile here."),
			Children:    kit.Input{Placeholder: "Name"},
			Footer:      kit.Button{Children: h.Text("Save changes")},
			ShowClose:   true,
		})
	}
	app := telatest.MountFunc(t, page)
	app.Click(telatest.ByRole("button", "Edit profile"))
	fmt.Println("open:", app.Count(telatest.ByRole("dialog", "Edit profile")) == 1)
	app.Press("Escape")
	fmt.Println("open:", app.Count(telatest.ByRole("dialog", "Edit profile")) == 1)
	// Output:
	// open: true
	// open: false
}

func ExampleDialog_controlled() {
	t := &exampleT{}
	defer t.close()
	open := true
	// A controlled dialog shows the state it is given and reports what the
	// user asked for, here an Escape; the app decides whether it closes.
	app := telatest.MountFunc(t, func() tela.Node {
		return h.Fragment(kit.Dialog{
			Open:         &open,
			OnOpenChange: func(next bool) { fmt.Println("asked to open:", next) },
			Title:        h.Text("Welcome"),
			Description:  h.Text("Controlled by the app's own state."),
		})
	})
	app.Press("Escape")
	fmt.Println("still open:", app.Count(telatest.ByRole("dialog", "Welcome")) == 1)
	// Output:
	// asked to open: false
	// still open: true
}

func ExampleAlertDialog() {
	t := &exampleT{}
	defer t.close()
	deleted := false
	page := func() tela.Node {
		return h.Div(h.Class("p-6"), kit.AlertDialog{
			Trigger:     kit.Button{Variant: kit.Outline, Children: h.Text("Delete project")},
			Title:       h.Text("Are you absolutely sure?"),
			Description: h.Text("This action cannot be undone."),
			Action:      h.Text("Delete"),
			Destructive: true,
			OnAction:    func() { deleted = true },
		})
	}
	app := telatest.MountFunc(t, page)
	app.Click(telatest.ByRole("button", "Delete project"))
	app.Click(telatest.ByRole("button", "Delete"))
	fmt.Println("deleted:", deleted)
	// Output: deleted: true
}

func ExamplePopover() {
	t := &exampleT{}
	defer t.close()
	page := func() tela.Node {
		return h.Div(h.Class("p-6"), kit.Popover{
			Trigger:   kit.Button{Variant: kit.Outline, Children: h.Text("Dimensions")},
			Placement: "bottom", Align: "start",
			AriaLabel: "Dimensions",
			Children:  h.P(h.Text("Set the dimensions for the layer.")),
		})
	}
	app := telatest.MountFunc(t, page)
	trigger := telatest.ByRole("button", "Dimensions")
	app.Click(trigger)
	fmt.Println("open:", app.Count(telatest.ByRole("dialog", "Dimensions")) == 1)
	app.Click(trigger) // the trigger toggles it
	fmt.Println("open:", app.Count(telatest.ByRole("dialog", "Dimensions")) == 1)
	// Output:
	// open: true
	// open: false
}

func ExampleTooltip() {
	t := &exampleT{}
	defer t.close()
	page := func() tela.Node {
		return h.Div(h.Class("p-6"), kit.Tooltip{
			Trigger: kit.Button{Variant: kit.Outline, Size: kit.Icon, AriaLabel: "Add", Children: h.Text("+")},
			Content: h.Text("Add to library"),
		})
	}
	app := telatest.MountFunc(t, page, telatest.NoSettle())
	app.Hover(telatest.ByRole("button", "Add"))
	fmt.Println("tooltip:", app.Count(telatest.ByRole("tooltip", "Add to library")) == 1)
	app.MouseMove(300, 300)
	app.Advance(time.Second)
	fmt.Println("tooltip:", app.Count(telatest.ByRole("tooltip", "Add to library")) == 1)
	// Output:
	// tooltip: true
	// tooltip: false
}

func ExampleTriggerFunc() {
	t := &exampleT{}
	defer t.close()
	// Any element can be a trigger: it is given the props and puts them
	// where it wants.
	page := func() tela.Node {
		return h.Div(h.Class("p-6"), kit.Popover{
			Trigger: kit.TriggerFunc(func(p tela.Prop) tela.Node {
				return h.Button(p, h.Class("rounded-full"), h.AriaLabel("Account"), kit.Avatar{Fallback: "AB"})
			}),
			AriaLabel: "Account",
			Children:  h.P(h.Text("Signed in as Ana.")),
		})
	}
	app := telatest.MountFunc(t, page)
	app.Click(telatest.ByRole("button", "Account"))
	fmt.Println("open:", app.Count(telatest.ByRole("dialog", "Account")) == 1)
	// Output: open: true
}
