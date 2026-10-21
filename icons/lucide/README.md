# Lucide icons for Tela

[Lucide](https://lucide.dev) (1,902 distinct drawings, 2,171 names counting the
deprecated aliases) as `tela.Icon` variables, generated with `tela icons gen`.

```go
import (
	"github.com/kuronosu/tela/icons/lucide"
	. "github.com/kuronosu/tela/h"
)

Button(Class("inline-flex items-center gap-2"),
	Icon(lucide.Check, Class("size-4 text-muted-foreground")),
	Text("Save"))
```

The icons are 24x24 drawings with a 2 px round stroke in `currentColor`: they
take the text color of the element, and `size-*`, `stroke-[1.5]`, `fill-*` and
the other SVG classes of `h.Icon` apply. Names are Lucide's file names in
PascalCase (`arrow-up-right.svg` is `lucide.ArrowUpRight`, `volume-2` is
`Volume2`; digits that Lucide separates stay apart: `arrow-down-0-1` is
`ArrowDown0_1`, `arrow-down-01` is `ArrowDown01`). Lucide keeps its old names
as aliases, so `lucide.AlertCircle` and `lucide.CircleAlert` both exist; the
268 old names are marked `// Deprecated: use CircleAlert.` so that gopls and
staticcheck flag them and say which icon to use.

**There is one variable per icon and no table of icons by name.** The linker
drops the variables a program does not use: a program that shows 50 icons grows
by about 30 KiB, one that shows 1 by well under 1 KiB. A table would link
all of them (about +0.9 MB). To pick an icon by a name that is only known at
run time, build your own map from the icons you use.

## Versions

The module's version follows Lucide's: the tag `icons/lucide/vX.Y.Z` holds the
icons of Lucide `X.Y.Z` (`lucide.Version` says which). The module requires the
`github.com/kuronosu/tela` release with `tela.Icon` (v0.8.0 or later).

A regeneration without a new Lucide release (a change of the generator or of
the comments, with the same icons) is published as a pre-release,
`icons/lucide/vX.Y.Z-tela.N` with `N` from 1: `v1.61.0-tela.1` holds the
icons of Lucide 1.61.0 as `tela icons gen` made them after `v1.61.0`. Go orders
a pre-release before its release, and `@latest` ignores it while a release
exists, so ask for one by its version.

Generated from `lucide-static` 1.61.0 (npm integrity
`sha512-yLAiQB3MWUJEstsEDAwenSA3m7vr1tdLvgnC16Q/XMxRRma0DWwMs8M2bWlvgShrIs1m2mWMJ+0AiahqQO4GrA==`).

## License

Lucide is under the ISC license, and the icons it took from Feather under the
MIT license; both notices are in [LICENSE](LICENSE) and at the top of
`icons_gen.go`. The Go code of this module is under Tela's license.

## Development

The module has its own `go.mod`, with `replace github.com/kuronosu/tela =>
../..` so it builds against the Tela next to it; the root module does not
import it and `go test ./...` at the root does not run its tests. Test it from
this directory:

```bash
cd icons/lucide
CGO_ENABLED=0 go vet ./...
CGO_ENABLED=0 go test ./...
```

The tests check that every icon parses and has Lucide's defaults, render some,
check that every deprecated name points at an existing icon, and compare 1,902
icons at 24 px and at scales 1 and 2 with headless Chromium's rendering of the
same SVG (screenshots in `testdata/chromium`, 2 MB;
`TELA_CHROMIUM=1 go test -run TestChromiumLucide .` takes them again, it needs
`chromium`, and `TELA_CHROMIUM_DUMP=dir` writes the images and the worst cells
side by side).

### Regenerating (a new Lucide release)

Download the package without running its scripts and unpack it in an empty
directory:

```bash
mkdir /tmp/lucide && cd /tmp/lucide
npm pack lucide-static@X.Y.Z --ignore-scripts
mkdir x && tar xzf lucide-static-X.Y.Z.tgz -C x
```

Then, in this directory: copy `x/package/LICENSE` over `LICENSE`, change
`Version` and the `-source` flag of the `go:generate` line in `gen.go`, and run

```bash
LUCIDE_ICONS=/tmp/lucide/x/package/icons CGO_ENABLED=0 go generate ./...
```

This writes two files. `gen_deprecated.go` (a program that `go generate` runs
first) writes `deprecated.json`: the old names Lucide keeps, each with the icon
that replaces it, found from the package (an SVG that is not in
`icon-nodes.json` is an old name, and `dist/esm/lucide-static.mjs` says which
icon it became). Then `tela icons gen -deprecated deprecated.json` writes
`icons_gen.go`, with a `Deprecated: use X.` paragraph on those variables. The
output is the same on every run for the same package and the same generator.

Review the diff (the generator stops on an SVG that does not parse, on two
files that make the same name, and on a minified document that would draw
differently), run the tests and, if icons changed in ways that matter,
`TELA_CHROMIUM=1` again. Tag the release commit `icons/lucide/vX.Y.Z`, or
`icons/lucide/vX.Y.Z-tela.N` when only the generator changed.
