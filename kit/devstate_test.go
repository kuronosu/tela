//go:build teladev

package kit_test

import (
	"testing"
	"time"

	"github.com/kuronosu/tela"
	"github.com/kuronosu/tela/h"
	"github.com/kuronosu/tela/internal/core"
	"github.com/kuronosu/tela/kit"
	"github.com/kuronosu/tela/telatest"
)

// The state that kit's components keep inside the hooks of package aria
// survives a tela dev reload (ADR-113, ADR-309): an uncontrolled choice, a
// selection, a slider's value, a color, a calendar's date. Each case does
// something, reloads (the state the dev loop saves, given to the next
// mount) and checks that the new mount shows the same thing. These tests
// set the process-wide seed of the next mount (core.SetDevSeed), so they
// do not run in parallel. Run with
//
//	go test -tags teladev -run DevState ./kit/

// devReload captures the state of app as tela dev does before a swap,
// closes it and mounts root again with that state.
func devReload(t *testing.T, app *telatest.App, root tela.Component, opts ...telatest.Option) *telatest.App {
	t.Helper()
	var (
		data []byte
		rep  core.DevStateReport
		done bool
	)
	core.CaptureAppDevState(func(d []byte, r core.DevStateReport, err error) {
		if err != nil {
			t.Errorf("CaptureAppDevState: %v", err)
		}
		data, rep, done = d, r, true
	})
	app.WaitFor("the dev state", func() bool { return done })
	for _, r := range rep.Reset {
		if !r.Silent {
			t.Errorf("a slot resets with a warning: %s #%d of %s: %s", r.Hook, r.Index, r.Component, r.Reason)
		}
	}
	app.Close()
	core.SetDevSeed(data)
	return telatest.Mount(t, root, opts...)
}

func devPage(c tela.Component) tela.Component {
	return tela.C(func() tela.Node { return h.Div(h.Class("p-6"), c) })
}

func TestDevStateKeepsRadioGroupChoice(t *testing.T) {
	root := devPage(kit.RadioGroup[string]{AriaLabel: "Plan", DefaultValue: new("free"),
		Items: []kit.RadioItem[string]{{Value: "free", Label: "Free"}, {Value: "pro", Label: "Pro"}}})
	app := telatest.Mount(t, root)
	app.Click(telatest.ByRole("radio", "Pro"))
	app.Expect(telatest.ByRole("radio", "Pro"), telatest.Checked(true))
	app = devReload(t, app, root)
	app.Expect(telatest.ByRole("radio", "Pro"), telatest.Checked(true))
	app.Expect(telatest.ByRole("radio", "Free"), telatest.Checked(false))
}

func TestDevStateKeepsTabsSelection(t *testing.T) {
	root := devPage(kit.Tabs[string]{AriaLabel: "Sections", DefaultValue: new("one"), Tabs: []kit.Tab[string]{
		{Value: "one", Label: "One", Content: h.Text("First")},
		{Value: "two", Label: "Two", Content: h.Text("Second")},
	}})
	app := telatest.Mount(t, root)
	app.Click(telatest.ByRole("tab", "Two"))
	app.Expect(telatest.ByRole("tab", "Two"), telatest.Selected(true))
	app = devReload(t, app, root)
	app.Expect(telatest.ByRole("tab", "Two"), telatest.Selected(true))
	app.ExpectText("Second")
}

func TestDevStateKeepsToggleGroupSelection(t *testing.T) {
	root := devPage(kit.ToggleGroup[string]{AriaLabel: "Align", DefaultValue: new("left"), Items: []kit.ToggleGroupItem[string]{
		{Value: "left", Label: "Left"}, {Value: "right", Label: "Right"},
	}})
	app := telatest.Mount(t, root)
	app.Click(telatest.ByRole("radio", "Right"))
	app.Expect(telatest.ByRole("radio", "Right"), telatest.Checked(true))
	app = devReload(t, app, root)
	app.Expect(telatest.ByRole("radio", "Right"), telatest.Checked(true))
}

func TestDevStateKeepsSelectChoice(t *testing.T) {
	root := devPage(kit.Select[string]{Items: fruitItems, Placeholder: "Select a fruit", AriaLabel: "Fruit"})
	app := telatest.Mount(t, root)
	app.Click(telatest.ByRole("button", "Select a fruit Fruit"))
	app.Click(telatest.ByRole("option", "Blueberry"))
	app.Expect(telatest.ByRole("button", "Blueberry Fruit"), telatest.Expanded(false))
	app = devReload(t, app, root)
	app.Expect(telatest.ByRole("button", "Blueberry Fruit"), telatest.Expanded(false))
}

func TestDevStateKeepsSliderValue(t *testing.T) {
	root := devPage(kit.Slider{AriaLabel: "Volume", DefaultValue: new(30.0)})
	app := telatest.Mount(t, root)
	app.Focus(telatest.ByRole("slider", "Volume"))
	app.Press("ArrowRight")
	app.Press("ArrowRight")
	app.Expect(telatest.ByRole("slider", "Volume"), telatest.HasValue("32"))
	app = devReload(t, app, root)
	app.Expect(telatest.ByRole("slider", "Volume"), telatest.HasValue("32"))
}

func TestDevStateKeepsColorPickerColor(t *testing.T) {
	root := devPage(kit.ColorPicker{DefaultValue: new(indigo), Swatches: presets})
	app := telatest.Mount(t, root, telatest.Size(480, 560))
	app.Click(telatest.ByRole("button", "#ef4444"))
	app.Expect(hexField, telatest.HasValue("#ef4444"))
	// Black keeps the hue of red, which the color alone does not hold.
	app.Click(telatest.ByRole("button", "#000000"))
	app.Expect(hueThumb, telatest.HasValue("Hue 0°"))
	app = devReload(t, app, root, telatest.Size(480, 560))
	app.Expect(hexField, telatest.HasValue("#000000"))
	app.Expect(hueThumb, telatest.HasValue("Hue 0°"))
}

// The checkbox items of an uncontrolled menu live in kit's own store (a
// UseOnce), kept by its codec.
func TestDevStateKeepsMenuCheckboxes(t *testing.T) {
	root := devPage(kit.DropdownMenu[string]{
		Trigger: kit.Button{Variant: kit.Outline, Children: h.Text("View")},
		Items: []kit.MenuItem[string]{
			{Kind: kit.MenuCheckbox, Key: "bar", Label: "Status bar", DefaultChecked: true},
			{Kind: kit.MenuCheckbox, Key: "grid", Label: "Grid"},
		},
	})
	app := telatest.Mount(t, root, telatest.Theme(kit.Theme()))
	app.Click(telatest.ByRole("button", "View"))
	app.Click(telatest.ByRole("menuitemcheckbox", "Grid"))
	app.Click(telatest.ByRole("button", "View"))
	app.Click(telatest.ByRole("menuitemcheckbox", "Status bar"))
	app = devReload(t, app, root, telatest.Theme(kit.Theme()))
	app.Click(telatest.ByRole("button", "View"))
	app.Expect(telatest.ByRole("menuitemcheckbox", "Grid"), telatest.Checked(true))
	app.Expect(telatest.ByRole("menuitemcheckbox", "Status bar"), telatest.Checked(false))
}

func TestDevStateKeepsCalendarDate(t *testing.T) {
	root := devPage(kit.Calendar{Today: calToday, AriaLabel: "Birthday"})
	opts := []telatest.Option{telatest.Size(800, 460), telatest.Theme(kit.Theme())}
	app := telatest.Mount(t, root, opts...)
	app.Click(telatest.ByRole("button", "Previous"))
	app.Click(calCell(calDate(2026, time.September, 9)))
	app.Expect(calCell(calDate(2026, time.September, 9)), telatest.Selected(true))
	app = devReload(t, app, root, opts...)
	app.ExpectRole("heading", "September 2026")
	app.Expect(calCell(calDate(2026, time.September, 9)), telatest.Selected(true))
}
