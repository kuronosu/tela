module github.com/kuronosu/tela/looks

go 1.27

require github.com/kuronosu/tela v0.12.6

require (
	github.com/ebitengine/purego v0.11.2 // indirect
	github.com/go-text/typesetting v0.3.6-0.20261006155729-e49e2bdf1e4b // indirect
	github.com/godbus/dbus/v5 v5.2.4 // indirect
	golang.org/x/image v0.23.0 // indirect
	golang.org/x/sys v0.46.0 // indirect
	golang.org/x/text v0.43.0 // indirect
)

// Development inside the repository: the module tests against the tela
// next to it. Dependents ignore this directive and use the version above.
replace github.com/kuronosu/tela => ..
