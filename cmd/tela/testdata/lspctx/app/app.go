// Package app is the main package of the module the tests of tela lsp use to
// check aliases declared in another package: the app declares them, ui uses
// them.
package app

import (
	"github.com/kuronosu/tela"
	"github.com/kuronosu/tela/cmd/tela/testdata/lspctx/theme"
)

// App takes its class aliases from the theme package.
var App = tela.App{ClassAliases: theme.Aliases}
