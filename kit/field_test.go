package kit_test

import (
	"fmt"
	"testing"

	"github.com/kuronosu/tela/h"
	"github.com/kuronosu/tela/kit"
)

// shadcn/ui's Field parts (new-york-v4/ui/field.tsx at kit.ShadcnCommit),
// verbatim.
const (
	shadcnFieldVertical   = "group/field flex w-full gap-3 data-[invalid=true]:text-destructive flex-col [&>*]:w-full [&>.sr-only]:w-auto"
	shadcnFieldHorizontal = "group/field flex w-full gap-3 data-[invalid=true]:text-destructive flex-row items-center [&>[data-slot=field-label]]:flex-auto has-[>[data-slot=field-content]]:items-start has-[>[data-slot=field-content]]:[&>[role=checkbox],[role=radio]]:mt-px"
	shadcnFieldContent    = "group/field-content flex flex-1 flex-col gap-1.5 leading-snug"
	shadcnFieldLabel      = "group/field-label peer/field-label flex w-fit gap-2 leading-snug group-data-[disabled=true]/field:opacity-50 has-[>[data-slot=field]]:w-full has-[>[data-slot=field]]:flex-col has-[>[data-slot=field]]:rounded-md has-[>[data-slot=field]]:border [&>*]:data-[slot=field]:p-4 has-data-[state=checked]:border-primary has-data-[state=checked]:bg-primary/5 dark:has-data-[state=checked]:bg-primary/10"
	shadcnFieldDesc       = "text-sm leading-normal font-normal text-muted-foreground group-has-[[data-orientation=horizontal]]/field:text-balance last:mt-0 nth-last-2:-mt-1 [[data-variant=legend]+&]:-mt-1.5 [&>a]:underline [&>a]:underline-offset-4 [&>a:hover]:text-primary"
	shadcnFieldError      = "text-sm font-normal text-destructive"
)

// fieldCell is a field with an input: the Tela side is kit.Field and the
// markup is shadcn's Field, FieldLabel, Input, FieldDescription and
// FieldError.
func fieldCell(name string, f kit.Field, in kit.Input) cell {
	f.Props = h.TestID(name + "-field")
	in.Props = h.TestID(name + "-input")
	f.Children = in
	horizontal := f.Orientation == kit.Horizontal
	id := "f-" + name
	root, orientation := shadcnFieldVertical, "vertical"
	if horizontal {
		root, orientation = shadcnFieldHorizontal, "horizontal"
	}
	attrs, rootAttrs := "", ""
	if f.Error != "" || f.Invalid {
		attrs, rootAttrs = ` aria-invalid="true"`, ` data-invalid="true"`
	}
	if in.Placeholder != "" {
		attrs += fmt.Sprintf(` placeholder=%q`, in.Placeholder)
	}
	if in.DefaultValue != "" {
		attrs += fmt.Sprintf(` value=%q`, in.DefaultValue)
	}
	control := fmt.Sprintf(`<input data-probe="%s-input" id="%s"%s class="%s">`, name, id, attrs, cn(shadcnInput, in.Class))
	label := ""
	if f.Label != "" {
		label = fmt.Sprintf(`<label data-slot="field-label" for="%s" class="%s">%s</label>`, id, cn(shadcnLabel, shadcnFieldLabel), f.Label)
	}
	desc, errHTML := "", ""
	if f.Description != "" {
		desc = fmt.Sprintf(`<p data-slot="field-description" class="%s">%s</p>`, cn(shadcnFieldDesc), f.Description)
	}
	if f.Error != "" {
		errHTML = fmt.Sprintf(`<div role="alert" data-slot="field-error" class="%s">%s</div>`, shadcnFieldError, f.Error)
	}
	var body string
	switch {
	case !horizontal:
		body = label + control + desc + errHTML
	case desc != "" || errHTML != "":
		body = control + fmt.Sprintf(`<div data-slot="field-content" class="%s">%s</div>`, shadcnFieldContent, label+desc+errHTML)
	default:
		body = control + label
	}
	return cell{
		name: name, width: 320, probes: []string{name + "-field", name + "-input"}, tela: f,
		html: fmt.Sprintf(`<div role="group" data-slot="field"%s data-orientation="%s" data-probe="%s-field" class="%s">%s</div>`,
			rootAttrs, orientation, name, cn(root), body),
	}
}

func fieldSheet() *sheet {
	return &sheet{name: "field", cells: []cell{
		fieldCell("label-only", kit.Field{Label: "Email"}, kit.Input{Placeholder: "m@example.com", Type: "email"}),
		fieldCell("description", kit.Field{Label: "Username", Description: "Choose a unique username for your account."},
			kit.Input{Placeholder: "ana"}),
		fieldCell("error", kit.Field{Label: "Password", Error: "Password must be at least 8 characters."},
			kit.Input{DefaultValue: "short"}),
		fieldCell("description-error", kit.Field{Label: "Name", Description: "As it appears on your card.", Error: "Name is required."},
			kit.Input{Placeholder: "Full name"}),
		fieldCell("horizontal", kit.Field{Orientation: kit.Horizontal, Label: "Search"}, kit.Input{Placeholder: "Find…", Class: "w-40"}),
		fieldCell("horizontal-description", kit.Field{Orientation: kit.Horizontal, Label: "Nickname", Description: "Shown in comments."},
			kit.Input{Placeholder: "Nick", Class: "w-40"}),
	}}
}

func TestFieldGolden(t *testing.T)   { fieldSheet().golden(t) }
func TestChromiumField(t *testing.T) { fieldSheet().chromium(t) }
