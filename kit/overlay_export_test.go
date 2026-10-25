package kit

import (
	"github.com/kuronosu/tela"
	"github.com/kuronosu/tela/h"
)

// The overlays of the kit are drawn in a Portal, over the window, so a
// Chromium page cannot hold them in a cell next to the shadcn markup. These
// functions, for the tests of package kit_test only, return the panel of an
// overlay as it looks open, as an ordinary component in the page's flow:
// the same classes and the same element tree as the real overlay, without
// its positioning, scrim, hooks and portal.

// StaticDialog is the panel of d, as an open dialog draws it.
func StaticDialog(d Dialog) tela.Component {
	return tela.C(func() tela.Node {
		s := &UseStyle().Dialog
		p := dialogPanel{
			style: s, title: d.Title, description: d.Description, children: d.Children, footer: d.Footer,
			showClose: !d.HideClose, closeIcon: d.CloseIcon, closeLabel: d.CloseLabel,
			class: d.Class, headerClass: d.HeaderClass, titleClass: d.TitleClass, descriptionClass: d.DescriptionClass,
			footerClass: d.FooterClass, closeClass: d.CloseClass,
		}
		return p.root(d.Props)
	})
}

// StaticAlertDialog is the panel of a, as an open alert dialog draws it.
func StaticAlertDialog(a AlertDialog) tela.Component {
	return tela.C(func() tela.Node {
		s := &UseStyle().AlertDialog
		return alertDialogRoot(s, a, pick("AlertDialog", "Size", s.Sizes, a.Size), a.Size,
			tela.Prop{}, tela.Prop{}, tela.Prop{}, tela.ElementRef{}, func() {})
	})
}

// StaticPopover is the panel of p, as an open popover draws it on side
// "bottom".
func StaticPopover(p Popover) tela.Component {
	return tela.C(func() tela.Node {
		s := &UseStyle().Popover
		return h.Div(h.Class(s.Root, s.Sides["bottom"], p.Class), p.Props, p.Children)
	})
}

// StaticTooltip is the tooltip t, as an open one draws it above its
// trigger: its content and its arrow at the bottom center, where the
// floating arrow sits when the tooltip is centered on its trigger.
func StaticTooltip(t Tooltip) tela.Component {
	return tela.C(func() tela.Node {
		s := &UseStyle().Tooltip
		return h.Div(h.Class(s.Root, "relative", t.Class), t.Props, t.Content,
			h.Div(h.Class("absolute top-full left-1/2 size-0"),
				h.Div(h.Class(s.Arrow, s.ArrowSides["top"], "absolute top-0 left-0 -translate-x-1/2 -translate-y-1/2", t.ArrowClass))))
	})
}
