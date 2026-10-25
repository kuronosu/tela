package kit_test

import (
	"fmt"
	"html"
	"testing"

	"github.com/kuronosu/tela/h"
	"github.com/kuronosu/tela/kit"
)

// shadcn/ui's Input (new-york-v4/ui/input.tsx at kit.ShadcnCommit),
// verbatim.
const shadcnInput = "h-9 w-full min-w-0 rounded-md border border-input bg-transparent px-3 py-1 text-base shadow-xs transition-[color,box-shadow] outline-none selection:bg-primary selection:text-primary-foreground file:inline-flex file:h-7 file:border-0 file:bg-transparent file:text-sm file:font-medium file:text-foreground placeholder:text-muted-foreground disabled:pointer-events-none disabled:cursor-not-allowed disabled:opacity-50 md:text-sm dark:bg-input/30 focus-visible:border-ring focus-visible:ring-[3px] focus-visible:ring-ring/50 aria-invalid:border-destructive aria-invalid:ring-destructive/20 dark:aria-invalid:ring-destructive/40"

func inputCell(name string, i kit.Input) cell {
	i.Props = h.TestID(name + "-input")
	attrs := ""
	if i.Type != "" {
		attrs += fmt.Sprintf(` type=%q`, i.Type)
	}
	if i.Placeholder != "" {
		attrs += fmt.Sprintf(` placeholder=%q`, html.EscapeString(i.Placeholder))
	}
	if i.DefaultValue != "" {
		attrs += fmt.Sprintf(` value=%q`, html.EscapeString(i.DefaultValue))
	}
	if i.Disabled {
		attrs += " disabled"
	}
	if i.Invalid {
		attrs += ` aria-invalid="true"`
	}
	if i.ReadOnly {
		attrs += " readonly"
	}
	return cell{
		name: name, width: 280, probes: []string{name + "-input"}, tela: i,
		html: fmt.Sprintf(`<input data-probe="%s-input"%s class="%s">`, name, attrs, cn(shadcnInput, i.Class)),
	}
}

// inputSheet has the default bounds since ADR-159 centered an Input's
// line as Chromium's inner editor does, with line-height: normal under
// h-9 (the worst cells were 4.60 at x1 before it, 1.83 after).
func inputSheet() *sheet {
	return &sheet{name: "input", cells: []cell{
		inputCell("placeholder", kit.Input{Placeholder: "Email", Type: "email"}),
		inputCell("value", kit.Input{DefaultValue: "ana@example.com", Type: "email"}),
		inputCell("disabled", kit.Input{Disabled: true, Placeholder: "Disabled"}),
		inputCell("disabled-value", kit.Input{Disabled: true, DefaultValue: "Locked value"}),
		inputCell("readonly", kit.Input{ReadOnly: true, DefaultValue: "Read only"}),
		inputCell("invalid", kit.Input{Invalid: true, DefaultValue: "not an email", Type: "email"}),
		inputCell("class", kit.Input{Placeholder: "Search", Class: "h-10 rounded-full px-4"}),
	}}
}

func TestInputGolden(t *testing.T)   { inputSheet().golden(t) }
func TestChromiumInput(t *testing.T) { inputSheet().chromium(t) }
