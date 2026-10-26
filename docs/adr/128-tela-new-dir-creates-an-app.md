# ADR-128: `tela new <dir>` creates an app

- **Date:** 2026-10-25
- **Status:** Accepted

## Decision

`tela new <dir>` creates an app (the owner's request). **Templates** are plain files in `cmd/tela/templates/app`, embedded with `go:embed` and rendered with `text/template` (a `.tmpl` suffix keeps the go command from reading `go.mod` and `.go` files there). **What it writes:** `go.mod` (module path from `-module` or the directory name, `go` directive equal to Tela's own, checked by a test), `main.go` (the Counter of Getting Started, a public `Counter()` wrapper, `var App` so `tela dev`, `snapshot`, `inspect` and the test share one window description), `main_test.go` with `telatest.MountApp`, a README and a `.gitignore`. **Dependencies:** unless `-skip-deps`, it runs `go get github.com/kuronosu/tela@<v>` and `go mod tidy` with `CGO_ENABLED=0`; `<v>` is the binary's own version when it is a release or pseudo-version, `latest` for a build from a checkout, so `go install …@v0.9.1` yields apps on v0.9.1. If that fails the files stay, the exit code is 1 and the message gives the two commands to run. **Safety:** it refuses a directory that is not empty, writes nothing before the checks pass, and has no network use of its own. **Not done:** `tela init` for an existing module, other templates (`tela new --template`), and `tela add` stay deferred.
