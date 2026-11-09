package core

import (
	"strings"
	"testing"
)

// A Label that a group of controls is labelled by (aria-labelledby) names
// something: kit.Field's label above a RadioGroup has a For that points at
// no control, and dev mode should not warn about it.
func TestLabelWarningAcceptsLabelledBy(t *testing.T) {
	named := func() Node {
		ref := UseElementRef()
		return div(
			label(For("group"), AttachRef(ref), Text("Plan")),
			div(Role("radiogroup"), AriaLabelledBy(ref), div(Text("a"))))
	}
	var logs []string
	tr := devTree(t, Component(named), &logs)
	tr.Frame()
	if len(logs) != 0 {
		t.Errorf("a label that names a group: logs %q, want none", logs)
	}
	// A label nothing refers to still warns, and so does one whose ref is
	// held by an element that is not labelled by it.
	unnamed := func() Node {
		ref := UseElementRef()
		return div(label(For("group"), AttachRef(ref), Text("Plan")), div(Role("radiogroup")))
	}
	logs = nil
	tr = devTree(t, Component(unnamed), &logs)
	tr.Frame()
	if all := strings.Join(logs, "\n"); !strings.Contains(all, `names no control`) {
		t.Errorf("an unreferenced label: logs %q, want the warning", logs)
	}
}
