module example.com/lspapp

go 1.27

require github.com/kuronosu/tela v0.0.0

require (
	github.com/ebitengine/purego v0.11.2 // indirect
	github.com/go-text/typesetting v0.3.6-0.20261006155729-e49e2bdf1e4b // indirect
	github.com/godbus/dbus/v5 v5.2.4 // indirect
	golang.org/x/image v0.23.0 // indirect
	golang.org/x/sys v0.46.0 // indirect
	golang.org/x/text v0.43.0 // indirect
)

// The fixture is an app of its own, as an editor's folder is, so that its
// lint context is this module, not Tela's repository: the tests of tela
// lsp read only the fixture and the export data of Tela's packages, and an
// edit elsewhere in the repository leaves their results cached.
replace github.com/kuronosu/tela => ../../../..
