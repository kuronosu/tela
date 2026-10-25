package kit_test

import (
	"fmt"
	"html"
	"testing"

	"github.com/kuronosu/tela/h"
	"github.com/kuronosu/tela/kit"
)

// shadcn/ui's Textarea (new-york-v4/ui/textarea.tsx at kit.ShadcnCommit),
// verbatim.
const shadcnTextarea = "flex field-sizing-content min-h-16 w-full rounded-md border border-input bg-transparent px-3 py-2 text-base shadow-xs transition-[color,box-shadow] outline-none placeholder:text-muted-foreground focus-visible:border-ring focus-visible:ring-[3px] focus-visible:ring-ring/50 disabled:cursor-not-allowed disabled:opacity-50 aria-invalid:border-destructive aria-invalid:ring-destructive/20 md:text-sm dark:bg-input/30 dark:aria-invalid:ring-destructive/40"

func textareaCell(name string, t kit.Textarea) cell {
	t.Props = h.TestID(name + "-textarea")
	attrs := ""
	if t.Placeholder != "" {
		attrs += fmt.Sprintf(` placeholder=%q`, html.EscapeString(t.Placeholder))
	}
	if t.Invalid {
		attrs += ` aria-invalid="true"`
	}
	if t.Disabled {
		attrs += " disabled"
	}
	return cell{
		name: name, width: 320, probes: []string{name + "-textarea"}, tela: t,
		html: fmt.Sprintf(`<textarea data-probe="%s-textarea"%s class="%s">%s</textarea>`, name, attrs, cn(shadcnTextarea, t.Class), html.EscapeString(t.DefaultValue)),
	}
}

func textareaSheet() *sheet {
	return &sheet{name: "textarea", cells: []cell{
		textareaCell("placeholder", kit.Textarea{Placeholder: "Type your message here."}),
		textareaCell("value", kit.Textarea{DefaultValue: "One line of text."}),
		textareaCell("lines", kit.Textarea{DefaultValue: "The first line.\nThe second line.\nThe third line, and the field grows."}),
		textareaCell("invalid", kit.Textarea{Invalid: true, DefaultValue: "Too short"}),
		textareaCell("disabled", kit.Textarea{Disabled: true, Placeholder: "Disabled"}),
		textareaCell("class", kit.Textarea{Placeholder: "A taller one", Class: "min-h-32"}),
	}}
}

func TestTextareaGolden(t *testing.T)   { textareaSheet().golden(t) }
func TestChromiumTextarea(t *testing.T) { textareaSheet().chromium(t) }
