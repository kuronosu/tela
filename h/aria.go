package h

import (
	"github.com/kuronosu/tela"
	"github.com/kuronosu/tela/internal/core"
)

// The semantics props: React's role and aria-* attributes, and Img's alt
// text. They describe what an element is to assistive technologies, to
// telatest's ByRole queries and to agents (`tela inspect --aria` prints
// the tree as a Playwright aria snapshot); nothing changes on screen.
// Elements have implicit roles: Button is a button, H1 to H6 headings, P a
// paragraph, Img an image, A with Href a link; Div and Span have none.

// Role sets the ARIA role of an element (WAI-ARIA 1.2), for components
// built from divs: Role("switch"), Role("dialog"), Role("tab"). An unknown
// role panics in dev mode (TELA_DEV=1) with the closest valid one.
//
//	Button(Role("switch"), AriaChecked(on), AriaLabel("Wi-Fi"),
//	    OnClick(func(tela.MouseEvent) { setOn.Set(!on) }), knob)
func Role(role string) tela.Prop { return core.Role(role) }

// AriaLabel sets the accessible name of an element, for elements whose
// content does not name them (an icon button).
//
//	Button(AriaLabel("Close"), OnClick(close), closeIcon)
func AriaLabel(label string) tela.Prop { return core.AriaLabel(label) }

// AriaLabelledBy names an element with the text of another one, attached
// to ref with Ref.
//
//	title := tela.UseElementRef()
//	Div(Role("dialog"), AriaLabelledBy(title),
//	    H2(Ref(title), Text("Delete file?")), ...)
func AriaLabelledBy(ref tela.ElementRef) tela.Prop { return core.AriaLabelledBy(ref) }

// AriaDescription sets the accessible description of an element: a
// longer hint read after its name.
//
//	Button(AriaDescription("Removes the file for everyone"), Text("Delete"))
func AriaDescription(description string) tela.Prop { return core.AriaDescription(description) }

// AriaHidden hides an element and its subtree from the semantics tree
// when true (decorations, duplicated content), without hiding it on
// screen.
//
//	Span(AriaHidden(true), Text("★"))
func AriaHidden(hidden bool) tela.Prop { return core.AriaHidden(hidden) }

// AriaChecked sets the checked state of a checkbox, radio or switch.
//
//	Div(Role("checkbox"), AriaChecked(done), TabIndex(0), Text("Done"))
func AriaChecked(checked bool) tela.Prop { return core.AriaChecked(checked) }

// AriaPressed makes a button a toggle button and sets whether it is
// pressed.
//
//	Button(AriaPressed(bold), OnClick(toggleBold), Text("B"))
func AriaPressed(pressed bool) tela.Prop { return core.AriaPressed(pressed) }

// AriaSelected sets whether a tab, option, row or cell is selected.
//
//	Button(Role("tab"), AriaSelected(active == "general"), Text("General"))
func AriaSelected(selected bool) tela.Prop { return core.AriaSelected(selected) }

// AriaExpanded sets whether what a button or link controls (a menu, a
// disclosure) is expanded.
//
//	Button(AriaExpanded(open), OnClick(toggle), Text("Options"))
func AriaExpanded(expanded bool) tela.Prop { return core.AriaExpanded(expanded) }

// AriaDisabled marks an element and its descendants as disabled for
// assistive technologies without making them inert (Disabled does both).
//
//	Button(AriaDisabled(true), OnClick(explainWhy), Text("Publish"))
func AriaDisabled(disabled bool) tela.Prop { return core.AriaDisabled(disabled) }

// AriaLevel sets the level of an element with Role("heading") (H1 to H6
// have theirs), or of a list item, row or tree item.
//
//	Div(Role("heading"), AriaLevel(2), Text("Billing"))
func AriaLevel(level int) tela.Prop { return core.AriaLevel(level) }

// AriaValueNow sets the current value of a range widget (Role("slider"),
// "progressbar", "spinbutton", "meter").
//
//	Div(Role("progressbar"), AriaValueNow(40), AriaValueMin(0), AriaValueMax(100), bar)
func AriaValueNow(value float64) tela.Prop { return core.AriaValueNow(value) }

// AriaValueMin sets the minimum value of a range widget.
func AriaValueMin(value float64) tela.Prop { return core.AriaValueMin(value) }

// AriaValueMax sets the maximum value of a range widget.
func AriaValueMax(value float64) tela.Prop { return core.AriaValueMax(value) }

// AriaValueText sets the value of a range widget as people read it.
//
//	Div(Role("slider"), AriaValueNow(3), AriaValueText("Medium"), track)
func AriaValueText(text string) tela.Prop { return core.AriaValueText(text) }

// Alt sets the alternative text of an Img: its accessible name. Use
// Alt("") for a decorative image, which the semantics tree leaves out. It
// panics on other elements.
//
//	Img(Src(avatar), Alt("Ada Lovelace"), Class("size-10 rounded-full"))
func Alt(text string) tela.Prop { return core.Alt(text) }
