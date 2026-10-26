package withtests

import (
	"testing"

	"github.com/kuronosu/tela/h"
	"github.com/kuronosu/tela/telatest"
)

func TestView(t *testing.T) {
	telatest.Mount(t, h.Div(h.Class("flex flexx"))) // want class `unknown class "flexx"`
}
