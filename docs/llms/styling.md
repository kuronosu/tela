# Recipe: styling in full

Questions: which Tailwind classes and variants work, which do not; theme tokens, a brand theme, class aliases; colors in Go; fonts; icons; stacking variants (dark, responsive, aria, group).

Classes are Tailwind CSS v4's, verbatim, parsed into typed styles. If you would write it in React + Tailwind v4, write the same here. Order: element defaults, then classes in order (a later class wins over an earlier one that sets the same property; `Class` may repeat and takes several strings), then `Style(...)` props. No cascade, no selectors, no stylesheet. Everything is `border-box`. `Div` is `display: block`, like HTML.

**Supported:** spacing (`p-4`, `px-6`, `m-auto`, `gap-3`, `space-y-4`), sizing (`w-64`, `w-full`, `max-w-3xl`, `min-h-screen`, `size-8`, `h-[72px]`, `aspect-video`), flex (`flex`, `flex-col`, `items-center`, `justify-between`, `flex-1`, `shrink-0`, `grow`, `basis-1/3`, `flex-wrap`, `order-*`), grid (`grid grid-cols-3`, `grid-cols-[200px_1fr]`, `col-span-2`, `place-items-center`), positioning (`relative`, `absolute`, `fixed`, `sticky top-0`, `inset-0`, `z-10`), overflow and scrolling (`overflow-y-auto`, `overscroll-contain`, `scroll-smooth`), colors with opacity (`bg-white/15`, `text-zinc-500`, `bg-[#1e1e2e]`), borders (`border`, `border-b`, `border-border`, `rounded-xl`, `rounded-t-lg`, `divide-y divide-border` for the lines between the children, `divide-x-2`), `ring-*`, `ring-offset-*`, `outline-*`, shadows (`shadow-sm`...`shadow-2xl`, `inset-shadow-*`), gradients (`bg-linear-to-br from-indigo-500 via-purple-500 to-pink-500`, `bg-radial`, `bg-conic`), typography (`text-sm`, `font-semibold`, `tracking-tight`, `leading-6`, `truncate`, `line-clamp-2`, `text-balance`, `tabular-nums`, `uppercase`, `font-mono`, `font-['Family']`, `whitespace-pre`, `break-words`), effects (`opacity-50`, `blur-sm`, `backdrop-blur`, `drop-shadow-*`, `mix-blend-*`, masks), transforms (`translate-x-2`, `scale-95`, `rotate-45`, `origin-*`; 2D only), transitions (`transition`, `transition-colors`, `duration-200`, `ease-out`, `delay-*`), `animate-spin`/`pulse`/`ping`/`bounce` (stagger with `delay-150` or `[animation-delay:-0.3s]`), `cursor-pointer`, `pointer-events-none`, `select-none`, `sr-only`, `field-sizing-content`, background images (`bg-[url(img/hero.jpg)] bg-cover`, files registered with `tela.Assets(fsys)`), `[clip-path:circle(50%)]`, arbitrary values (`w-[calc(50%-8px)]`, `top-[3px]`, `grid-cols-[repeat(auto-fill,minmax(200px,1fr))]`; the theme's variables work inside them: `bg-[conic-gradient(var(--color-primary)_72%,transparent_0)]`, `shadow-[0_0_0_2px_var(--color-ring)]`, `rounded-[var(--radius)]`, `bg-(--color-primary)`, with a token's light and dark value following the scheme). Units: `px`, `rem`, `em`, `%`, `vw`, `vh`, `fr`, `ch`. `rem` follows the system's text scale like a browser's font size (`text-*` and `p-4` grow with it, px values do not; `App.IgnoreTextScale` opts out). `calc()` only as `min()`, `max()`, `clamp()` and `a ± b`.

**Variants** (stack them: `dark:md:hover:bg-zinc-800`):
- State: `hover:` `active:` `focus:` `focus-visible:` `focus-within:` `disabled:` `enabled:` `checked:`, text fields `placeholder:` `invalid:` `read-only:` `placeholder-shown:` `selection:`.
- ARIA: `aria-checked:` `aria-selected:` `aria-expanded:` `aria-pressed:` `aria-invalid:` `aria-disabled:` `aria-busy:` `aria-hidden:` `aria-required:` `aria-readonly:` (they match the element's own `h.Aria...(true)` prop), `aria-[current=page]:` (also `step`, `location`, `date`, `time`, `true`; there is no bare `aria-current:`), `aria-[sort=ascending|descending|other|none]:`.
- Structure: `first:` `last:` `only:` `odd:` `even:` (the place among the element's own siblings: don't wrap items in a keyed `Div` just for the key, use `Key(...)` or `tela.WithKey`, which add no element); `group` + `group-hover:`/`group-focus-within:`/`group-aria-*:` on descendants; `peer` + `peer-checked:`/`peer-invalid:`... on following siblings (no named groups).
- Environment: `dark:`, `sm:` `md:` `lg:` `xl:` `2xl:` (window at least 640/768/1024/1280/1536 px), `max-md:` ..., `min-[600px]:`, `max-[40rem]:`, `motion-reduce:`, `motion-safe:`, `contrast-more:` (the system asks for high contrast), `rtl:`/`ltr:`, `starting:` (entrance: the style a newly shown element transitions from, CSS `@starting-style`).

```go
func NavLink(label string, current bool, onClick func()) tela.Node {
	page := ""
	if current {
		page = "page"
	}
	return Button(AriaCurrent(page), OnClick(onClick),
		Class("group flex h-9 w-full items-center gap-3 rounded-md px-3 text-sm text-muted-foreground transition-colors",
			"hover:bg-accent hover:text-accent-foreground",
			"aria-[current=page]:bg-accent aria-[current=page]:font-medium aria-[current=page]:text-foreground"),
		Icon(lucide.Bell, Class("size-4 group-hover:text-foreground")), Text(label))
}

func Toast(message string) tela.Node {
	// Fades and slides in when it appears; under Presence, Exit classes run when it is removed.
	return Div(Class("rounded-lg border border-border bg-popover px-4 py-3 text-sm shadow-lg transition duration-300",
		"starting:translate-y-2 starting:opacity-0"), Exit("translate-y-2 opacity-0"), Role("status"), Text(message))
}

func Responsive() tela.Node {
	return Div(Class("grid grid-cols-1 gap-4 p-4 md:grid-cols-2 xl:grid-cols-3"),
		Div(Class("h-24 rounded-xl bg-linear-to-br from-sky-400 to-indigo-600 shadow-lg dark:from-sky-600 dark:to-indigo-900")),
		Div(Class("h-24 rounded-xl bg-white/60 ring-1 ring-black/5 backdrop-blur dark:bg-white/5 dark:ring-white/10")))
}
```

**Theme tokens** (shadcn/ui names) work in every color utility (`bg-`, `text-`, `border-`, `ring-`, `from-`, `fill-`, `stroke-`, `shadow-`...), with opacity (`bg-primary/90`, `ring-ring/50`) and variants, and follow the color scheme by themselves: no `dark:` pairs needed. Tokens: `background`, `foreground`, `card`, `card-foreground`, `popover`, `popover-foreground`, `primary`, `primary-foreground`, `secondary`, `secondary-foreground`, `muted`, `muted-foreground`, `accent`, `accent-foreground`, `destructive`, `border`, `input`, `ring`, `chart-1`..`chart-5`. Give the window `RootClass: "bg-background text-foreground"`. Palette colors (`bg-zinc-900`) do not follow the scheme: pair them with `dark:`.

`kit.Theme()` is shadcn's neutral theme with its 10 px radius (`rounded-lg` = 10 px, `rounded-md` 8, `rounded-xl` 14); `tela.DefaultTheme()` has the same colors with Tailwind's radius scale. Change colors, add your own tokens, alias class combinations:

```go
var brandTheme = func() tela.Theme {
	t := kit.Theme()
	t.Colors["primary"] = tela.SchemeColor{Light: tela.Hex("#4f46e5"), Dark: tela.Hex("#818cf8")}
	t.Colors["primary-foreground"] = tela.SchemeColor{Light: tela.Hex("#ffffff"), Dark: tela.Hex("#1e1b4b")}
	t.Colors["brand"] = tela.SchemeColor{Light: tela.Hex("#7c3aed"), Dark: tela.Hex("#a78bfa")} // bg-brand, text-brand...
	t.FontSans = "Geist"                                                                         // a system font
	return t
}()

var BrandApp = tela.App{
	Title: "Notes", Theme: &brandTheme, RootClass: "bg-background text-foreground",
	ClassAliases: map[string]string{
		"text-muted": "text-sm text-muted-foreground",
		"eyebrow":    "text-xs font-semibold tracking-wide text-brand uppercase",
	},
	Root: P(Class("eyebrow"), Text("Notes")),
}
```

Colors in Go: `tela.Hex("#4f46e5")`, `tela.RGB`, `tela.RGBA(r, g, b, a)`, `tela.OKLCH(l, c, h)`, `tela.UseTheme().Color("primary")`. `h.Scheme(tela.Dark)` gives a subtree its own scheme (a dark sidebar in a light app). `tela.SetColorScheme(tela.Dark)` switches the app's scheme at run time, and `tela.SetTheme(t)` its theme: an accent picker copies `kit.Theme()`, sets `t.Colors["primary"]` (and `ring`) and calls `tela.SetTheme(t)`, so every `bg-primary` follows.

**Fonts:** text uses the system's fonts; none are in the binary. `font-sans` is `ui-sans-serif, system-ui, sans-serif`. To fix the look: `tela.App{Fonts: []tela.Font{inter.Sans}}` (`fonts/inter`, 1.8 MB) or your files, `tela.FontData("Lora", ttf)` / `tela.FontFS(fsys, "Lora", "fonts/*.ttf")` plus `.Sans()`, `.Serif()`, `.Mono()`, `.Scripts("Hebr")`; app fonts come first. Tests, snapshots and mcp use the App's fonts, then Inter, Source Serif 4 and JetBrains Mono, never the system's (`telatest.Fonts`).

**Icons:** `Icon(lucide.Settings, Class("size-4"))`. An icon is 1em square (follows `text-*` size) unless sized, colored by the text color (`text-muted-foreground`), `stroke-[1.5]` thins it; decorative unless it has an `AriaLabel`. Lucide names are the file names of lucide.dev in PascalCase (`arrow-up-right` is `lucide.ArrowUpRight`, `circle-alert` is `lucide.CircleAlert`); to check one, `go doc github.com/kuronosu/tela/icons/lucide CircleAlert` (an old name such as `AlertCircle` compiles and its doc says what replaces it). Your own SVGs: `tela.Icon{SVG: "<svg ...>"}`, `tela.MustSVG(data)`, or `tela icons gen -pkg icons -o ./icons ./svg` for a whole directory.

**Not implemented** (you get a dev error that says what to use): floats and `clear` (use flex/grid); margin collapsing (use `gap`/`space-y-*`); `display: table` and `border-collapse` (use `kit.Table` or grid); `space-*-reverse`; `columns-*`; pseudo-elements `before:` `after:` `marker:` `file:` (use nodes); `data-*:`, `has-*:`, `not-*:`, `nth-*:`, `*:`, `[&...]:`, `@container`/`@md:`, `print:`, `supports-*:`; CSS variables other than the theme's own (`var(--color-<token>)`, `var(--radius)`, `var(--radius-<size>)`, `var(--spacing)`; an app's `--foo` is an error); arbitrary properties other than `[clip-path:...]` and `[animation-delay:...]`; 3D transforms; `scroll-snap`, `scroll-m-*`; `text-indent`; `hyphens`; `accent-*`; `resize`; `bg-fixed`; `vertical-align` on spans; custom `@keyframes` (use the `animate-*` classes, springs or `tela.Animate`); `content-box`.
