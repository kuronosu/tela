# Class coverage

What `internal/style` parses (Tailwind CSS v4.3 vocabulary). "Parsed" means the
class compiles and its value survives in `ComputedStyle`; layout, paint and
animation consume most of it in later phases (noted per row). A class outside
this table is an `UnknownClassError` with a suggestion; a class in the
"not supported" table is an `UnsupportedClassError` that says what to use.

## Supported

| Category | Classes | Notes |
|---|---|---|
| Display | `block inline inline-block flex inline-flex grid inline-grid contents hidden flow-root` | `flow-root` is `block`. `contents` is stored; layout may not honor it yet |
| Position | `static relative absolute fixed sticky`, `inset-*`, `inset-x/y-*`, `top/right/bottom/left-*`, `start-*`, `end-*`, `inset-s-*`, `inset-e-*`, `inset-bs-*`, `inset-be-*` | values: spacing scale, `px`, fractions, `full`, `auto`, arbitrary, negatives. `start`/`end`/`inset-s`/`inset-e` are logical: left/right, or right/left under `direction: rtl` (`h.Dir`); `inset-bs`/`inset-be` are top/bottom |
| z-index, overflow, visibility | `z-N z-auto z-[N] -z-N`, `overflow(-x/-y)-{auto,hidden,clip,visible,scroll}`, `visible invisible collapse`, `isolate isolation-auto` | |
| Sizing | `w h size min-w min-h max-w max-h` with spacing, `px`, fractions, `full`, `auto`, `screen`/`svw lvw dvw svh lvh dvh` (as vw/vh), `min max fit`, container names (`md`, `3xl`), `prose` (65ch), `screen-md`, `none`, arbitrary, `calc()`, `min()`, `max()`, `clamp()` | `aspect-{auto,square,video,N/M,[ratio]}`, `container` (width 100% and a max-width per breakpoint) |
| Spacing | `p m` with `x y t r b l s e bs be`, negative margins, `m-auto`, `gap gap-x gap-y`, `space-x-N space-y-N` | `space-*` is implemented as gap (ARCHITECTURE section 10); `ComputedStyle.SpaceX/SpaceY` mark it so layout also applies it in block containers. `s e` and `space-x` are logical (the inline start is the right in rtl); `bs be` are top and bottom |
| Flex | `flex-{row,row-reverse,col,col-reverse,wrap,wrap-reverse,nowrap,1,auto,initial,none,N,[..]}`, `grow shrink basis order` | `flex-grow`/`flex-shrink` (v3) accepted |
| Alignment | `justify-* items-* self-* content-* justify-items-* justify-self-* place-content-* place-items-* place-self-*` | `-safe` variants not supported |
| Grid | `grid-cols-N/none/[..]`, `grid-rows-*`, `col-N`, `-col-N`, `col-span-N/full/[..]`, `col-start-*`, `col-end-*` (and negative `-col-start-N`), `col-auto`, `col-[..]` (same for `row`), `grid-flow-*`, `auto-cols-*`, `auto-rows-*`, `inline-grid` | track lists support `px rem % fr auto min-content max-content minmax() fit-content() repeat()` incl. `auto-fill`/`auto-fit`, and line names in arbitrary values: `grid-cols-[[sidebar]_200px_[main]_1fr]`, `grid-cols-[repeat(3,[col]_1fr)]`. Placements take line numbers, spans and names: `col-[main]` (main-start / main-end, or the lines named main), `col-start-[main]`, `col-[2_side/span_col_2]`, `col-span-[card]`. Laid out by `internal/layout` (ADR-073) |
| Background | `bg-{color}[/N]`, `bg-[#hex]`, `bg-[rgb()]`, `bg-[oklch()]`, `bg-[url()]`, `bg-{cover,contain,auto}`, `bg-{center,top,...}`, `bg-{repeat,no-repeat,repeat-x,repeat-y,repeat-round,repeat-space}`, `bg-clip-*`, `bg-origin-*`, `bg-none`, `bg-position-[..]`, `bg-size-[..]`, `bg-[length:..]`, layer lists `bg-[linear-gradient(..),url(..)]` (first on top), `bg-blend-*` (no `plus-lighter`); typed `BgImage(image.Image)` | `url()` names a file of the file systems registered with `tela.Assets` (ADR-079); size, position, repeat and origin apply to every layer. `bg-clip-text` paints the backgrounds through the glyphs of the element's text |
| Gradients | `bg-linear-{to-t..to-tl,N,[..]}`, `bg-radial[-[..]]`, `bg-conic[-N,-[..]]`, `/srgb /oklab`, `from-* via-* to-*` (color, `%`), `-bg-linear-N` | v3 `bg-gradient-to-*` accepted. Default interpolation is Oklab for the named utilities and sRGB for the arbitrary forms (`bg-linear-[..]`, `bg-[linear-gradient(..)]`), as in CSS and Tailwind; write `in oklab` / `in srgb` in the value to choose (ADR-158). `Gradient.Paint(rect)` builds the gfx paint (CSS gradient-line formulas) |
| Borders | `border border-{0,2,4,8,[..]}`, per side `x y t r b l s e bs be` (`s e` logical), `border-{color}[/N]`, `border-{solid,dashed,dotted,double,hidden,none}` | border style drawing arrives with the paint work |
| Radius | `rounded` with `t r b l s e tl tr br bl ss se ee es` (`s e ss se ee es` logical), `none sm md lg xl 2xl 3xl 4xl full`, arbitrary (`[50%]`) | `full` is 9999px |
| Outline, ring | `outline outline-N outline-{style} outline-none outline-hidden outline-offset-N outline-{color}`, `ring ring-N ring-inset ring-{color} ring-offset-N ring-offset-{color} inset-ring-N inset-ring-{color}` | rings are box shadows, composed like Tailwind (inset shadow, inset ring, ring offset, ring, shadow) |
| Shadows | `shadow-{2xs..2xl,inner,none}`, `shadow-{color}[/N]`, `shadow-[..]`, `inset-shadow-*`, `text-shadow-*`, `drop-shadow-*` | the color class replaces the color of every layer, whatever the order |
| Effects | `opacity-N`, `blur-*`, `brightness-N contrast-N grayscale invert saturate-N sepia hue-rotate-N`, `drop-shadow-*`, the same with `backdrop-`, `filter-none`, `mix-blend-*`, the arbitrary property `[clip-path:..]` with `inset()` (and `round`), `circle()`, `ellipse()`, `polygon()` (optional `nonzero`/`evenodd`) and `none` | filters are applied in Tailwind's fixed order. `FilterSet.List()` gives gfx filters. `[clip-path:..]` is the only arbitrary property (Tailwind has no clip-path utility; ADR-079); variants apply to it, and `transition-[clip-path]` interpolates shapes of the same kind and length |
| SVG paints | `fill-{color}[/N]`, `fill-current`, `fill-none`, `fill-[..]`, `stroke-{color}[/N]`, `stroke-current`, `stroke-none`, `stroke-N` (integers), `stroke-[1.5]`, `stroke-[2px]`, `stroke-[length:..]`, tokens (`fill-primary`) | inherited like CSS; `fill-current` keeps the keyword, so it takes the color of the element that draws the SVG. They are the root overrides of `Icon` and SVG `Img` documents (ADR-098); not transitioned |
| Replaced content | `object-{contain,cover,fill,none,scale-down}`, `object-{center,top,bottom,left,right,top-left,...}`, `object-[25%_75%]` | `Img`, raster and SVG sources (ADR-098) |
| Masks | `mask-[url(..)]` (PNG, JPEG, GIF, WebP, SVG through `tela.Assets`), `mask-[linear-gradient(..)]`, lists, `mask-none`, `mask-{contain,cover,auto}`, `mask-size-[..]`, `mask-[size:..]`, `mask-{center,top,...,bottom-right}`, `mask-position-[..]`, `mask-repeat*`, `mask-no-repeat`, `mask-origin-*`, `mask-clip-*`, `mask-no-clip`, `mask-{alpha,luminance,match}`, `mask-{add,subtract,intersect,exclude}`, `mask-type-*` (no effect on elements, as in browsers); gradient masks `mask-{t,r,b,l,x,y}-{from,to}-*`, `mask-linear-N`, `-mask-linear-N`, `mask-linear-[..]`, `mask-linear-{from,to}-*`, `mask-radial-{from,to}-*`, `mask-circle`, `mask-ellipse`, `mask-radial-{closest,farthest}-{side,corner}`, `mask-radial-[..]`, `mask-radial-at-*`, `mask-conic-N`, `mask-conic-{from,to}-*` (stops: `%`, spacing numbers, lengths, colors, tokens) | Tailwind's CSS variables are emulated (ADR-098): the gradient families are intersected, a family that would be invalid in CSS leaves the element unmasked. Classes that set the same variable follow the class order, the last wins (ADR-103), where Tailwind follows its stylesheet order. A mask makes a stacking context |
| Typography | `text-{xs..9xl}[/leading]`, `text-[len]`, `text-{color}[/N]`, `text-{left,center,right,justify,start,end}`, `text-{wrap,nowrap,balance,pretty}`, `text-{ellipsis,clip}`, `font-{thin..black}`, `font-{sans,serif,mono}`, `font-[..]`, `font-stretch-*`, `italic not-italic`, `tracking-*`, `leading-*`, `uppercase lowercase capitalize normal-case`, `underline overline line-through no-underline`, `decoration-*`, `underline-offset-*`, `truncate`, `line-clamp-N`, `whitespace-*`, `break-{normal,words,all,keep}`, `wrap-*`, numeric variants (`tabular-nums` ...) | `text-balance`/`text-pretty` behave as `text-wrap`. `text-wrap` composes with `whitespace-*` |
| Transforms | `translate(-x/-y)-*` (incl. `full`, fractions, negatives), `scale(-x/-y)-N`, `rotate-N`, `skew(-x/-y)-N`, `origin-*`, `transform-none`; `transform transform-gpu` are no-ops | 2D only. `Transform.Matrix(box)` builds the gfx.Affine |
| Transitions | `transition[-{all,colors,opacity,shadow,transform,none,[..]}]`, `duration-N`, `ease-{linear,in,out,in-out,[..]}`, `delay-N`, `animate-{spin,ping,pulse,bounce,none,[..]}` | `animate-*` classes keep the animation shorthand raw in `ComputedStyle.Animation`; `core` parses it (`anim.ParseSpec`) and runs the built-in keyframes spin, ping, pulse and bounce (ADR-076). Transitions animate colors, opacity, shadows, transforms, filters, sizes, spacing, insets, border radius and width, font size and clip paths; `ease-[steps(..)]` is not parsed yet; `animate-[custom]` needs the Go keyframes API and is reported in dev mode |
| Interaction | `cursor-*`, `pointer-events-{none,auto}`, `select-{none,text,all,auto}` | |
| Text fields | `caret-{color}[/N]`, `caret-transparent`, `caret-current`, `field-sizing-{content,fixed}`, variants `placeholder:` (text properties of the placeholder) and `selection:` (`text-*` and `bg-*` colors of selected text in the element and its descendants) | `caret-*` is inherited; Input and Textarea use them (ADR-069) |
| Scrolling | `scroll-auto scroll-smooth`, `overscroll(-x/-y)-{auto,contain,none}`; Tela-specific: `scroll-instant`, `scrollbar-classic scrollbar-overlay` | not inherited. `scroll-smooth` animates `ScrollTo`/`ScrollBy`/`ScrollIntoView` (CSS) and also user scrolling when the app sets `InstantScroll`; `scroll-instant` turns off animation and flings for user scrolling; `scrollbar-*` override `App.ClassicScrollbars` per element. `overscroll-none` is `contain` (no overscroll effect is drawn) |
| Variants | `hover active focus focus-visible focus-within disabled enabled checked read-only placeholder-shown invalid`, `first last only odd even`, `group-*`, `peer-*` (states above), `dark`, `motion-reduce`, `motion-safe`, `starting` (CSS `@starting-style`: the style a newly rendered element transitions from), `rtl ltr` (the element's direction, from `h.Dir`), `sm md lg xl 2xl` (width at least), `max-sm … max-2xl` (width below), `min-sm … min-2xl`, `min-[600px]` and `max-[40rem]` (px, rem, em), `aria-busy aria-checked aria-disabled aria-expanded aria-hidden aria-pressed aria-readonly aria-required aria-selected` (the element's `h.Aria…` prop is true; also `group-aria-*`, `peer-aria-*` and `aria-[checked=true]`), stacked (`dark:md:max-xl:hover:`; ADR-097) | evaluated against `VariantContext`; core feeds real state from phases 4 and 6 |
| Theme tokens | every color utility above with a token of the theme (`bg-primary`, `text-muted-foreground/80`, `border-border`, `ring-ring/50`, `from-chart-1`...): shadcn/ui's names by default, plus the theme's own; class aliases of a `NewParserAliases` parser | resolved for the element's color scheme in `Compute` (ADR-089); not inside arbitrary values |
| Markers | `group`, `peer` | reported in `Classes.Group` / `Classes.Peer` |
| Accepted, ignored | `box-border appearance-none appearance-auto filter backdrop-filter antialiased subpixel-antialiased` | Tela is always border-box and has no native controls |
| Accessibility helpers | `sr-only not-sr-only` | 1px absolutely positioned box |
| Arbitrary values | lengths with `px rem em % vw vh ch` (and `svh lvh dvh ...` as vh), `calc()` (add, subtract, multiply or divide by a number), `min() max() clamp()` of lengths without percentages, colors (`#hex`, `rgb()`, `hsl()`, `oklch()`, `oklab()`), type hints (`length:`, `color:`, `number:`, `url:`...), underscores as spaces, opacity `/50` and `/[0.15]` | |

## Partially supported

| Feature | What works | What does not, and why |
|---|---|---|
| `min()`, `max()`, `clamp()` | arguments without percentages (`vw`, `rem`, `px`, `em`) | with a percentage the result depends on layout; the error says to use `calc()` or fixed lengths |
| `calc()` | `+ - * /` of lengths and numbers, nested parentheses and functions; a percentage can be mixed with absolute units (`calc(100% - 2rem)`) | a percentage inside `min()`/`max()`/`clamp()` (above) |
| Grid track units | `px rem %` | `em vw vh ch` and `min()`/`max()`/`clamp()` inside track lists are rejected for now |
| Named grid areas | the layout supports `grid-template-areas` and placement by area name; from classes, `col-[header]` and `row-[header]` place an item between lines that a track list names `header-start` and `header-end` (`grid-cols-[[header-start]_1fr_[header-end]]`) | Tailwind v4 has no `grid-template-areas` or `grid-area` utility, and arbitrary properties (`[grid-template-areas:..]`) are not supported: area templates are not reachable from classes |
| Arbitrary gradients | `linear-gradient() radial-gradient() conic-gradient()` args, stops with one or two positions, theme color names | repeating gradients, color hints (a lone percentage between stops), `in <colorspace>` other than `srgb` and `oklab` (`in oklch`, `in hsl`, hue methods: an error that says so) |
| `rem` in shadows and grid tracks | resolved with the theme's root size when the class is parsed | em is not element-relative there |
| Color names | `transparent`, `current`, theme names, hex, `rgb() hsl() oklch() oklab()` | CSS keyword colors (`red`): the error suggests a hex value |
| Interpolation modifiers | `/srgb`, `/oklab` (on the arbitrary forms a Tela extension: Tailwind rejects them there), `in srgb` / `in oklab` in the value | `/oklch`, `/hsl`, `/longer`...: gfx has no polar interpolation yet (ADR-023) |
| Logical properties | inline `s`/`e`/`start`/`end`/`inset-s`/`inset-e` resolve with the element's direction; block `bs`/`be` are top/bottom | the logical sizes `inline-*`, `block-*`, `min-inline-*`... and `scroll-m*`/`scroll-p*`; vertical writing modes |
| `contents`, `flow-root` | parsed | layout decides; `flow-root` is `block` |
| Color of an inset `shadow-inner` | uses the `shadow-*` color class too | Tailwind uses the inset shadow color |

## Not supported (what to use)

| Class or feature | Error says |
|---|---|
| `!important` (`!p-4`, `p-4!`) | drop the `!`: there is no cascade; later classes and Style props win |
| `float-*`, `clear-*` | flex or grid |
| `table`, `table-*`, `border-collapse`, `border-spacing-*` | grid (the kit Table is built on it) |
| `divide-*` | a border on the children, or gap |
| `space-x-reverse`, `space-y-reverse` | `flex-row-reverse`, `flex-col-reverse` |
| `box-content` | boxes are always border-box |
| `columns-*`, `break-before/after/inside-*` | grid |
| `content-[..]`, `before:`, `after:`, `marker:`, `file:` | regular nodes |
| arbitrary properties other than `[clip-path:..]` (`[mask-type:luminance]`), arbitrary variants `[&>*]:` | a utility class |
| `var(--x)`, `p-(--x)` | theme tokens and arbitrary values |
| named groups/peers `group/name`, `group-hover/name:` | plain `group` and `peer` |
| `aria-[sort=ascending]:` and other arbitrary aria selectors (Tela has no prop for them), `data-*:`, `has-*:`, `not-*:`, `@container` variants, `print:` | explicit styles and the supported variants |
| `rotate-x/y/z`, `translate-z`, `perspective-*`, `transform-3d`, `backface-*` | 2D transforms |
| `stroke-[N%]` | a number of user units or a length |
| `list-*`, `indent-*`, `hyphens-*`, `align-sub`, `align-super`, `align-[..]` | regular nodes; padding; `align-baseline/top/middle/bottom/text-top/text-bottom` (inline-level boxes in text) |
| `accent-*`, `resize`, `scroll-m*`, `scroll-p*`, `snap-*`, `touch-*`, `will-change-*`, `contain-*`, `content-visibility-*` | not supported yet or not needed |
| `bg-fixed bg-local bg-scroll` | `position: fixed` layers |
| `clip-path` (bare), `[clip-path:path()/url()/rect()/xywh()]`, reference boxes other than `border-box` | `[clip-path:inset()/circle()/ellipse()/polygon()]` |
| `backdrop-opacity-*`, `mix-blend-plus-darker` | `backdrop-brightness`, `mix-blend-multiply` |
| `grid-cols-subgrid`, `grid-rows-subgrid` | declare the tracks again on the nested grid |
| CSS keyword colors, `color-mix()`, `lab()`, `lch()`, `hwb()`, `color()` | hex, `rgb()`, `hsl()`, `oklch()`, `oklab()` |
| units `cm mm in pt pc ex vmin vmax lh` | `px rem em % vw vh ch` |
