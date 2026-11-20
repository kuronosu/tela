module github.com/kuronosu/tela/examples/notes

go 1.27

require github.com/kuronosu/tela v0.14.7

require (
	github.com/ebitengine/purego v0.11.2 // indirect
	github.com/go-text/typesetting v0.3.6-0.20261006155729-e49e2bdf1e4b // indirect
	github.com/godbus/dbus/v5 v5.2.4 // indirect
	github.com/yuin/goldmark v1.8.7 // indirect
	golang.org/x/image v0.23.0 // indirect
	golang.org/x/sys v0.49.0 // indirect
	golang.org/x/text v0.43.0 // indirect
)

// Development inside the repository: the example builds against the tela
// next to it. It is a module of its own (ADR-298) because it is the one
// program that depends on modernc.org/sqlite, which the core module must
// not.
replace github.com/kuronosu/tela => ../..
