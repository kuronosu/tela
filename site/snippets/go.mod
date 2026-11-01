// The Go programs shown on the landing page, one package per snippet. The page
// imports the sources with ?raw, and scripts/render-snippets.sh renders them
// into src/assets/landing/ with tela snapshot. lintdemo holds the deliberate
// typo that the page shows tela lint catching; it is never rendered.
module github.com/kuronosu/tela/site/snippets

go 1.27

require github.com/kuronosu/tela v0.12.6

require (
	github.com/go-text/typesetting v0.3.6-0.20261006155729-e49e2bdf1e4b // indirect
	github.com/godbus/dbus/v5 v5.2.4 // indirect
	golang.org/x/image v0.23.0 // indirect
	golang.org/x/sys v0.46.0 // indirect
	golang.org/x/text v0.43.0 // indirect
)

replace github.com/kuronosu/tela => ../..
