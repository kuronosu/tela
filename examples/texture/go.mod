module github.com/kuronosu/tela/examples/texture

go 1.27

require (
	github.com/ebitengine/purego v0.11.2
	github.com/kuronosu/tela v0.14.10
	golang.org/x/sys v0.49.0
)

require (
	github.com/go-text/typesetting v0.3.6-0.20261006155729-e49e2bdf1e4b // indirect
	github.com/godbus/dbus/v5 v5.2.4 // indirect
	github.com/yuin/goldmark v1.8.7 // indirect
	golang.org/x/image v0.23.0 // indirect
	golang.org/x/text v0.43.0 // indirect
)

// Development inside the repository: the example builds against the tela
// next to it. It is a module of its own because it is an application with a
// graphics API of its own: it calls Vulkan through purego, which links a
// Linux binary to the C library (ADR-281), in every build of it.
replace github.com/kuronosu/tela => ../..
