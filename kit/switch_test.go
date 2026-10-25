package kit_test

import (
	"fmt"
	"strings"
	"testing"

	"github.com/kuronosu/tela"
	"github.com/kuronosu/tela/h"
	"github.com/kuronosu/tela/kit"
	"github.com/kuronosu/tela/telatest"
)

// shadcn/ui's Switch (new-york-v4/ui/switch.tsx at kit.ShadcnCommit),
// verbatim.
const (
	shadcnSwitch      = "peer group/switch inline-flex shrink-0 items-center rounded-full border border-transparent shadow-xs transition-all outline-none focus-visible:border-ring focus-visible:ring-[3px] focus-visible:ring-ring/50 disabled:cursor-not-allowed disabled:opacity-50 data-[size=default]:h-[1.15rem] data-[size=default]:w-8 data-[size=sm]:h-3.5 data-[size=sm]:w-6 data-[state=checked]:bg-primary data-[state=unchecked]:bg-input dark:data-[state=unchecked]:bg-input/80"
	shadcnSwitchThumb = "pointer-events-none block rounded-full bg-background ring-0 transition-transform group-data-[size=default]/switch:size-4 group-data-[size=sm]/switch:size-3 data-[state=checked]:translate-x-[calc(100%-2px)] data-[state=unchecked]:translate-x-0 dark:data-[state=checked]:bg-primary-foreground dark:data-[state=unchecked]:bg-foreground"
)

// switchHTML is the markup of shadcn's Switch.
func switchHTML(probe, id string, on, disabled bool, size kit.Size) string {
	state, dsize := "unchecked", "default"
	if on {
		state = "checked"
	}
	if size == kit.Sm {
		dsize = "sm"
	}
	attrs := radixAttrs("switch", on, disabled, false)
	if id != "" {
		attrs += fmt.Sprintf(" id=%q", id)
	}
	return fmt.Sprintf(`<button data-probe=%q %s data-size=%q data-slot="switch" class="%s"><span data-state=%q data-slot="switch-thumb" class="%s"></span></button>`,
		probe, attrs, dsize, shadcnSwitch, state, shadcnSwitchThumb)
}

func switchCell(name string, s kit.Switch) cell {
	s.Props = h.TestID(name + "-switch")
	return cell{name: name, probes: []string{name + "-switch"}, tela: s,
		html: switchHTML(name+"-switch", "", s.DefaultChecked, s.Disabled, s.Size)}
}

// labelledSwitch is a switch with its label in a horizontal Field.
func labelledSwitch(name, text string, s kit.Switch) cell {
	s.Props = h.TestID(name + "-switch")
	id := "f-" + name
	return cell{name: name, width: 260, probes: []string{name + "-switch"},
		tela: kit.Field{Orientation: kit.Horizontal, Label: text, Children: s},
		html: fmt.Sprintf(`<div role="group" data-slot="field" data-orientation="horizontal" class="%s">%s<label data-slot="field-label" for=%q class="%s">%s</label></div>`,
			cn(shadcnFieldHorizontal), switchHTML(name+"-switch", id, s.DefaultChecked, s.Disabled, s.Size), id, cn(shadcnLabel, shadcnFieldLabel), text)}
}

// switchSheet has the default bounds since ADR-159 put the labels'
// baselines at x2 where Chromium's are (the worst cells were 3.98 at x2
// before it, 1.31 after).
func switchSheet() *sheet {
	return &sheet{name: "switch", slot: 280, cells: []cell{
		switchCell("off", kit.Switch{}),
		switchCell("on", kit.Switch{DefaultChecked: true}),
		switchCell("sm-off", kit.Switch{Size: kit.Sm}),
		switchCell("sm-on", kit.Switch{Size: kit.Sm, DefaultChecked: true}),
		switchCell("disabled", kit.Switch{Disabled: true}),
		switchCell("disabled-on", kit.Switch{Disabled: true, DefaultChecked: true}),
		labelledSwitch("label", "Airplane mode", kit.Switch{}),
		labelledSwitch("label-on", "Notifications", kit.Switch{DefaultChecked: true}),
		labelledSwitch("label-sm", "Compact", kit.Switch{Size: kit.Sm, DefaultChecked: true}),
	}}
}

func TestSwitchGolden(t *testing.T)   { switchSheet().golden(t) }
func TestChromiumSwitch(t *testing.T) { switchSheet().chromium(t) }

func switchRole(name string) telatest.Locator { return telatest.ByRole("switch", name) }

func TestSwitchUncontrolled(t *testing.T) {
	var got []bool
	app := telatest.MountFunc(t, func() tela.Node {
		return h.Div(kit.Switch{AriaLabel: "Wi-Fi", OnCheckedChange: func(b bool) { got = append(got, b) }},
			kit.Switch{AriaLabel: "Bluetooth", DefaultChecked: true})
	})
	app.Expect(switchRole("Wi-Fi"), telatest.Checked(false))
	app.Expect(switchRole("Bluetooth"), telatest.Checked(true))
	app.Click(switchRole("Wi-Fi"))
	app.Expect(switchRole("Wi-Fi"), telatest.Checked(true))
	app.Focus(switchRole("Wi-Fi"))
	app.Press("Space")
	app.Expect(switchRole("Wi-Fi"), telatest.Checked(false))
	if fmt.Sprint(got) != "[true false]" {
		t.Errorf("OnCheckedChange got %v, want [true false]", got)
	}
}

// controlledSwitch keeps its state in the caller.
type controlledSwitch struct{}

func (controlledSwitch) Build() tela.Node {
	on, setOn := tela.UseState(true)
	return h.Div(kit.Switch{AriaLabel: "Sync", Checked: &on, OnCheckedChange: setOn.Set},
		h.Span(h.Class("sr-only"), h.Text(fmt.Sprintf("sync is %t", on))))
}

func TestSwitchControlled(t *testing.T) {
	app := telatest.Mount(t, controlledSwitch{})
	app.ExpectText("sync is true")
	app.Click(switchRole("Sync"))
	app.ExpectText("sync is false")
	app.Expect(switchRole("Sync"), telatest.Checked(false))
}

func TestSwitchDisabled(t *testing.T) {
	app := telatest.MountFunc(t, func() tela.Node {
		return h.Fragment(kit.Switch{AriaLabel: "Locked", Disabled: true})
	})
	app.Expect(switchRole("Locked"), telatest.Disabled(true))
	b := app.Bounds(switchRole("Locked"))
	app.ClickAt(b.X+b.Width/2, b.Y+b.Height/2)
	app.Expect(switchRole("Locked"), telatest.Checked(false))
}

// TestSwitchThumbMoves checks the knob's position in both states: it is at
// the left edge when off, and 14 px (its size, less 2) to the right when on.
func TestSwitchThumbMoves(t *testing.T) {
	app := telatest.MountFunc(t, func() tela.Node {
		return h.Div(h.Class("p-4"), kit.Switch{AriaLabel: "Sync", Props: h.TestID("sw")})
	})
	thumb := telatest.ByClass("translate-x-0")
	off := app.Bounds(thumb)
	sw := app.Bounds(telatest.ByTestID("sw"))
	if d := off.X - (sw.X + 1); d < -0.01 || d > 0.01 {
		t.Errorf("off: the knob is at %g, want the switch's left edge plus its border (%g)", off.X, sw.X+1)
	}
	app.Click(switchRole("Sync"))
	app.Advance(1000000000)
	on := app.Bounds(thumb)
	if d := on.X - off.X - 14; d < -0.01 || d > 0.01 {
		t.Errorf("on: the knob moved %g px, want 14", on.X-off.X)
	}
}

func TestSwitchLabelFieldAndSnapshot(t *testing.T) {
	app := telatest.MountFunc(t, func() tela.Node {
		return h.Div(
			kit.Field{Orientation: kit.Horizontal, Label: "Airplane mode", Children: kit.Switch{}},
			kit.Switch{AriaLabel: "Notify", DefaultChecked: true, Required: true, Invalid: true})
	})
	app.Click(telatest.ByText("Airplane mode"))
	app.Expect(switchRole("Airplane mode"), telatest.Checked(true))
	snap := app.AriaSnapshot()
	for _, want := range []string{`switch "Airplane mode" [checked]`, `switch "Notify" [checked] [invalid]`} {
		if !strings.Contains(snap, want) {
			t.Errorf("snapshot lacks %q:\n%s", want, snap)
		}
	}
}

func TestSwitchUndefinedSize(t *testing.T) {
	_, err := telatest.AriaSnapshot(kit.Switch{Size: "sml"})
	if err == nil || !strings.Contains(err.Error(), `did you mean "sm"?`) {
		t.Errorf("error %v, want the closest size", err)
	}
}

func ExampleSwitch() {
	t := &exampleT{}
	defer t.close()
	wifi := true
	page := func() tela.Node {
		return h.Div(h.Class("p-6"),
			kit.Switch{AriaLabel: "Wi-Fi", DefaultChecked: wifi, OnCheckedChange: func(on bool) { wifi = on }})
	}
	app := telatest.MountFunc(t, page)
	app.Click(telatest.ByRole("switch", "Wi-Fi"))
	fmt.Println("wifi", wifi)
	// Output: wifi false
}
