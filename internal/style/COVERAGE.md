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
| Position | `static relative absolute fixed sticky`, `inset-*`, `inset-x/y-*`, `top/right/bottom/left-*`, `start-*`, `end-*` | values: spacing scale, `px`, fractions, `full`, `auto`, arbitrary, negatives. `start`/`end` are left/right (LTR) |
| z-index, overflow, visibility | `z-N z-auto z-[N] -z-N`, `overflow(-x/-y)-{auto,hidden,clip,visible,scroll}`, `visible invisible collapse`, `isolate isolation-auto` | |
| Sizing | `w h size min-w min-h max-w max-h` with spacing, `px`, fractions, `full`, `auto`, `screen`/`svw lvw dvw svh lvh dvh` (as vw/vh), `min max fit`, container names (`md`, `3xl`), `prose` (65ch), `screen-md`, `none`, arbitrary, `calc()`, `min()`, `max()`, `clamp()` | `aspect-{auto,square,video,N/M,[ratio]}`, `container` (width 100% and a max-width per breakpoint) |
| Spacing | `p m` with `x y t r b l s e`, negative margins, `m-auto`, `gap gap-x gap-y`, `space-x-N space-y-N` | `space-*` is implemented as gap (ARCHITECTURE section 10); `ComputedStyle.SpaceX/SpaceY` mark it so layout also applies it in block containers |
| Flex | `flex-{row,row-reverse,col,col-reverse,wrap,wrap-reverse,nowrap,1,auto,initial,none,N,[..]}`, `grow shrink basis order` | `flex-grow`/`flex-shrink` (v3) accepted |
| Alignment | `justify-* items-* self-* content-* justify-items-* justify-self-* place-content-* place-items-* place-self-*` | `-safe` variants not supported |
| Grid | `grid-cols-N/none/[..]`, `grid-rows-*`, `col-span-N/full`, `col-start-*`, `col-end-*`, `col-auto`, `col-[..]` (same for `row`), `grid-flow-*`, `auto-cols-*`, `auto-rows-*` | track lists support `px rem % fr auto min-content max-content minmax() fit-content() repeat()` incl. `auto-fill`/`auto-fit`. Layout arrives in phase 6 |
| Background | `bg-{color}[/N]`, `bg-[#hex]`, `bg-[rgb()]`, `bg-[oklch()]`, `bg-[url()]`, `bg-{cover,contain,auto}`, `bg-{center,top,...}`, `bg-{repeat,no-repeat,repeat-x,repeat-y,repeat-round,repeat-space}`, `bg-clip-*`, `bg-origin-*`, `bg-none`, `bg-position-[..]`, `bg-size-[..]`, `bg-[length:..]` | `bg-clip-text` is stored; text clipping is not painted yet |
| Gradients | `bg-linear-{to-t..to-tl,N,[..]}`, `bg-radial[-[..]]`, `bg-conic[-N,-[..]]`, `/srgb /oklab`, `from-* via-* to-*` (color, `%`), `-bg-linear-N` | v3 `bg-gradient-to-*` accepted. Default interpolation is Oklab. `Gradient.Paint(rect)` builds the gfx paint (CSS gradient-line formulas) |
| Borders | `border border-{0,2,4,8,[..]}`, per side `x y t r b l s e`, `border-{color}[/N]`, `border-{solid,dashed,dotted,double,hidden,none}` | border style drawing arrives with the paint work |
| Radius | `rounded` with `t r b l s e tl tr br bl ss se ee es`, `none sm md lg xl 2xl 3xl 4xl full`, arbitrary (`[50%]`) | `full` is 9999px |
| Outline, ring | `outline outline-N outline-{style} outline-none outline-hidden outline-offset-N outline-{color}`, `ring ring-N ring-inset ring-{color} ring-offset-N ring-offset-{color} inset-ring-N inset-ring-{color}` | rings are box shadows, composed like Tailwind (inset shadow, inset ring, ring offset, ring, shadow) |
| Shadows | `shadow-{2xs..2xl,inner,none}`, `shadow-{color}[/N]`, `shadow-[..]`, `inset-shadow-*`, `text-shadow-*`, `drop-shadow-*` | the color class replaces the color of every layer, whatever the order |
| Effects | `opacity-N`, `blur-*`, `brightness-N contrast-N grayscale invert saturate-N sepia hue-rotate-N`, `drop-shadow-*`, the same with `backdrop-`, `filter-none`, `mix-blend-*` | filters are applied in Tailwind's fixed order. `FilterSet.List()` gives gfx filters |
| Typography | `text-{xs..9xl}[/leading]`, `text-[len]`, `text-{color}[/N]`, `text-{left,center,right,justify,start,end}`, `text-{wrap,nowrap,balance,pretty}`, `text-{ellipsis,clip}`, `font-{thin..black}`, `font-{sans,serif,mono}`, `font-[..]`, `font-stretch-*`, `italic not-italic`, `tracking-*`, `leading-*`, `uppercase lowercase capitalize normal-case`, `underline overline line-through no-underline`, `decoration-*`, `underline-offset-*`, `truncate`, `line-clamp-N`, `whitespace-*`, `break-{normal,words,all,keep}`, `wrap-*`, numeric variants (`tabular-nums` ...) | `text-balance`/`text-pretty` behave as `text-wrap`. `text-wrap` composes with `whitespace-*` |
| Transforms | `translate(-x/-y)-*` (incl. `full`, fractions, negatives), `scale(-x/-y)-N`, `rotate-N`, `skew(-x/-y)-N`, `origin-*`, `transform-none`; `transform transform-gpu` are no-ops | 2D only. `Transform.Matrix(box)` builds the gfx.Affine |
| Transitions | `transition[-{all,colors,opacity,shadow,transform,none,[..]}]`, `duration-N`, `ease-{linear,in,out,in-out,[..]}`, `delay-N`, `animate-{spin,ping,pulse,bounce,none,[..]}` | the animation shorthand is kept raw until phase 6 |
| Interaction | `cursor-*`, `pointer-events-{none,auto}`, `select-{none,text,all,auto}` | |
| Variants | `hover active focus focus-visible focus-within disabled enabled checked`, `first last only odd even`, `group-*`, `peer-*` (states above), `dark`, `motion-reduce`, `motion-safe`, `sm md lg xl 2xl`, stacked (`dark:md:hover:`) | evaluated against `VariantContext`; core feeds real state from phases 4 and 6 |
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
| Arbitrary gradients | `linear-gradient() radial-gradient() conic-gradient()` args, stops with one or two positions, theme color names | repeating gradients, color hints (a lone percentage between stops), `in <colorspace>` inside the function |
| `rem` in shadows and grid tracks | resolved with the theme's root size when the class is parsed | em is not element-relative there |
| Color names | `transparent`, `current`, theme names, hex, `rgb() hsl() oklch() oklab()` | CSS keyword colors (`red`): the error suggests a hex value |
| Interpolation modifiers | `/srgb`, `/oklab` | `/oklch`, `/hsl`, `/longer`...: gfx has no polar interpolation yet (ADR-023) |
| Logical properties | `s`/`e`/`start`/`end` map to left/right | right-to-left layout (future) |
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
| `content-[..]`, `before:`, `after:`, `placeholder:`, `marker:`, `selection:` | regular nodes |
| arbitrary properties `[mask-type:luminance]`, arbitrary variants `[&>*]:` | a utility class |
| `var(--x)`, `p-(--x)` | theme tokens and arbitrary values |
| named groups/peers `group/name`, `group-hover/name:` | plain `group` and `peer` |
| `aria-*:`, `data-*:`, `has-*:`, `not-*:`, `max-*:`, `min-*:`, `@container` variants, `print:`, `rtl:`, `ltr:` | explicit styles and the supported variants |
| `rotate-x/y/z`, `translate-z`, `perspective-*`, `transform-3d`, `backface-*` | 2D transforms |
| `fill-*`, `stroke-*` | icons take the text color (until SVG support) |
| `object-*` | size the Img explicitly (until image fitting) |
| `list-*`, `indent-*`, `hyphens-*`, `align-*` (vertical-align) | regular nodes; padding; flex alignment |
| `accent-*`, `caret-*`, `resize`, `overscroll-*`, `scroll-*`, `snap-*`, `touch-*`, `will-change-*`, `mask-*`, `clip-path`, `field-sizing`, `contain-*`, `content-visibility-*` | not supported yet or not needed |
| `bg-fixed bg-local bg-scroll`, `bg-blend-*` | `position: fixed` layers, `mix-blend-*` |
| `backdrop-opacity-*`, `mix-blend-plus-darker` | `backdrop-brightness`, `mix-blend-multiply` |
| `grid-cols-subgrid`, grid line names | declare the tracks again; `col-start-N` |
| CSS keyword colors, `color-mix()`, `lab()`, `lch()`, `hwb()`, `color()` | hex, `rgb()`, `hsl()`, `oklch()`, `oklab()` |
| units `cm mm in pt pc ex vmin vmax lh` | `px rem em % vw vh ch` |
