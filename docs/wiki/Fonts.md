# Fonts

A Tela app draws its text with the fonts installed on the system, as a browser
does, and carries no font file in its binary. An app that wants one fixed look
brings the fonts it likes with `App.Fonts`, and pays for them in its binary
only then.

| Without `Fonts` | With `Fonts: []tela.Font{inter.Sans}` |
|---|---|
| Segoe UI on Windows, the system font on macOS, the desktop's font on Linux | Inter everywhere |
| `examples/hello` is 19.6 MB | 21.6 MB (Inter is 1.8 MB) |

## What `font-sans` draws

The theme's font stacks are CSS generic family names:

| Class | Family list | Role |
|---|---|---|
| `font-sans` (the default text) | `ui-sans-serif, system-ui, sans-serif` | the interface font |
| `font-serif` | `ui-serif, serif` | text with serifs |
| `font-mono` (`code`, `kbd`, `pre`) | `ui-monospace, monospace` | code |
| emoji | `emoji` | the color emoji font |

Each generic name resolves, per operating system, to the first installed
family of an ordered list: on macOS SF, New York, SF Mono and Apple Color
Emoji; on Windows Segoe UI, Cambria, Cascadia Mono (or Consolas) and Segoe UI
Emoji; on Linux the desktop's interface font where Tela can read it, then
fontconfig's usual preferences (Noto Sans, DejaVu Sans, Liberation Sans...).
A named family, `font-['Fira_Code']` (`_` for a space), is the installed family
of that name, and a character the families before it cannot draw (Japanese,
Hebrew, Thai...) comes from an installed font that has it.

The theme can put a family first: `theme.FontSans = "Geist"` (see
[Theming](Theming)).

If no font is installed at all (a minimal container), text has no glyphs and
the app prints once which package to import.

## Bringing a font

`tela.App.Fonts` lists the app's fonts. They come before the system's: a font
that stands for a generic name wins that name, and the app's fonts are tried
first when a character needs a fallback. Child windows share them.

```go
import "github.com/kuronosu/tela/fonts/inter"

func main() {
	tela.Main(tela.App{
		Title: "Notes",
		Fonts: []tela.Font{inter.Sans},
		Root:  Notes(),
	})
}
```

### Tela's fonts

Each is a small package of the core module. Importing it embeds the font and
nothing else; every font is read and parsed only when text first needs it.

| Package | Font | Role | Binary |
|---|---|---|---|
| `fonts/inter` | `inter.Sans`: Inter 4.1, upright and italic | `Sans()` | 1.8 MB |
| `fonts/jetbrainsmono` | `jetbrainsmono.Mono`: JetBrains Mono 2.304 | `Mono()` | 0.6 MB |
| `fonts/sourceserif` | `sourceserif.Serif`: Source Serif 4 | `Serif()` | 2.1 MB |
| `fonts/notoemoji` | `notoemoji.Emoji`: Noto Color Emoji (COLRv1) | `Emoji()` | 5.0 MB |
| `fonts/notoarabic` | `notoarabic.Arabic`: Noto Sans Arabic | `Scripts("Arab")` | 0.8 MB |
| `fonts/standard` | `standard.Fonts`: all of the above | | 10.3 MB |

All are licensed under the SIL Open Font License 1.1 (see
[`THIRD_PARTY.md`](https://github.com/kuronosu/tela/blob/main/THIRD_PARTY.md)).
Take only the ones you need: `Fonts: []tela.Font{inter.Sans, jetbrainsmono.Mono}`.

### Your own files

[`tela.FontData`](https://pkg.go.dev/github.com/kuronosu/tela#FontData) takes
files in memory, usually embedded. [`tela.FontFS`](https://pkg.go.dev/github.com/kuronosu/tela#FontFS)
takes the files an `fs.FS` (an `embed.FS`, `os.DirFS`) matches with `fs.Glob`
patterns, and reads them when text first needs the family.

```go
//go:embed fonts/Lora*.ttf
var fonts embed.FS

//go:embed Roboto.ttf RobotoItalic.ttf
var roboto, robotoItalic []byte

tela.App{
	Fonts: []tela.Font{
		tela.FontFS(fonts, "Lora", "fonts/Lora*.ttf").Serif(),
		tela.FontData("Roboto", roboto, robotoItalic).Sans(),
	},
}
```

The family is the name the font is used by, `font-['Lora']`; `""` takes the
name from the first file. Give the files of one family: an upright file and an
italic one, or one variable font (weights and optical sizes work as in CSS).
TrueType, OpenType/CFF and color fonts (COLRv1, CBDT, sbix) are supported.

A font is a value, and these methods return a modified copy:

| Method | Makes the font |
|---|---|
| `Sans()` | the family of `ui-sans-serif`, `system-ui` and `sans-serif`: `font-sans` and the default text |
| `Serif()` | the family of `ui-serif` and `serif` |
| `Mono()` | the family of `ui-monospace` and `monospace` |
| `Emoji()` | the family of `emoji` |
| `Scripts("Arab", "Hebr")` | the fallback for text in those scripts (ISO 15924 codes) that the requested family cannot draw; the umbrella codes stand for their scripts, as in browsers: `Hans` and `Hant` for `Hani`, `Jpan` for `Hani`, `Hira` and `Kana`, `Kore` for `Hang` and `Hani` |

Without a method the font is used by its name only. They combine:
`FontData("Fira", fira).Mono().Scripts("Latn")`.

Mistakes are reported where they can be fixed. `Run` checks the patterns
before it opens the window and returns an error naming one that matches no
file; a file that is not a font is logged when text first needs the family,
and the font is left out. A script code that is not the ISO 15924 code of a
script of text (a typo, or `Zyyy`, `Zinh`, `Zsye`) panics in dev mode with
examples of valid ones (release builds ignore it).

## Tests and snapshots

Tests, `tela snapshot`, `tela inspect` and `tela mcp` never use the system's
fonts, so goldens are the same on every machine. They use the app's fonts
(`telatest.FromApp(App)`, which `tela snapshot` applies from the package's
`var App`, or [`telatest.Fonts`](https://pkg.go.dev/github.com/kuronosu/tela/telatest#Fonts))
and then Tela's standard fonts: Inter for `font-sans`, Source Serif 4 for
`font-serif`, JetBrains Mono for `font-mono`, Noto Color Emoji and Noto Sans
Arabic.

```go
app := telatest.Mount(t, Notes(), telatest.Fonts(inter.Sans))
```

So an app on the system's fonts looks like Inter in a golden and like the
system's face in its window; an app that brings the fonts it uses looks the
same in both.

## Hyphenation

Hyphenation dictionaries work like fonts: none is in the binary unless the
app brings one. Soft hyphens (`\u00ad` in a Go string) always let a word
break with a hyphen, unless the text has `hyphens-none`. With
`hyphens-auto`, words also break where the dictionary of the text's
language allows, as in Chromium: only when the language is known (`h.Lang`,
or `tela.SetDefaultLang` for the whole app) and the app has a dictionary
for it.

```go
import (
    "github.com/kuronosu/tela/hyphen/en" // en.US, en.GB (54 KB)
    "github.com/kuronosu/tela/hyphen/es" // es.Spanish (16 KB)
)

tela.Main(tela.App{Hyphenation: []tela.Hyphenation{en.US, es.Spanish}, Root: Article()})

P(Class("hyphens-auto"), Lang("en"), Text("Internationalization..."))
```

`hyphen/fr` has French. Other languages take TeX patterns, such as
hyph-utf8's `hyph-it.pat.txt`: `tela.HyphenationPatterns("it", patterns, "")`.
Tests hyphenate only with the app's dictionaries (`telatest.FromApp`,
[`telatest.Hyphenation`](https://pkg.go.dev/github.com/kuronosu/tela/telatest#Hyphenation)).
