package kit_test

import (
	"fmt"
	"testing"

	"github.com/kuronosu/tela"
	"github.com/kuronosu/tela/h"
	"github.com/kuronosu/tela/kit"
)

// shadcn/ui's Label (new-york-v4/ui/label.tsx at kit.ShadcnCommit),
// verbatim.
const shadcnLabel = "flex items-center gap-2 text-sm leading-none font-medium select-none group-data-[disabled=true]:pointer-events-none group-data-[disabled=true]:opacity-50 peer-disabled:cursor-not-allowed peer-disabled:opacity-50"

// labelCell is a label and the input it labels, one above the other (or the
// input first, which is how a peer is written). The label is the probe.
func labelCell(name string, l kit.Label, in kit.Input, inputFirst bool) cell {
	id := "l-" + name
	l.Props = h.TestID(name + "-label")
	l.Children = h.Text("Email address")
	in.Placeholder = "you@example.com"

	label := fmt.Sprintf(`<label data-probe="%s-label" for="%s" class="%s">Email address</label>`, name, id, cn(shadcnLabel, l.Class))
	attrs := ` placeholder="you@example.com"`
	if in.Disabled {
		attrs += " disabled"
	}
	input := fmt.Sprintf(`<input id="%s"%s class="%s">`, id, attrs, cn(shadcnInput, in.Class))

	html := label + input
	if inputFirst {
		html = input + label
	}
	// shadcn disables a label through its group: data-disabled="true" on an
	// ancestor with the class group.
	wrap := `<div class="flex w-full flex-col gap-3">%s</div>`
	if l.Disabled {
		wrap = `<div class="group flex w-full flex-col gap-3" data-disabled="true">%s</div>`
	}
	return cell{
		name: name, width: 240, probes: []string{name + "-label"},
		tela: labelPair{label: l, input: in, inputFirst: inputFirst},
		html: fmt.Sprintf(wrap, html),
	}
}

// labelPair is a label and its input, joined by an id the page makes: every
// panel of a sheet has its own copy of the cell, and an id is unique in a
// window.
type labelPair struct {
	label      kit.Label
	input      kit.Input
	inputFirst bool
}

func (p labelPair) Build() tela.Node {
	id := tela.UseID()
	p.label.For, p.input.ID = id, id
	first, second := tela.Child(p.label), tela.Child(p.input)
	if p.inputFirst {
		first, second = second, first
	}
	return h.Div(h.Class("flex w-full flex-col gap-3"), first, second)
}

func labelSheet() *sheet {
	return &sheet{name: "label", cells: []cell{
		labelCell("plain", kit.Label{}, kit.Input{}, false),
		labelCell("disabled", kit.Label{Disabled: true}, kit.Input{Disabled: true}, false),
		// peer-disabled: dims a label that follows a disabled control with
		// the class "peer".
		labelCell("peer", kit.Label{}, kit.Input{Class: "peer", Disabled: true}, true),
		labelCell("class", kit.Label{Class: "text-base font-semibold"}, kit.Input{}, false),
	}}
}

func TestLabelGolden(t *testing.T)   { labelSheet().golden(t) }
func TestChromiumLabel(t *testing.T) { labelSheet().chromium(t) }
