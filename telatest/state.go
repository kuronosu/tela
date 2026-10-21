package telatest

import (
	"fmt"
	"strconv"
	"strings"

	"github.com/kuronosu/tela/internal/semantics"
)

// State is one condition on an element that [App.Expect] checks: a
// toggle such as [Checked], a flag such as [Disabled], or its [HasName]
// and [HasValue]. They read the element's semantics node, so they test
// what assistive technologies are told, without matching strings of the
// aria snapshot.
type State struct {
	// want is the expectation as the failure message words it ("checked=true").
	want string
	// check reports whether n satisfies it and, when it does not, what n
	// has instead.
	check func(n *semantics.Node) (ok bool, got string)
}

// toggleState builds the State of an ARIA toggle (checked, pressed,
// selected, expanded). An element without the state, or with the mixed
// one, never satisfies it: a test of an unset state is a mistake worth
// reporting.
func toggleState(name, prop string, want bool, get func(n *semantics.Node) semantics.Toggle) State {
	return State{
		want: name + "=" + strconv.FormatBool(want),
		check: func(n *semantics.Node) (bool, string) {
			switch got := get(n); got {
			case semantics.ToggleUnset:
				return false, name + " is not set on it (h." + prop + " sets it)"
			case semantics.Of(want):
				return true, ""
			default:
				return false, name + "=" + got.String()
			}
		},
	}
}

// Checked expects the element to have aria-checked equal to checked
// (a checkbox, radio, switch or option; h.AriaChecked sets it). An element
// without the state, or with the mixed one, does not satisfy it.
func Checked(checked bool) State {
	return toggleState("checked", "AriaChecked", checked, func(n *semantics.Node) semantics.Toggle { return n.Checked })
}

// Pressed expects the element to have aria-pressed equal to pressed (a
// toggle button; h.AriaPressed sets it).
func Pressed(pressed bool) State {
	return toggleState("pressed", "AriaPressed", pressed, func(n *semantics.Node) semantics.Toggle { return n.Pressed })
}

// Selected expects the element to have aria-selected equal to selected (a
// tab, option, row or cell; h.AriaSelected sets it).
func Selected(selected bool) State {
	return toggleState("selected", "AriaSelected", selected, func(n *semantics.Node) semantics.Toggle { return n.Selected })
}

// Expanded expects the element to have aria-expanded equal to expanded
// (what a button, link or combobox controls; h.AriaExpanded sets it).
func Expanded(expanded bool) State {
	return toggleState("expanded", "AriaExpanded", expanded, func(n *semantics.Node) semantics.Toggle { return n.Expanded })
}

// flagState builds the State of a boolean flag of the node.
func flagState(name string, flag semantics.State, want bool) State {
	return State{
		want: name + "=" + strconv.FormatBool(want),
		check: func(n *semantics.Node) (bool, string) {
			if n.Has(flag) == want {
				return true, ""
			}
			return false, name + "=" + strconv.FormatBool(!want)
		},
	}
}

// Disabled expects the element to be disabled (true) or enabled (false):
// disabled by h.Disabled on it or an ancestor, or by h.AriaDisabled.
func Disabled(disabled bool) State { return flagState("disabled", semantics.StateDisabled, disabled) }

// Focused expects the element to have the keyboard focus (true) or not
// (false).
func Focused(focused bool) State { return flagState("focused", semantics.StateFocused, focused) }

// HasName expects the element's accessible name to be exactly name (white
// space normalized, case sensitive): what [ByRole] matches by.
func HasName(name string) State {
	want := normalize(name)
	return State{
		want: "name=" + strconv.Quote(want),
		check: func(n *semantics.Node) (bool, string) {
			return n.Name == want, "name=" + strconv.Quote(n.Name)
		},
	}
}

// HasValue expects the element's value to be exactly value: the text of a
// text field, or the value text of a range widget (the h.AriaValueText or
// h.AriaValueNow of a slider or progress bar, as in the aria snapshot).
func HasValue(value string) State {
	return State{
		want: "value=" + strconv.Quote(value),
		check: func(n *semantics.Node) (bool, string) {
			return n.Value == value, "value=" + strconv.Quote(n.Value)
		},
	}
}

// Expect waits until the element l locates satisfies every state, like
// Playwright's toBeChecked, toBeDisabled and toBeExpanded: it settles the
// app and checks, and while a state does not hold lets a frame of app time
// pass and checks again, until it holds or the [Timeout] expires. The
// locator must match exactly one element (strictness). The failure
// message says which states differ, shows the element's actual states and
// its line of the aria snapshot.
//
//	app.Expect(telatest.ByRole("switch", "Wi-Fi"), telatest.Checked(true), telatest.Disabled(false))
//	app.Expect(telatest.ByRole("button", "Menu"), telatest.Expanded(true))
//	app.Expect(telatest.ByRole("textbox", "Email"), telatest.HasValue("ada@example.com"))
func (a *App) Expect(l Locator, states ...State) {
	a.tb.Helper()
	a.mustBeOpen()
	if p := l.problem(); p != "" {
		a.fatalf("%s", p)
	}
	if len(states) == 0 {
		a.fatalf("Expect(%s) without states checks nothing; pass states such as telatest.Checked(true), Disabled(false) or HasValue(\"...\")", l)
	}
	a.waitFor(func() (bool, string) {
		a.tb.Helper()
		nodes := a.find(l)
		switch len(nodes) {
		case 0:
			return false, a.missMessage(l)
		case 1:
		default:
			a.fatalf("%s", a.strictMessage(l, nodes))
		}
		n := nodes[0]
		var wants, gots []string
		for _, s := range states {
			if ok, got := s.check(n); !ok {
				wants = append(wants, s.want)
				gots = append(gots, got)
			}
		}
		if len(wants) == 0 {
			return true, ""
		}
		var b strings.Builder
		fmt.Fprintf(&b, "expected %s to have %s, but it has %s", a.describeMatch(n), strings.Join(wants, ", "), strings.Join(gots, ", "))
		fmt.Fprintf(&b, "\nits states: %s", describeStates(n))
		if line := ariaLine(n); line != "" {
			fmt.Fprintf(&b, "\nits aria line: %s", line)
		}
		return false, b.String()
	})
}

// describeStates lists the states of n for a failure message.
func describeStates(n *semantics.Node) string {
	var s []string
	for _, t := range []struct {
		name string
		v    semantics.Toggle
	}{{"checked", n.Checked}, {"pressed", n.Pressed}, {"selected", n.Selected}, {"expanded", n.Expanded}} {
		if t.v != semantics.ToggleUnset {
			s = append(s, t.name+"="+t.v.String())
		}
	}
	s = append(s, "disabled="+strconv.FormatBool(n.Has(semantics.StateDisabled)),
		"focused="+strconv.FormatBool(n.Has(semantics.StateFocused)))
	if n.Has(semantics.StateReadOnly) {
		s = append(s, "readonly=true")
	}
	if n.Has(semantics.StateRequired) {
		s = append(s, "required=true")
	}
	if n.Has(semantics.StateBusy) {
		s = append(s, "busy=true")
	}
	if n.Name != "" {
		s = append(s, "name="+strconv.Quote(n.Name))
	}
	if n.Value != "" {
		s = append(s, "value="+strconv.Quote(n.Value))
	}
	return strings.Join(s, ", ")
}

// ariaLine returns the line of the aria snapshot that n makes (its role,
// name and states), or "" when it has none of its own.
func ariaLine(n *semantics.Node) string {
	if n.Flattened() {
		return ""
	}
	line, _, _ := strings.Cut(semantics.Snapshot(n), "\n")
	return line
}
