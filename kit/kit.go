// Portions of this file follow shadcn/ui, the theming CSS of apps/v4 at the commit
// kit.ShadcnCommit (https://github.com/shadcn-ui/ui), Copyright (c) 2023 shadcn,
// licensed under the MIT License.
// See THIRD_PARTY.md.

package kit

import (
	"fmt"
	"reflect"
	"sort"
	"strings"

	"github.com/kuronosu/tela"
	"github.com/kuronosu/tela/h"
)

// ShadcnCommit is the commit of github.com/shadcn-ui/ui (branch main) whose
// registry style "new-york-v4" (apps/v4/registry/new-york-v4/ui) the class
// strings of this package were ported from, and that the Chromium
// conformance pages in testdata/chromium are written against. Every
// component of the package follows this one commit (ADR-130).
const ShadcnCommit = "617134d5810bba521803b3ed787190bacbb80b35"

// Variant is the look of a component among the ones its recipe defines: a
// Button is Default, Secondary, Outline, Ghost, Destructive or Link. The
// constants are shared by the components that have the same variants; an
// app defines its own by adding a name to a recipe's Variants map
// (s.Button.Variants["glass"] = "...") and using Variant("glass").
type Variant string

// The variants shadcn/ui's components share. Default is the zero value, so
// leaving Variant out gives the default look.
const (
	Default     Variant = ""
	Secondary   Variant = "secondary"
	Outline     Variant = "outline"
	Ghost       Variant = "ghost"
	Destructive Variant = "destructive"
	Link        Variant = "link"
)

// Size is the size of a component among the ones its recipe defines. Like
// [Variant] it is open: an app adds a size by naming it in a recipe's
// Sizes map.
type Size string

// The sizes shadcn/ui's components share. The zero value is the default
// size. Icon, IconSm, IconLg and IconXs are the square sizes of a button
// that holds only an icon.
const (
	Xs     Size = "xs"
	Sm     Size = "sm"
	Lg     Size = "lg"
	Icon   Size = "icon"
	IconXs Size = "icon-xs"
	IconSm Size = "icon-sm"
	IconLg Size = "icon-lg"
)

// Orientation is the direction a component lays its parts out in. The zero
// value is the component's own default: Horizontal for a [Separator],
// Vertical for a [Field].
type Orientation string

const (
	Horizontal Orientation = "horizontal"
	Vertical   Orientation = "vertical"
)

// Theme returns the theme of the kit's look: shadcn/ui's neutral colors
// with a base radius of 10 px (shadcn's 0.625rem). It is [tela.DefaultTheme]
// with the radius shadcn's components were designed for, so the
// rounded-md, rounded-lg and rounded-xl of the recipes come out as 8, 10
// and 14 px.
//
//	theme := kit.Theme()
//	tela.Main(tela.App{Title: "Notes", Root: Notes{}, Theme: &theme,
//	    RootClass: "bg-background text-foreground"})
//
// Change the tokens on the returned value (theme.Colors["primary"] = ...)
// to restyle every component at once; see [DefaultStyle] to restyle one.
func Theme() tela.Theme {
	t := tela.DefaultTheme()
	t.Radius = 10
	return t
}

// Style holds the recipe of every component: the whole look of the kit as
// data (ADR-129, K17). [DefaultStyle] returns a copy of the defaults to
// edit, and [StyleProvider] applies an edited one to a subtree.
//
// A recipe is a struct of class strings. Its fields follow one rule:
//
//   - Root is the class of the component's root element, and is always
//     there;
//   - one field per other part, named after the part: Header, Title,
//     Description, Action, Content, Footer, Image, Fallback, Label, Error,
//     Icon;
//   - Variants, Sizes (and Orientations where there are two) are maps from
//     a [Variant], [Size] or [Orientation] to classes, so that new keys are
//     new variants;
//   - a state a component can be in without a variable of its own
//     (Disabled) is a string named after it.
//
// The classes go on the element in the order Root, variant, size, state,
// then the Class field of the component, and Tela's last-wins rule makes
// the last one decide, so a recipe or an instance overrides what comes
// before with no merging.
//
// Part names are public API; the strings are defaults and may change with
// the look.
type Style struct {
	AlertDialog  AlertDialogStyle
	Avatar       AvatarStyle
	Badge        BadgeStyle
	Button       ButtonStyle
	Calendar     CalendarStyle
	Card         CardStyle
	Checkbox     CheckboxStyle
	ColorInput   ColorInputStyle
	ColorPicker  ColorPickerStyle
	ContextMenu  MenuStyle
	DatePicker   DatePickerStyle
	Dialog       DialogStyle
	DropdownMenu MenuStyle
	Field        FieldStyle
	FieldSet     FieldSetStyle
	Input        InputStyle
	Label        LabelStyle
	Menubar      MenubarStyle
	Popover      PopoverStyle
	Progress     ProgressStyle
	RadioGroup   RadioGroupStyle
	Select       SelectStyle
	Separator    SeparatorStyle
	Skeleton     SkeletonStyle
	Slider       SliderStyle
	Switch       SwitchStyle
	Table        TableStyle
	Tabs         TabsStyle
	Textarea     TextareaStyle
	Toast        ToastStyle
	Toggle       ToggleStyle
	ToggleGroup  ToggleGroupStyle
	Tooltip      TooltipStyle
}

// defaultStyle is the kit's look. Each component file fills in its own
// field in an init function, so that adding a component touches no other
// file but the Style struct.
var defaultStyle Style

// DefaultStyle returns a copy of the default recipes: the maps are copied
// too, so editing the result never changes the defaults or the style of
// another caller.
//
//	s := kit.DefaultStyle()
//	s.Card.Root += " backdrop-blur-xl"
//	s.Button.Variants["glass"] = "bg-white/10 text-white hover:bg-white/20"
//	kit.StyleProvider{Style: &s, Children: app}
func DefaultStyle() Style { return deepCopy(defaultStyle) }

// deepCopy copies v, a recipe or a struct of recipes: strings, structs and
// the maps of strings of this package's recipes.
func deepCopy[T any](v T) T {
	out := reflect.New(reflect.TypeFor[T]()).Elem()
	copyValue(out, reflect.ValueOf(v))
	return out.Interface().(T)
}

func copyValue(dst, src reflect.Value) {
	switch src.Kind() {
	case reflect.Struct:
		for i := range src.NumField() {
			copyValue(dst.Field(i), src.Field(i))
		}
	case reflect.Map:
		if src.IsNil() {
			return
		}
		dst.Set(reflect.MakeMapWithSize(src.Type(), src.Len()))
		for it := src.MapRange(); it.Next(); {
			v := reflect.New(src.Type().Elem()).Elem()
			copyValue(v, it.Value())
			dst.SetMapIndex(it.Key(), v)
		}
	default:
		dst.Set(src)
	}
}

// styleCtx carries the active style to the components of a subtree.
var styleCtx = tela.CreateContext(&defaultStyle)

// StyleProvider gives Style to the kit components below it, in place of the
// defaults: the style of a subtree, like h.Scheme is its color scheme.
// Providers nest; the nearest one wins. A nil Style provides the defaults.
//
//	s := kit.DefaultStyle()
//	s.Button.Variants["glass"] = "bg-white/10 text-white backdrop-blur hover:bg-white/20"
//	return kit.StyleProvider{Style: &s, Children: Dashboard{}}
//
// Give it a style once, for example from tela.UseMemo or a package
// variable, and do not edit that style afterwards: the components below
// rebuild when the pointer changes, not when what it points to does.
type StyleProvider struct {
	Style    *Style
	Children tela.Component
}

// Build provides the style to the children.
func (p StyleProvider) Build() tela.Node {
	s := p.Style
	if s == nil {
		s = &defaultStyle
	}
	if p.Children == nil {
		return styleCtx.Provider(s)
	}
	return styleCtx.Provider(s, p.Children)
}

// UseStyle returns the style the component is under: the nearest
// [StyleProvider]'s, else the defaults. It is a hook. The result is
// shared: read it, and copy it with a recipe's own fields before changing
// anything; to make a style of your own start from [DefaultStyle].
//
// The kit components call it; call it yourself to build components that
// follow the active style:
//
//	func (b Banner) Build() tela.Node {
//	    s := kit.UseStyle()
//	    return h.Div(cls(s.Card.Root, "bg-accent"), b.Children)
//	}
func UseStyle() *Style { return tela.UseContext(styleCtx) }

// devMode reports whether Tela's dev mode is on at the call: the mode of
// the tree being built, else the process default (tela.DevMode). It is a
// lookup, so a build may ask it freely.
func devMode() bool { return tela.DevMode() }

// pick returns the classes recipe[k] defines for the Variant, Size or other
// name k of a component's field, or those of the default (the empty name)
// when k is the default. A name the active recipe does not define panics
// in dev mode, naming the ones it does and the closest, and falls back to
// the default otherwise.
func pick[K ~string](component, field string, recipe map[K]string, k K) string {
	if c, ok := recipe[k]; ok {
		return c
	}
	if k != "" && devMode() {
		names := make([]string, 0, len(recipe))
		for n := range recipe {
			names = append(names, string(n))
		}
		sort.Strings(names)
		msg := fmt.Sprintf("kit: %s.%s %q is not defined by the active style, which defines %s", component, field, string(k), quoteNames(names))
		if c := closest(string(k), names); c != "" {
			msg += fmt.Sprintf("; did you mean %q?", c)
		} else {
			msg += fmt.Sprintf("; add it to Style.%s.%ss (see kit.DefaultStyle) or use one of them", component, field)
		}
		panic(msg)
	}
	return recipe[""]
}

// quoteNames lists names for a message, writing the empty name as Default.
func quoteNames(names []string) string {
	q := make([]string, len(names))
	for i, n := range names {
		if n == "" {
			q[i] = "Default"
		} else {
			q[i] = `"` + n + `"`
		}
	}
	return strings.Join(q, ", ")
}

// closest returns the name of list nearest to s by edit distance, or "" when
// none is near enough to be a typo.
func closest(s string, list []string) string {
	best, bestD := "", max(2, len(s)/3)+1
	for _, c := range list {
		if c == "" {
			continue
		}
		if d := editDistance(strings.ToLower(s), strings.ToLower(c)); d < bestD {
			best, bestD = c, d
		}
	}
	return best
}

func editDistance(a, b string) int {
	prev := make([]int, len(b)+1)
	cur := make([]int, len(b)+1)
	for j := range prev {
		prev[j] = j
	}
	for i := 1; i <= len(a); i++ {
		cur[0] = i
		for j := 1; j <= len(b); j++ {
			cost := 1
			if a[i-1] == b[j-1] {
				cost = 0
			}
			cur[j] = min(prev[j]+1, cur[j-1]+1, prev[j-1]+cost)
		}
		prev, cur = cur, prev
	}
	return prev[len(b)]
}

// icon draws a slot icon with the recipe's classes, or nothing when the
// slot is empty.
func icon(ic tela.Icon, class ...string) tela.Child {
	if ic == (tela.Icon{}) {
		return nil
	}
	return h.Icon(ic, cls(class...))
}

// orientationOr returns o, or def for the zero value.
func orientationOr(o, def Orientation) Orientation {
	if o == "" {
		return def
	}
	return o
}

// when returns class if cond is true, else no class.
func when(cond bool, class string) string {
	if cond {
		return class
	}
	return ""
}

// cls is the class prop of a kit element: its parts joined by joinClass.
func cls(parts ...string) tela.Prop { return h.Class(joinClass(parts...)) }

// joinClass joins class strings with a space, skipping the empty ones, and
// puts every class that has a variant (hover:, focus-visible:, dark:,
// aria-invalid:...) after every class that has none, keeping the order
// within each group. Tela applies classes in order, last wins, variants
// included (ARCHITECTURE §10), while Tailwind's stylesheet emits variant
// utilities after the plain ones, so in CSS a matching variant beats a plain
// class wherever each is written. shadcn's recipes rely on that: the root's
// focus-visible:border-ring must beat the outline variant's border-border.
// Joining in two groups gives a recipe the stylesheet's precedence, and an
// instance Class, joined last, still overrides within its group.
func joinClass(parts ...string) string {
	var plain, variant []string
	for _, p := range parts {
		for _, c := range strings.Fields(p) {
			if hasVariant(c) {
				variant = append(variant, c)
			} else {
				plain = append(plain, c)
			}
		}
	}
	return strings.Join(append(plain, variant...), " ")
}

// hasVariant reports whether a class has a variant prefix: a colon outside
// square brackets and parentheses (bg-[url(a:b)] and [clip-path:...] have
// none).
func hasVariant(class string) bool {
	depth := 0
	for _, r := range class {
		switch r {
		case '[', '(':
			depth++
		case ']', ')':
			depth--
		case ':':
			if depth == 0 {
				return true
			}
		}
	}
	return false
}
