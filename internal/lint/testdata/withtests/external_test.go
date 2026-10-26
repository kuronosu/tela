package withtests_test

import (
	"testing"

	"github.com/kuronosu/tela/h"
	"github.com/kuronosu/tela/internal/lint/testdata/withtests"
	"github.com/kuronosu/tela/telatest"
)

func TestExternal(t *testing.T) {
	telatest.Mount(t, h.Div(withtests.View(), h.Role("buton"))) // want role `unknown ARIA role "buton"`
}
