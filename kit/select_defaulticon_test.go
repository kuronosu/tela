package kit_test

import (
	"bytes"
	"testing"

	"github.com/kuronosu/tela"
	"github.com/kuronosu/tela/h"
	"github.com/kuronosu/tela/kit"
	"github.com/kuronosu/tela/telatest"
)

// iconBox draws a Select in a style, so that a test can compare pictures.
type iconBox struct {
	style *kit.Style
	icon  tela.Icon
}

func (b iconBox) Build() tela.Node {
	return kit.StyleProvider{Style: b.style, Children: h.Div(h.Class("p-4"),
		kit.Select[string]{Class: "w-48", AriaLabel: "Fruit", DefaultValue: new("apple"), Icon: b.icon, Items: fruitItems})}
}

func selectPicture(t *testing.T, style *kit.Style, icon tela.Icon) []byte {
	t.Helper()
	a := telatest.Mount(t, iconBox{style: style, icon: icon}, telatest.Size(240, 80))
	return a.Screenshot().Pix
}

// SelectStyle.DefaultIcon is the glyph at the end of the trigger when the
// Select has no Icon of its own, which wins over it; an empty DefaultIcon is
// the chevron down.
func TestSelectDefaultIcon(t *testing.T) {
	d1, d2 := kit.DefaultStyle(), kit.DefaultStyle()
	plain, styled := &d1, &d2
	styled.Select.DefaultIcon = iconMail

	chevron := selectPicture(t, plain, tela.Icon{})
	fromStyle := selectPicture(t, styled, tela.Icon{})
	if bytes.Equal(chevron, fromStyle) {
		t.Error("Select.DefaultIcon changed nothing: the chevron is still drawn")
	}
	if !bytes.Equal(fromStyle, selectPicture(t, plain, iconMail)) {
		t.Error("the style's DefaultIcon does not draw as the same icon set on the Select")
	}
	if !bytes.Equal(selectPicture(t, styled, iconCheck), selectPicture(t, plain, iconCheck)) {
		t.Error("the Select's own Icon does not win over the style's DefaultIcon")
	}
}
