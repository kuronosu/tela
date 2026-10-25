package aria_test

import (
	"testing"

	"github.com/kuronosu/tela"
	"github.com/kuronosu/tela/aria"
	. "github.com/kuronosu/tela/h"
	"github.com/kuronosu/tela/telatest"
)

func hexField() telatest.Locator { return telatest.ByRole("textbox", "Hex") }

func fieldPicker(def tela.Color, f aria.ColorFieldOptions, changes *events) picker {
	f.AriaLabel = "Hex"
	st := aria.ColorStateOptions{DefaultValue: &def}
	if changes != nil {
		st.OnChange = func(c tela.Color) { changes.add(aria.FormatHexColor(c, true)) }
	}
	return picker{state: st, field: f, parts: "af"}
}

func TestColorFieldCommitsOnEnterAndBlur(t *testing.T) {
	var changes events
	p := fieldPicker(tela.Hex("#336699"), aria.ColorFieldOptions{}, &changes)
	app := telatest.MountFunc(t, p.page(), telatest.Size(232, 400))
	app.Expect(hexField(), telatest.HasValue("#336699"))
	app.Focus(hexField())
	app.Press("Control+a")
	app.Type("ff8800")
	// Typing does not change the color until the field commits.
	app.Expect(hexField(), telatest.HasValue("ff8800"))
	app.ExpectText("color #336699ff")
	if got := changes.take(); got != "" {
		t.Errorf("OnChange while typing: %s", got)
	}
	app.Press("Enter")
	app.ExpectText("color #ff8800ff")
	app.Expect(hexField(), telatest.HasValue("#ff8800"))
	if got := changes.take(); got != "#ff8800ff" {
		t.Errorf("OnChange: %s", got)
	}
	// Blur commits too.
	app.Press("Control+a")
	app.Type("#abc")
	app.Press("Tab")
	app.ExpectText("color #aabbccff")
	app.Expect(hexField(), telatest.HasValue("#aabbcc"))
	if got := changes.take(); got != "#aabbccff" {
		t.Errorf("OnChange: %s", got)
	}
}

func TestColorFieldRevertsInvalidInput(t *testing.T) {
	var changes events
	p := fieldPicker(tela.Hex("#336699"), aria.ColorFieldOptions{}, &changes)
	app := telatest.MountFunc(t, p.page(), telatest.Size(232, 400))
	app.Focus(hexField())
	// Characters that can never be part of a code are not typed.
	app.Press("Control+a")
	app.Type("zz12")
	app.Expect(hexField(), telatest.HasValue("12"))
	// An incomplete code reverts when committed.
	app.Press("Enter")
	app.Expect(hexField(), telatest.HasValue("#336699"))
	app.ExpectText("color #336699ff")
	// An empty text reverts too.
	app.Press("Control+a")
	app.Press("Backspace")
	app.Press("Tab")
	app.Expect(hexField(), telatest.HasValue("#336699"))
	// More than eight digits cannot be typed.
	app.Focus(hexField())
	app.Press("Control+a")
	app.Type("123456789")
	app.Expect(hexField(), telatest.HasValue("12345678"))
	if got := changes.take(); got != "" {
		t.Errorf("OnChange for invalid input: %s", got)
	}
}

func TestColorFieldAlpha(t *testing.T) {
	// Without Alpha a code of 6 digits keeps the opacity.
	p := fieldPicker(tela.Hex("#336699").WithAlpha(0.5), aria.ColorFieldOptions{}, nil)
	app := telatest.MountFunc(t, p.page(), telatest.Size(232, 400))
	app.Expect(hexField(), telatest.HasValue("#336699"))
	app.Focus(hexField())
	app.Press("Control+a")
	app.Type("ff0000\n")
	app.ExpectText("color #ff000080")
	app.Press("Control+a")
	app.Type("00ff0020\n") // the opacity is ignored without Alpha
	app.ExpectText("color #00ff0080")

	// With Alpha the text has it and a code of 8 or 4 digits sets it.
	p = fieldPicker(tela.Hex("#336699").WithAlpha(0.5), aria.ColorFieldOptions{Alpha: true}, nil)
	app = telatest.MountFunc(t, p.page(), telatest.Size(232, 400))
	app.Expect(hexField(), telatest.HasValue("#33669980"))
	app.Focus(hexField())
	app.Press("Control+a")
	app.Type("00ff0020\n")
	app.ExpectText("color #00ff0020")
	app.Press("Control+a")
	app.Type("f00\n") // no opacity in the code: it keeps the current one
	app.ExpectText("color #ff000020")
	app.Press("Control+a")
	app.Type("f00f\n")
	app.ExpectText("color #ff0000ff")
}

func TestColorFieldKeys(t *testing.T) {
	p := fieldPicker(tela.Hex("#0000fe"), aria.ColorFieldOptions{}, nil)
	app := telatest.MountFunc(t, p.page(), telatest.Size(232, 400))
	app.Focus(hexField())
	app.Press("ArrowUp")
	app.Expect(hexField(), telatest.HasValue("#0000ff"))
	app.Press("ArrowUp") // a carry into the next component
	app.Expect(hexField(), telatest.HasValue("#000100"))
	app.Press("ArrowDown")
	app.Expect(hexField(), telatest.HasValue("#0000ff"))
	app.Press("PageUp")
	app.Expect(hexField(), telatest.HasValue("#00010f"))
	app.Press("PageDown")
	app.Press("PageDown")
	app.Expect(hexField(), telatest.HasValue("#0000ef"))
	// Limits.
	app.Press("Control+a")
	app.Type("fffffe\n")
	app.Press("PageUp")
	app.Expect(hexField(), telatest.HasValue("#ffffff"))
	app.Press("Control+a")
	app.Type("000001\n")
	app.Press("PageDown")
	app.Expect(hexField(), telatest.HasValue("#000000"))
}

// The field follows the color when something else changes it.
func TestColorFieldFollowsTheColor(t *testing.T) {
	p := fieldPicker(aria.HSB{H: 0, S: 100, B: 100, A: 1}.Color(), aria.ColorFieldOptions{}, nil)
	app := telatest.MountFunc(t, p.page(), telatest.Size(232, 400))
	app.Expect(hexField(), telatest.HasValue("#ff0000"))
	x, y := areaPoint(0, 100*0.8)
	app.ClickAt(x, y)
	app.Expect(hexField(), telatest.HasValue("#cccccc"))
	app.ClickAt(16+0.5*areaLen, hueY) // no hue slider on this page: a click on the layout
	app.Expect(hexField(), telatest.HasValue("#cccccc"))
	// A draft is not overwritten while the user types.
	app.Focus(hexField())
	app.Press("Control+a")
	app.Type("12")
	app.Expect(hexField(), telatest.HasValue("12"))
}

func TestColorFieldControlledRefuses(t *testing.T) {
	v := tela.Hex("#336699")
	p := fieldPicker(v, aria.ColorFieldOptions{}, nil)
	p.state = aria.ColorStateOptions{Value: &v}
	app := telatest.MountFunc(t, p.page(), telatest.Size(232, 400))
	app.Focus(hexField())
	app.Press("Control+a")
	app.Type("ff0000\n")
	// The caller did not store it: the field shows the color again.
	app.Expect(hexField(), telatest.HasValue("#336699"))
	app.ExpectText("color #336699ff")
}

func TestColorFieldDisabledAndReadOnly(t *testing.T) {
	var changes events
	p := fieldPicker(tela.Hex("#336699"), aria.ColorFieldOptions{Disabled: true}, &changes)
	app := telatest.MountFunc(t, p.page(), telatest.Size(232, 400))
	app.Expect(hexField(), telatest.Disabled(true))

	p = fieldPicker(tela.Hex("#336699"), aria.ColorFieldOptions{ReadOnly: true}, &changes)
	app = telatest.MountFunc(t, p.page(), telatest.Size(232, 400))
	app.Focus(hexField())
	app.Type("ff0000\n")
	app.Press("ArrowUp")
	app.Expect(hexField(), telatest.HasValue("#336699"))
	if got := changes.take(); got != "" {
		t.Errorf("OnChange of a read-only field: %s", got)
	}
}

func TestColorFieldLabel(t *testing.T) {
	page := func() tela.Node {
		st := aria.UseColorState(aria.ColorStateOptions{DefaultValue: new(tela.Black)})
		f := aria.UseColorField(aria.ColorFieldOptions{State: st})
		return Div(Class("flex gap-2 p-4"), Span(f.LabelProps, Text("Hex code")), Input(f.InputProps, Class("w-28 border")))
	}
	app := telatest.MountFunc(t, page, telatest.Size(300, 100))
	app.ExpectRole("textbox", "Hex code")
	app.Click(telatest.ByText("Hex code"))
	app.ExpectFocused(telatest.ByRole("textbox", "Hex code"))
}
