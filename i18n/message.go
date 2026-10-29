package i18n

import (
	"fmt"
	"math"
	"reflect"
	"strconv"
	"strings"
	"time"

	"golang.org/x/text/feature/plural"
)

// T translates a message of the current locale and fills in its
// arguments; see [Formatter.T].
//
// T is not a hook: it does not make a component build again when the
// locale changes. Use [UseT] in components, and T in event handlers,
// goroutines and code outside the tree.
func T(key string, args ...any) string { return Formatter{}.T(key, args...) }

// T returns the message key for the Formatter's locale, with its
// arguments filled in. args are name/value pairs, like the arguments of
// log/slog:
//
//	t("hello", "name", "Ana")                  // "Hello, {name}!"
//	t("files", "count", 3)                     // "{count, plural, one {# file} other {# files}}"
//
// The catalog of the locale is searched first, then its parents (es-MX,
// es) and the fallback locale ([SetFallback]; "en" by
// default). A key that is in none is returned as is, and reported once in
// dev mode (Tela's, on unless the program is a release build; TELA_DEV=0
// turns it off); so is an argument the message uses but args does not
// have (the message then shows its {name}). Numbers, dates (time.Time) and
// plurals are formatted for the locale.
func (f Formatter) T(key string, args ...any) string {
	d := f.loc()
	st := current()
	for _, c := range st.chain(d) {
		m, ok := c.msgs[key]
		if !ok {
			continue
		}
		ld := d
		if c.loc.lang != d.lang {
			// The message is in another language (the fallback's):
			// its plurals and numbers follow that language.
			ld = c.loc
		}
		return evaluate(ld, key, m, args)
	}
	devReportMissing(st, d, key)
	return key
}

// numVal is a number argument: the original value for the number
// formatter (exact for integers) and its float64 value.
type numVal struct {
	orig any
	f    float64
}

func toNumber(v any) (numVal, bool) {
	switch x := v.(type) {
	case int:
		return numVal{int64(x), float64(x)}, true
	case int8:
		return numVal{int64(x), float64(x)}, true
	case int16:
		return numVal{int64(x), float64(x)}, true
	case int32:
		return numVal{int64(x), float64(x)}, true
	case int64:
		return numVal{x, float64(x)}, true
	case uint:
		return numVal{uint64(x), float64(x)}, true
	case uint8:
		return numVal{uint64(x), float64(x)}, true
	case uint16:
		return numVal{uint64(x), float64(x)}, true
	case uint32:
		return numVal{uint64(x), float64(x)}, true
	case uint64:
		return numVal{x, float64(x)}, true
	case float32:
		return numVal{float64(x), float64(x)}, true
	case float64:
		return numVal{x, x}, true
	}
	// Named numeric types (type Count int).
	rv := reflect.ValueOf(v)
	switch rv.Kind() {
	case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64:
		return numVal{rv.Int(), float64(rv.Int())}, true
	case reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64, reflect.Uintptr:
		return numVal{rv.Uint(), float64(rv.Uint())}, true
	case reflect.Float32, reflect.Float64:
		return numVal{rv.Float(), rv.Float()}, true
	}
	return numVal{}, false
}

// evaluator formats one message.
type evaluator struct {
	f    Formatter
	args []any
	b    strings.Builder
	// pound is the plural a # refers to.
	pound *poundVal
	// bad is the first problem found, reported in dev mode.
	bad []string
}

type poundVal struct {
	n numVal // the number minus the offset
}

func evaluate(d *locData, key string, m []node, args []any) string {
	e := &evaluator{f: Formatter{d}, args: args}
	if len(args)%2 != 0 && devMode() {
		devWarn(fmt.Sprintf("tela: i18n: T(%q, ...): odd number of arguments; pass name/value pairs, as in T(%q, \"name\", value)", key, key))
	}
	e.nodes(m)
	if len(e.bad) > 0 && devMode() {
		for _, s := range e.bad {
			devWarnOnce(key+"\x00"+d.name+"\x00"+s, "tela: i18n: message "+strconv.Quote(key)+" in "+d.name+": "+s)
		}
	}
	return e.b.String()
}

func (e *evaluator) lookup(name string) (any, bool) {
	for i := 0; i+1 < len(e.args); i += 2 {
		if k, ok := e.args[i].(string); ok && k == name {
			return e.args[i+1], true
		}
	}
	return nil, false
}

func (e *evaluator) problem(format string, args ...any) {
	e.bad = append(e.bad, fmt.Sprintf(format, args...))
}

func (e *evaluator) nodes(ns []node) {
	for i := range ns {
		n := &ns[i]
		switch n.kind {
		case nodeText:
			e.b.WriteString(n.text)
		case nodePound:
			if e.pound != nil {
				e.b.WriteString(e.f.formatNumber(e.pound.n.orig, numberConfig{minFrac: -1, maxFrac: -1}))
			} else {
				e.b.WriteByte('#')
			}
		case nodeArg:
			e.arg(n)
		case nodePlural:
			e.plural(n)
		case nodeSelect:
			e.selectCase(n)
		}
	}
}

func (e *evaluator) arg(n *node) {
	v, ok := e.lookup(n.name)
	if !ok {
		e.problem("argument %q is not passed; add %q, value to the arguments of T", n.name, n.name)
		e.b.WriteString("{" + n.name + "}")
		return
	}
	switch n.typ {
	case argSimple:
		e.b.WriteString(e.simple(v))
	case argNumber:
		nv, ok := toNumber(v)
		if !ok {
			e.problem("argument %q must be a number for {%s, number}, got %T", n.name, n.name, v)
			e.b.WriteString(fmt.Sprint(v))
			return
		}
		c := numberConfig{minFrac: -1, maxFrac: -1}
		switch n.style {
		case "integer":
			c.maxFrac = 0
		case "percent":
			c.percent = true
		}
		e.b.WriteString(e.f.formatNumber(nv.orig, c))
	case argDate, argTime:
		t, ok := v.(time.Time)
		if !ok {
			e.problem("argument %q must be a time.Time for {%s, %s}, got %T", n.name, n.name, map[argType]string{argDate: "date", argTime: "time"}[n.typ], v)
			e.b.WriteString(fmt.Sprint(v))
			return
		}
		style := Medium
		switch n.style {
		case "short":
			style = Short
		case "long":
			style = Long
		case "full":
			style = Full
		}
		if n.typ == argDate {
			e.b.WriteString(e.f.Date(t, style))
		} else {
			e.b.WriteString(e.f.Time(t, style))
		}
	}
}

// simple formats an argument of {name}: strings as they are, numbers for
// the locale, times as a medium date and time, anything else with fmt.
func (e *evaluator) simple(v any) string {
	switch x := v.(type) {
	case string:
		return x
	case time.Time:
		return e.f.DateTime(x, Medium, Medium)
	case fmt.Stringer:
		return x.String()
	case error:
		return x.Error()
	}
	if nv, ok := toNumber(v); ok {
		return e.f.formatNumber(nv.orig, numberConfig{minFrac: -1, maxFrac: -1})
	}
	return fmt.Sprint(v)
}

func (e *evaluator) plural(n *node) {
	v, ok := e.lookup(n.name)
	if !ok {
		e.problem("argument %q is not passed; add %q, count to the arguments of T", n.name, n.name)
		e.b.WriteString("{" + n.name + "}")
		return
	}
	nv, ok := toNumber(v)
	if !ok {
		e.problem("argument %q must be a number for {%s, plural}, got %T", n.name, n.name, v)
		e.selectBody(n.cases[n.other].body, nil)
		return
	}
	// Explicit values (=N) win and compare with the number itself.
	for i := range n.cases {
		if c := &n.cases[i]; c.exact && c.value == nv.f {
			e.selectBody(c.body, e.poundOf(nv, n.offset))
			return
		}
	}
	shifted := e.poundOf(nv, n.offset)
	form := e.pluralForm(n.ordinal, shifted.n.f)
	for i := range n.cases {
		if c := &n.cases[i]; !c.exact && c.selector == form {
			e.selectBody(c.body, shifted)
			return
		}
	}
	e.selectBody(n.cases[n.other].body, shifted)
}

// poundOf is the value of # for a number and an offset.
func (e *evaluator) poundOf(nv numVal, offset float64) *poundVal {
	if offset == 0 {
		return &poundVal{nv}
	}
	f := nv.f - offset
	return &poundVal{numVal{f, f}}
}

func (e *evaluator) selectBody(body []node, pound *poundVal) {
	saved := e.pound
	if pound != nil {
		e.pound = pound
	}
	e.nodes(body)
	e.pound = saved
}

func (e *evaluator) selectCase(n *node) {
	v, ok := e.lookup(n.name)
	if !ok {
		e.problem("argument %q is not passed; add %q, value to the arguments of T", n.name, n.name)
		e.b.WriteString("{" + n.name + "}")
		return
	}
	var sel string
	switch x := v.(type) {
	case string:
		sel = x
	case bool:
		sel = strconv.FormatBool(x)
	case fmt.Stringer:
		sel = x.String()
	default:
		sel = fmt.Sprint(v)
	}
	for i := range n.cases {
		if n.cases[i].selector == sel {
			e.nodes(n.cases[i].body)
			return
		}
	}
	e.nodes(n.cases[n.other].body)
}

// pluralForm returns the CLDR plural category ("one", "few"...) of x for
// the evaluator's locale. The operands come from the number as the
// default number format shows it (at most 3 fraction digits), as ICU
// does.
func (e *evaluator) pluralForm(ordinal bool, x float64) string {
	if math.IsNaN(x) || math.IsInf(x, 0) {
		return "other"
	}
	s := strconv.FormatFloat(math.Abs(x), 'f', 3, 64)
	s = strings.TrimRight(s, "0")
	s = strings.TrimSuffix(s, ".")
	intPart, frac, _ := strings.Cut(s, ".")
	digits := make([]byte, 0, len(intPart)+len(frac))
	for i := 0; i < len(intPart); i++ {
		digits = append(digits, intPart[i]-'0')
	}
	for i := 0; i < len(frac); i++ {
		digits = append(digits, frac[i]-'0')
	}
	rules := plural.Cardinal
	if ordinal {
		rules = plural.Ordinal
	}
	switch rules.MatchDigits(e.f.d.tag, digits, len(intPart), len(frac)) {
	case plural.Zero:
		return "zero"
	case plural.One:
		return "one"
	case plural.Two:
		return "two"
	case plural.Few:
		return "few"
	case plural.Many:
		return "many"
	}
	return "other"
}
