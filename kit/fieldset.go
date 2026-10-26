// Portions of this file follow shadcn/ui, field.tsx (apps/v4/registry/new-york-v4/ui) at the commit
// kit.ShadcnCommit (https://github.com/shadcn-ui/ui), Copyright (c) 2023 shadcn,
// licensed under the MIT License.
// See THIRD_PARTY.md.

package kit

import (
	"github.com/kuronosu/tela"
	"github.com/kuronosu/tela/h"
)

// LegendVariant is the look of a [FieldSet]'s legend (shadcn's FieldLegend
// variant): the size of its text. Like [Variant] it is open: an app adds a
// variant by naming it in the recipe's LegendVariants map.
type LegendVariant string

const (
	// LegendVariantLegend is the default: a title, text-base. The zero
	// value is the same variant.
	LegendVariantLegend LegendVariant = "legend"
	// LegendVariantLabel is a caption for a group of controls, text-sm, as
	// a [Field]'s label is.
	LegendVariantLabel LegendVariant = "label"
)

// FieldSetStyle is the recipe of [FieldSet] (shadcn/ui's FieldSet,
// FieldLegend and FieldDescription).
type FieldSetStyle struct {
	// Root is the group: the legend and, under it, the column of parts. A
	// fieldset is a block, and its legend is not one of the column's items,
	// so the legend's margin alone separates the two (as in a browser).
	Root string
	// Content is the column that holds the description and the children
	// (shadcn's fieldset: flex flex-col gap-6).
	Content string
	// ContentWithRadioGroup is added to Content when the children are a
	// [RadioGroup] (shadcn's has-[>[data-slot=radio-group]]:gap-3).
	ContentWithRadioGroup string
	// Legend is the legend's classes. It has w-fit because a browser's
	// legend is as wide as its text.
	Legend string
	// LegendVariants are the sizes of the legend.
	LegendVariants map[LegendVariant]string
	// Description is the help text under the legend.
	Description string
	// DescriptionAfterLegend is added to Description for each legend
	// variant that pulls the description up under it (shadcn's
	// [[data-variant=legend]+&]:-mt-1.5, which only the legend variant has).
	DescriptionAfterLegend map[LegendVariant]string
}

func init() {
	defaultStyle.FieldSet = FieldSetStyle{
		Root:                  "",
		Content:               "flex flex-col gap-6",
		ContentWithRadioGroup: "gap-3",
		Legend:                "mb-3 w-fit font-medium",
		LegendVariants: map[LegendVariant]string{
			"":                  "text-base",
			LegendVariantLegend: "text-base",
			LegendVariantLabel:  "text-sm",
		},
		Description: "text-sm leading-normal font-normal text-muted-foreground",
		DescriptionAfterLegend: map[LegendVariant]string{
			"":                  "-mt-1.5",
			LegendVariantLegend: "-mt-1.5",
		},
	}
}

// legendCtx carries the ref of the enclosing [FieldSet]'s legend to the
// groups below it that have no name of their own, and the zero ElementRef
// where none applies (outside a FieldSet, and inside a [Field], whose label
// names its control).
var legendCtx = tela.CreateContext(tela.ElementRef{})

// useGroupLabel returns the legend that names a group of controls (a
// [RadioGroup], a [ToggleGroup]): the enclosing [FieldSet]'s, unless the
// group has an AriaLabel of its own. The zero ElementRef means no legend.
// It is a hook (it reads the legend's context).
func useGroupLabel(ariaLabel string) tela.ElementRef {
	ref := tela.UseContext(legendCtx)
	if ariaLabel != "" {
		return tela.ElementRef{}
	}
	return ref
}

// radioGroupChild is what makes a child a [RadioGroup], for the gap of a
// [FieldSet] that holds one.
type radioGroupChild interface{ isRadioGroup() }

// FieldSet is a group of controls with a caption, shadcn/ui's FieldSet with
// its FieldLegend and FieldDescription. It is a group (role "group") named
// by the legend and described by the description. A [RadioGroup] or a
// [ToggleGroup] inside it takes its name from the legend, so it needs no
// AriaLabel and no [Label] that points at nothing; a group of its own
// AriaLabel keeps it.
//
//	kit.FieldSet{
//	    Legend:        "Theme",
//	    LegendVariant: kit.LegendVariantLabel,
//	    Description:   "Pick how the app looks.", // optional
//	    Children: kit.RadioGroup[string]{Items: themes, Value: &theme, OnValueChange: setTheme.Set},
//	}
//
// The children are laid out in a column with a gap of 24 px (12 px for a
// RadioGroup given directly as Children, as in shadcn). Give several as
// h.Fragment, such as a few [Field]s:
//
//	kit.FieldSet{Legend: "Address", Description: "Where we send the invoice.",
//	    Children: h.Fragment(
//	        kit.Field{Label: "Street", Children: kit.Input{}},
//	        kit.Field{Label: "City", Children: kit.Input{}})}
//
// shadcn's FieldGroup, the column that stacks fields with a 28 px gap, has no
// component: a FieldSet is already a column, and for any other place
// h.Div(h.Class("flex w-full flex-col gap-7"), ...) is the same.
//
// The legend is a block above the children whose margin (mb-3) separates it
// from them; the description is pulled up under a legend of the default
// variant (-mt-1.5). shadcn's nth-last-2:-mt-1, which pulls a description
// that has exactly one item after it, is not applied to the label variant:
// add -mt-1 to DescriptionClass for it.
type FieldSet struct {
	// Legend is the caption of the group, plain text; empty draws none.
	Legend string
	// LegendVariant is [LegendVariantLegend] (the zero value, a title) or
	// [LegendVariantLabel] (a caption the size of a field's label).
	LegendVariant LegendVariant
	// Description is help text under the legend; empty draws none.
	Description string
	// Children are the controls, laid out in a column.
	Children tela.Component

	// Class is added after the recipe's Root classes. The other Class
	// fields do the same for their part; ContentClass is for the column of
	// the description and the children (set its gap here).
	Class            string
	LegendClass      string
	DescriptionClass string
	ContentClass     string
	// Props are more props for the root element.
	Props tela.Child
}

// Build returns the group element: the legend, then the description and the
// children in a column.
func (f FieldSet) Build() tela.Node {
	s := &UseStyle().FieldSet
	legendRef, descRef := tela.UseElementRef(), tela.UseElementRef()

	root := []tela.Child{cls(s.Root, f.Class), h.Role("group")}
	var legend, desc, content tela.Child
	var name tela.ElementRef
	if f.Legend != "" {
		name = legendRef
		root = append(root, h.AriaLabelledBy(legendRef))
		legend = h.Div(cls(s.Legend, pick("FieldSet", "LegendVariant", s.LegendVariants, f.LegendVariant), f.LegendClass),
			h.Ref(legendRef), h.Text(f.Legend))
	}
	if f.Description != "" {
		root = append(root, h.AriaDescribedBy(descRef))
		desc = h.P(cls(s.Description, when(f.Legend != "", s.DescriptionAfterLegend[f.LegendVariant]), f.DescriptionClass),
			h.Ref(descRef), h.Text(f.Description))
	}
	if desc != nil || f.Children != nil {
		_, radios := f.Children.(radioGroupChild)
		content = h.Div(cls(s.Content, when(radios, s.ContentWithRadioGroup), f.ContentClass), desc, f.Children)
	}
	root = append(root, f.Props, legendCtx.Provider(name, legend, content))
	return h.Div(root...)
}
