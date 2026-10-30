package style

import (
	"errors"
	"fmt"
	"slices"
	"strconv"
	"strings"

	"github.com/kuronosu/tela/internal/gfx"
)

// Theme variables inside arbitrary values (ADR-202).
//
// Tailwind v4 exposes the theme as CSS variables, and an arbitrary value
// may use them: bg-[var(--color-primary)], rounded-[var(--radius)],
// w-[calc(var(--spacing)*4)], shadow-[0_0_0_2px_var(--color-ring)],
// bg-[conic-gradient(var(--color-primary)_72%,transparent_0)]. Tela has no
// CSS variables, so the parser substitutes the theme's own variables in
// the text of the value before the value is parsed:
//
//	var(--color-<name>)     a palette color or a semantic token
//	var(--radius)           the "lg" radius (shadcn/ui's --radius)
//	var(--radius-<name>)    a radius of the theme
//	var(--spacing)          the spacing unit
//
// each with the optional fallback var(--x, fallback), used when the theme
// does not have the name. Any other variable is an error.
//
// A semantic token has a light and a dark value (ADR-089), but a compiled
// class is shared by every color scheme. A class that uses one is
// therefore parsed twice, once per scheme, and its op picks the result by
// the scheme of the element (schemeOp), exactly as bg-primary resolves.
// Palette colors, radii and the spacing do not depend on the scheme.

// rgbaFunc is the internal color function that carries a theme color to
// the color parser without losing precision: tela-rgba(r g b a), floats of
// the extended sRGB range, as in gfx.Color.
const rgbaFunc = "tela-rgba"

// formatColor writes c as the text of a tela-rgba() color.
func formatColor(c gfx.Color) string {
	f := func(v float32) string { return strconv.FormatFloat(float64(v), 'f', -1, 32) }
	return rgbaFunc + "(" + f(c.R) + " " + f(c.G) + " " + f(c.B) + " " + f(c.A) + ")"
}

// parseRGBAFunc parses the inside of a tela-rgba() color.
func parseRGBAFunc(inner string) (gfx.Color, error) {
	f := strings.Fields(inner)
	if len(f) != 4 {
		return gfx.Color{}, errors.New("not a color")
	}
	var v [4]float32
	for i, s := range f {
		x, err := strconv.ParseFloat(s, 32)
		if err != nil {
			return gfx.Color{}, errors.New("not a color")
		}
		v[i] = float32(x)
	}
	return gfx.Color{R: v[0], G: v[1], B: v[2], A: v[3]}, nil
}

// varError is an unknown name in a theme variable: the Reason of the
// InvalidClassError that resolveUtility builds, with the closest name.
type varError struct {
	msg        string
	name       string // the variable as written, "--color-nope"
	suggestion string // the closest variable, "--color-node", or ""
}

func (e *varError) Error() string { return e.msg }

// themeVarsHint lists the variables that are supported, for the error of
// the others.
const themeVarsHint = "var(--color-<token>), var(--radius), var(--radius-<size>) and var(--spacing)"

// substituteVars replaces the theme variables in s (the text of an
// arbitrary value, underscores already turned into spaces) with their
// values for c's color scheme. It sets c.schemeVar when a semantic token
// was used.
func (c *pctx) substituteVars(s string) (string, error) {
	if !strings.Contains(s, "var(") {
		return s, nil
	}
	return c.subVars(s, nil)
}

func isIdentByte(b byte) bool {
	return b >= 'a' && b <= 'z' || b >= 'A' && b <= 'Z' || b >= '0' && b <= '9' || b == '-' || b == '_'
}

// matchParen returns the index of the parenthesis that closes the one at
// s[open], or -1.
func matchParen(s string, open int) int {
	depth := 0
	for i := open; i < len(s); i++ {
		switch s[i] {
		case '(':
			depth++
		case ')':
			depth--
			if depth == 0 {
				return i
			}
		}
	}
	return -1
}

// subVars is substituteVars with the names of the functions that enclose
// s already (outer), so that a length is grouped when it lands in a
// calc(): calc(var(--spacing)*4) becomes calc((0.25rem)*4), which the
// calc parser reads as the product it is.
func (c *pctx) subVars(s string, outer []string) (string, error) {
	var b strings.Builder
	stack := slices.Clone(outer)
	for i := 0; i < len(s); {
		switch {
		case s[i] == '(':
			j := i
			for j > 0 && isIdentByte(s[j-1]) {
				j--
			}
			stack = append(stack, strings.ToLower(s[j:i]))
			b.WriteByte('(')
			i++
		case s[i] == ')':
			if len(stack) > len(outer) {
				stack = stack[:len(stack)-1]
			}
			b.WriteByte(')')
			i++
		case strings.HasPrefix(s[i:], "var(") && (i == 0 || !isIdentByte(s[i-1])):
			end := matchParen(s, i+3)
			if end < 0 {
				return "", errors.New("var( is not closed")
			}
			text, isLen, err := c.resolveVar(s[i+4:end], stack)
			if err != nil {
				return "", err
			}
			if isLen && len(stack) > 0 && slices.Contains([]string{"calc", "min", "max", "clamp", ""}, stack[len(stack)-1]) {
				text = "(" + text + ")"
			}
			b.WriteString(text)
			i = end + 1
		default:
			b.WriteByte(s[i])
			i++
		}
	}
	return b.String(), nil
}

// resolveVar resolves the inside of a var(): "--name" or "--name, fallback".
// isLen reports a length, which the caller may have to group.
func (c *pctx) resolveVar(body string, stack []string) (text string, isLen bool, err error) {
	parts := splitTop(body, ',')
	name := parts[0]
	hasFallback := len(parts) > 1
	fallback := strings.Join(parts[1:], ",")
	name = strings.TrimSpace(name)
	if !strings.HasPrefix(name, "--") {
		return "", false, fmt.Errorf("var(%s) needs the name of a variable, such as var(--color-primary)", name)
	}
	text, isLen, found, err := c.themeVar(name)
	if err != nil {
		var ve *varError
		if hasFallback && errors.As(err, &ve) {
			found = false // an unknown name of the theme: the fallback applies
		} else {
			return "", false, err
		}
	}
	if found {
		return text, isLen, nil
	}
	if !hasFallback {
		return "", false, fmt.Errorf("the theme does not have the variable %s", name)
	}
	fb := strings.TrimSpace(fallback)
	if fb == "" {
		return "", false, fmt.Errorf("var(%s,) has an empty fallback", name)
	}
	text, err = c.subVars(fb, stack)
	return text, false, err
}

// themeVar returns the value of a variable of the theme. found is false
// for a name the theme does not define in a namespace it has (the error
// carries the closest name); an unsupported variable is an error.
func (c *pctx) themeVar(name string) (text string, isLen, found bool, err error) {
	th := c.th
	switch {
	case name == "--spacing":
		return th.Spacing.String(), true, true, nil
	case name == "--radius":
		if l, ok := th.Radius["lg"]; ok {
			return l.String(), true, true, nil
		}
		return "", false, false, &varError{msg: "the theme has no --radius (its radius scale has no \"lg\")", name: name}
	case strings.HasPrefix(name, "--radius-"):
		k := name[len("--radius-"):]
		if l, ok := th.Radius[k]; ok && k != "DEFAULT" {
			return l.String(), true, true, nil
		}
		return "", false, false, c.unknownVar(name, "--radius-", k, "radius", th.Keys(NamespaceRadius))
	case strings.HasPrefix(name, "--color-"):
		k := name[len("--color-"):]
		if col, ok := th.Colors[k]; ok {
			return formatColor(col), false, true, nil
		}
		if _, ok := th.Tokens[k]; ok {
			c.schemeVar = true
			col, _ := th.TokenColor(k, c.dark)
			return formatColor(col), false, true, nil
		}
		keys := append(th.Keys(NamespaceColor), th.TokenNames()...)
		return "", false, false, c.unknownVar(name, "--color-", k, "color", keys)
	}
	return "", false, false, unsupportedErr("CSS variables are not supported except the theme's own: " + themeVarsHint +
		"; write a literal value or use a theme class instead of " + name)
}

func (c *pctx) unknownVar(name, prefix, key, what string, keys []string) error {
	e := &varError{name: name, msg: fmt.Sprintf("unknown theme %s %q in var(%s)", what, key, name)}
	if s := closest(key, keys); s != "" {
		e.suggestion = prefix + s
		e.msg += fmt.Sprintf("; did you mean var(%s)?", e.suggestion)
	}
	return e
}

// schemeOp combines the ops a class compiles to for the light and the dark
// color scheme into one that picks by the scheme of the element it runs
// on.
func schemeOp(light, dark Op) Op {
	o := light
	if la, da := light.apply, dark.apply; la != nil && da != nil {
		o.apply = func(e *env) {
			if e.dark {
				da(e)
			} else {
				la(e)
			}
		}
	}
	if lp, dp := light.post, dark.post; lp != nil && dp != nil {
		o.post = func(e *env) {
			if e.dark {
				dp(e)
			} else {
				lp(e)
			}
		}
	}
	return o
}
