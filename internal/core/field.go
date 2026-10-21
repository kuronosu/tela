package core

import (
	"fmt"
	"math"
	"net/url"
	"os"
	"runtime"
	"strings"
	"time"

	"github.com/kuronosu/tela/internal/gfx"
	"github.com/kuronosu/tela/internal/layout"
	"github.com/kuronosu/tela/internal/style"
	"github.com/kuronosu/tela/internal/text"
	"github.com/kuronosu/tela/internal/text/edit"
)

// Text fields (ARCHITECTURE sections 11 and 14, ADR-069): Input (one
// line) and Textarea (several lines) edit a plain text value with
// internal/text/edit, which this file connects to the rest of core. The
// reference for every behavior is Chromium.
//
// # Box and layout
//
// A text field is a leaf box (modeField) that owns a field: its editor,
// the text.Layout of the editor's displayed text (rebuilt only when
// Editor.DisplayVersion or the text style changes, broken again only
// when the width changes) and that of its placeholder. Its intrinsic size
// is Chromium's: an Input is ceil(19 A + M) wide (size=20: A is the
// primary font's average character width, OS/2 xAvgCharWidth rounded to
// a whole pixel when that makes it larger, M its bounding box width
// rounded) and one line tall; a Textarea is ceil(A cols) wide (20
// columns) plus the classic scrollbar's 15 px when scrollbars are classic,
// and Rows lines tall (2 by default). field-sizing-content sizes both to
// their text (or placeholder), at least the 1 px caret wide. The line of an Input
// is centered vertically in its content box and is its baseline; a
// Textarea's baseline is its bottom margin edge (a scroll container).
//
// An Input clips its text to its content box horizontally and scrolls it
// itself (field.scrollX) to keep the caret visible; a Textarea is a
// scroll container (overflow auto, scroll.go) whose extents come from its
// text, so typing never lays the tree out: a keystroke shapes the field's
// own text and repaints its layer (field.go's layerField, layer.go), and
// only field-sizing-content fields are laid out again.
//
// # Values
//
// A field is uncontrolled (DefaultValue: the initial value, later ones
// ignored), controlled (Value: after every event the field shows Value
// again unless the app changed it, React's restoreControlledState) or
// bound to a *Value[string] (BindValue: the field writes it on the UI
// thread at once, without the update queue, and follows its changes,
// without building the component). A programmatic change (a new Value,
// a bound Value set from elsewhere) puts the caret at the end and clears
// the undo history (edit.SetText with ResetHistory), as browsers do when
// a script sets input.value; an equal value changes nothing.
//
// # Events
//
// An edit fires OnBeforeInput (cancelable, except during a composition
// and for an IME's deletion around the caret),
// then OnChange with the new value (React's onChange: every edit, the
// DOM's input event), each followed by the flush of a discrete event; the
// field's value is then restored if it is controlled. IME compositions
// fire OnCompositionStart, OnCompositionUpdate and OnCompositionEnd in
// the UI Events order (beforeinput, compositionupdate, input during the
// composition; compositionend after the commit), and OnChange during the
// composition with IsComposing set, as Chromium does. OnSelect fires
// when the user makes or changes a non-empty selection, and when code
// changes the selection (ElementRef.Select, SetSelectionRange).
//
// The keyboard, pointer, IME, clipboard and caret code live in
// fieldkeys.go and fieldime.go, the painting in fieldpaint.go, forms and
// labels in form.go.

// fieldType is the type of an Input (Type).
type fieldType uint8

const (
	fieldText fieldType = iota
	fieldPassword
	fieldEmail
	fieldSearch
	fieldURL
	fieldTel
	fieldNumber
)

var fieldTypeNames = [...]string{"text", "password", "email", "search", "url", "tel", "number"}

// buttonTypeNames are the types of a Button.
var buttonTypeNames = [...]string{"submit", "button", "reset"}

// formProps are the props of the form elements (Input, Textarea, the Type
// of a Button, the For of a Label), nil on a desc without any.
type formProps struct {
	typ          string
	hasType      bool
	value        string
	hasValue     bool
	bind         *Value[string]
	defValue     string
	placeholder  string
	readOnly     bool
	maxLength    int
	autoComplete string
	spellCheck   bool
	hasSpell     bool
	name         string
	rows         int
	forID        string
	// set records which props were given, for the checks of Primitive.
	set uint32
}

// Bits of formProps.set.
const (
	fpType = 1 << iota
	fpValue
	fpDefault
	fpBind
	fpPlaceholder
	fpReadOnly
	fpMaxLength
	fpAutoComplete
	fpSpellCheck
	fpName
	fpRows
	fpFor
)

// fpFieldOnly are the props that only text fields take.
const fpFieldOnly = fpValue | fpDefault | fpBind | fpPlaceholder | fpReadOnly | fpMaxLength | fpAutoComplete | fpSpellCheck

var fpNames = map[uint32]string{
	fpType: "Type", fpValue: "Value", fpDefault: "DefaultValue", fpBind: "BindValue", fpPlaceholder: "Placeholder",
	fpReadOnly: "ReadOnly", fpMaxLength: "MaxLength", fpAutoComplete: "AutoComplete", fpSpellCheck: "SpellCheck",
	fpName: "Name", fpRows: "Rows", fpFor: "For",
}

func formProp(name string, bit uint32, f func(fp *formProps)) Prop {
	return Prop{name: name, apply: func(d *desc) {
		if d.fp == nil {
			d.fp = &formProps{}
		}
		d.fp.set |= bit
		f(d.fp)
	}}
}

// FieldValue makes a text field controlled (h.Value): it shows s, and
// after every event it shows s again unless the app rebuilt it with a new
// value (React's controlled inputs). Give it an OnChange that stores the
// new value, or use Bind.
func FieldValue(s string) Prop {
	return formProp("Value", fpValue, func(fp *formProps) { fp.value, fp.hasValue = s, true })
}

// DefaultValue gives an uncontrolled text field its initial value; later
// values are ignored (React's defaultValue).
func DefaultValue(s string) Prop {
	return formProp("DefaultValue", fpDefault, func(fp *formProps) { fp.defValue = s })
}

// BindValue binds a text field to v: the field shows v's value, writes
// every edit to it at once (on the UI thread, without rebuilding the
// component that owns v unless something reads it in a build), and
// follows v.Set from anywhere.
func BindValue(v *Value[string]) Prop {
	if v == nil {
		panic("tela: BindValue(nil): pass the *tela.Value[string] returned by tela.UseValue")
	}
	return formProp("BindValue", fpBind, func(fp *formProps) { fp.bind = v })
}

// Bind makes a text field controlled by a state: FieldValue(text) plus an
// OnChange that calls set.Set with the new value.
func Bind(text string, set Setter[string]) Prop {
	if set.s == nil {
		panic("tela: Bind(text, zero Setter): pass the Setter returned by tela.UseState")
	}
	h := func(e ChangeEvent) { set.Set(e.Value) }
	return Prop{name: "Bind", apply: func(d *desc) {
		if d.fp == nil {
			d.fp = &formProps{}
		}
		d.fp.set |= fpValue
		d.fp.value, d.fp.hasValue = text, true
		d.events = append(d.events, EventProp{Name: "change", Handler: h, kind: evChange})
		d.evMask |= evChange.mask()
	}}
}

// FieldType sets the type of an Input ("text", "password", "email",
// "search", "url", "tel", "number") or of a Button ("submit", "button",
// "reset").
func FieldType(s string) Prop {
	return formProp("Type", fpType, func(fp *formProps) { fp.typ, fp.hasType = s, true })
}

// Rows sets the number of lines a Textarea is tall (2 by default).
func Rows(n int) Prop {
	return formProp("Rows", fpRows, func(fp *formProps) { fp.rows = max(n, 0) })
}

// Placeholder sets the text a text field shows while it is empty.
func Placeholder(s string) Prop {
	return formProp("Placeholder", fpPlaceholder, func(fp *formProps) { fp.placeholder = s })
}

// ReadOnly makes a text field read-only: focusable, selectable and
// copyable, but not editable (HTML readonly).
func ReadOnly(on bool) Prop {
	return formProp("ReadOnly", fpReadOnly, func(fp *formProps) { fp.readOnly = on })
}

// MaxLength limits what the user can type into a text field to n UTF-16
// code units (HTML maxlength); 0 or less means no limit.
func MaxLength(n int) Prop {
	return formProp("MaxLength", fpMaxLength, func(fp *formProps) { fp.maxLength = n })
}

// AutoComplete is HTML's autocomplete hint. It is accepted for
// familiarity; Tela has no autofill.
func AutoComplete(s string) Prop {
	return formProp("AutoComplete", fpAutoComplete, func(fp *formProps) { fp.autoComplete = s })
}

// SpellCheck is HTML's spellcheck hint. It is accepted and ignored: Tela
// has no spell checker.
func SpellCheck(on bool) Prop {
	return formProp("SpellCheck", fpSpellCheck, func(fp *formProps) { fp.spellCheck, fp.hasSpell = on, true })
}

// Name names a text field or a button in its form (HTML's name).
func Name(s string) Prop {
	return formProp("Name", fpName, func(fp *formProps) { fp.name = s })
}

// For makes a Label label the element whose ID is id.
func For(id string) Prop {
	return formProp("For", fpFor, func(fp *formProps) { fp.forID = id })
}

// ID gives a primitive an identifier, unique in its tree (h.Id), that
// Label's For refers to.
func ID(id string) Prop {
	return Prop{name: "ID", apply: func(d *desc) { d.id = id }}
}

// checkFormProps panics when form props are given to an element that
// does not take them (Primitive).
func checkFormProps(kind style.Kind, d *desc) {
	fp := d.fp
	field := kind.IsTextField()
	for bit := uint32(1); bit <= fpFor; bit <<= 1 {
		if fp.set&bit == 0 {
			continue
		}
		ok := true
		switch {
		case bit&fpFieldOnly != 0:
			ok = field
		case bit == fpType:
			ok = kind == style.KindInput || kind == style.KindButton
		case bit == fpName:
			ok = field || kind == style.KindButton
		case bit == fpRows:
			ok = kind == style.KindTextarea
		case bit == fpFor:
			ok = kind == style.KindLabel
		}
		if !ok {
			panic(fmt.Sprintf("tela: %s on <%v>: %s", fpNames[bit], kind, fpWhere(bit)))
		}
	}
	if fp.hasValue && fp.bind != nil {
		panic(fmt.Sprintf("tela: Value and BindValue on one <%v>: use one of them", kind))
	}
	if fp.hasType {
		names := fieldTypeNames[:]
		if kind == style.KindButton {
			names = buttonTypeNames[:]
		}
		for _, n := range names {
			if fp.typ == n {
				return
			}
		}
		if devAny.Load() || os.Getenv(EnvDev) == "1" {
			msg := fmt.Sprintf("tela: Type(%q) on <%v>: unknown type; use one of %s", fp.typ, kind, strings.Join(names, ", "))
			if s := closestName(fp.typ, names); s != "" {
				msg = fmt.Sprintf("tela: Type(%q) on <%v>: unknown type; did you mean %q? (types: %s)", fp.typ, kind, s, strings.Join(names, ", "))
			}
			panic(msg)
		}
	}
}

// fpWhere says which elements take a form prop.
func fpWhere(bit uint32) string {
	switch {
	case bit&fpFieldOnly != 0:
		return "only Input and Textarea take it"
	case bit == fpType:
		return "only Input (text, password, email...) and Button (submit, button, reset) take it"
	case bit == fpName:
		return "only Input, Textarea and Button take it"
	case bit == fpRows:
		return "only Textarea takes it"
	}
	return "only Label takes it (give the labeled element an Id)"
}

// closestName returns the name closest to s, or "".
func closestName(s string, names []string) string {
	best, bestD := "", 1<<30
	for _, n := range names {
		if d := editDistance(strings.ToLower(s), n); d < bestD {
			best, bestD = n, d
		}
	}
	if bestD <= max(2, len(best)/3) {
		return best
	}
	return ""
}

// fieldTypeOf returns the type of an Input node (text by default and for
// unknown types outside dev mode).
func fieldTypeOf(d *desc) fieldType {
	if d.fp != nil && d.fp.hasType && d.elem == style.KindInput {
		for i, n := range fieldTypeNames {
			if d.fp.typ == n {
				return fieldType(i)
			}
		}
	}
	return fieldText
}

// buttonType returns the type of a Button: submit (the default), button
// or reset.
func buttonType(d *desc) string {
	if d.fp != nil && d.fp.hasType {
		for _, n := range buttonTypeNames {
			if d.fp.typ == n {
				return n
			}
		}
	}
	return "submit"
}

// ---------------------------------------------------------------------
// The field.

// field is the state of a text field box.
type field struct {
	t     *Tree
	el    *Element
	bx    *Box
	typ   fieldType
	multi bool
	ed    *edit.Editor

	// bind is the bound Value (BindValue) and bobs its subscription.
	bind *Value[string]
	bobs *observer
	// edited reports that the value was last changed by a user edit
	// (HTML's rule for tooLong); raw is RawValue after the last event,
	// to fire OnChange only when it changed.
	edited bool
	raw    string

	// lay is the layout of the editor's Display, built from layVer with
	// the styles ts and ps; brokeAt is the width its lines were broken
	// (or, for an Input, aligned) at.
	lay     *text.Layout
	layVer  uint64
	ts      text.TextStyle
	ps      text.ParagraphStyle
	brokeAt float32
	// ph is the placeholder's layout, phKey what it was built from.
	ph      *text.Layout
	phText  string
	phTS    text.TextStyle
	phPS    text.ParagraphStyle
	phBroke float32
	// empty is a layout of no text in the field's style (its line
	// height).
	empty            *text.Layout
	emptyTS          text.TextStyle
	emptyPS          text.ParagraphStyle
	scrollX          float32 // an Input's horizontal scroll
	paintDirty       bool    // its layer must be recorded again
	selStart, selEnd int     // the selection of the last select check
}

// textField returns the field of el, or nil.
func textField(el *Element) *field {
	if el == nil || el.kind != elemPrimitive || el.box == nil {
		return nil
	}
	return el.box.fld
}

// alive reports whether f is still the field of a mounted element.
func (f *field) alive() bool { return f.el.mounted && f.bx.fld == f }

// props returns the form props of the field's node.
func (f *field) props() *formProps {
	if fp := f.el.node.d.fp; fp != nil {
		return fp
	}
	return &noFormProps
}

var noFormProps formProps

// readOnly reports whether the user cannot edit the field: ReadOnly or
// disabled.
func (f *field) readOnly() bool {
	return f.props().readOnly || f.el.istate&style.StateDisabled != 0 || outermostDisabled(f.el) != nil
}

// options returns the editor options for the field's props.
func (f *field) options() edit.Options {
	fp := f.props()
	o := edit.Options{
		Multiline: f.multi,
		Password:  f.typ == fieldPassword,
		ReadOnly:  fp.readOnly,
		MaxLength: fp.maxLength,
		Now:       f.t.clock,
	}
	if runtime.GOOS == "windows" {
		o.Words = edit.WordsWindows
	}
	return o
}

// clock is the tree's clock for the editor's undo grouping: the latest
// time of the backend clock it saw.
func (t *Tree) clock() time.Duration { return t.scr.now }

// mountField creates the field of a text field being mounted.
func (t *Tree) mountField(el *Element) {
	d := el.node.d
	f := &field{t: t, el: el, bx: el.box, multi: d.elem == style.KindTextarea, typ: fieldTypeOf(d), brokeAt: nan32()}
	el.box.fld = f
	fp := f.props()
	initial := fp.defValue
	switch {
	case fp.hasValue:
		initial = fp.value
	case fp.bind != nil:
		initial = *fp.bind.cur.Load()
	}
	f.ed = edit.New(initial, f.options())
	// Chromium puts the caret at the start of an initial value.
	f.ed.Select(0, 0)
	f.raw = f.ed.RawValue()
	f.selStart, f.selEnd = 0, 0
	f.bindTo(fp.bind)
	t.fieldStates(f)
}

// updateField brings the field of el up to date with its new node.
func (t *Tree) updateField(el *Element, old *desc) {
	f := el.box.fld
	if f == nil {
		return
	}
	d := el.node.d
	fp := f.props()
	var ofp *formProps
	if old.fp != nil {
		ofp = old.fp
	} else {
		ofp = &noFormProps
	}
	if typ := fieldTypeOf(d); typ != f.typ {
		f.typ = typ
	}
	if o := f.options(); !sameOptions(o, f.ed.Options()) {
		composing := f.ed.Composing()
		r := f.ed.SetOptions(o)
		if composing && !f.ed.Composing() {
			t.ime.reset = true
		}
		f.edited = f.edited && !r.ValueChanged
		t.fieldChanged(f, r)
	}
	if ofp.placeholder != fp.placeholder || ofp.rows != fp.rows {
		f.ph = nil
		t.fieldResized(f)
		f.markPaint()
	}
	f.bindTo(fp.bind)
	if fp.hasValue {
		t.setFieldValue(f, fp.value)
	}
	t.fieldStates(f)
}

// sameOptions reports whether two editor options are the same for the
// editor (the clock is always the tree's).
func sameOptions(a, b edit.Options) bool {
	return a.Multiline == b.Multiline && a.Password == b.Password && a.ReadOnly == b.ReadOnly &&
		a.MaxLength == b.MaxLength && a.Words == b.Words
}

// unmountField ends what the field of el holds on the tree.
func (t *Tree) unmountField(el *Element) {
	f := el.box.fld
	if f == nil {
		return
	}
	dropObserver(f.bobs)
	if t.caret.f == f {
		t.caret = caretState{}
	}
	if t.in.fdrag != nil && t.in.fdrag.f == f {
		t.in.fdrag = nil
	}
}

// bindTo subscribes the field to v (BindValue), or ends its
// subscription.
func (f *field) bindTo(v *Value[string]) {
	if v == f.bind {
		return
	}
	t := f.t
	f.bind = v
	if v == nil {
		dropObserver(f.bobs)
		return
	}
	if f.bobs == nil {
		f.bobs = &observer{kind: obsField, box: f.bx}
	}
	t.resubscribe(f.bobs, []*valueCore{&v.valueCore})
	t.setFieldValue(f, *v.cur.Load())
}

// bindChanged follows a change of the bound Value (obsField).
func (t *Tree) bindChanged(b *Box) {
	f := b.fld
	if f == nil || f.bind == nil || !f.alive() {
		return
	}
	t.setFieldValue(f, *f.bind.cur.Load())
}

// setFieldValue sets the value of f programmatically (a controlled or
// bound value): nothing when it is the value shown (with the preedit of a
// composition), else the caret goes to the end and the history is
// cleared, and a composition in progress is canceled (the IME is reset).
func (t *Tree) setFieldValue(f *field, s string) {
	if s == f.ed.RawValue() {
		return
	}
	composing := f.ed.Composing()
	r := f.ed.SetText(s, edit.ResetHistory)
	if composing {
		t.ime.reset = true
	}
	f.edited = false
	f.raw = f.ed.RawValue()
	t.fieldChanged(f, r)
	f.checkSelect(false)
}

// restoreControlled shows the Value prop again after an event (React's
// restoreControlledState) when the app did not change it.
func (t *Tree) restoreControlled(f *field) {
	if !f.alive() {
		return
	}
	if fp := f.props(); fp.hasValue {
		t.setFieldValue(f, fp.value)
	}
}

// fieldChanged updates what depends on the editor after an operation:
// the layout, the size of a field sized to its content, a Textarea's
// extents, the caret in view, the states and the painting.
func (t *Tree) fieldChanged(f *field, r edit.Result) {
	if !r.ValueChanged && !r.SelectionChanged && !r.DisplayChanged {
		return
	}
	if r.DisplayChanged {
		t.fieldResized(f)
		t.fieldStates(f)
	}
	f.reveal()
	f.markPaint()
}

// fieldResized lays the field out again when its size depends on its
// content (field-sizing-content), and updates a Textarea's extents.
func (t *Tree) fieldResized(f *field) {
	if cs := f.bx.cs; cs != nil && cs.FieldSizing == style.FieldSizingContent {
		t.engine.MarkDirty(f.bx)
		t.dirty()
		return
	}
	if s := f.bx.scr; s != nil && f.multi && t.layoutCurrent() {
		t.extents(s)
	}
}

// markPaint asks for the field's layer to be recorded again in the next
// frame (a paint-only frame when nothing else changed).
func (f *field) markPaint() {
	t := f.t
	f.paintDirty = true
	t.lay.fieldDirty = true
	t.requestRepaint()
}

// fieldStates sets the read-only:, placeholder-shown: and invalid: states
// of f.
func (t *Tree) fieldStates(f *field) {
	el := f.el
	fp := f.props()
	ro := f.readOnly()
	raw := f.ed.RawValue()
	shown := fp.placeholder != "" && raw == ""
	invalid := f.typeMismatch(raw) ||
		f.edited && fp.maxLength > 0 && edit.UTF16Len(f.ed.Value()) > fp.maxLength
	t.setState(el, style.StateReadOnly, ro)
	t.setState(el, style.StatePlaceholderShown, shown)
	t.setState(el, style.StateInvalid, invalid)
}

// typeMismatch reports HTML's type mismatch of an email or url value
// (an empty value matches).
func (f *field) typeMismatch(v string) bool {
	if v == "" {
		return false
	}
	switch f.typ {
	case fieldEmail:
		return !validEmail(v)
	case fieldURL:
		return !validURL(v)
	}
	return false
}

// validEmail reports whether s is a valid e-mail address (HTML's
// definition, without the multiple attribute): a local part of
// atext and dots, "@", and a domain of labels of letters, digits and
// hyphens that neither start nor end with a hyphen.
func validEmail(s string) bool {
	at := strings.IndexByte(s, '@')
	if at <= 0 || at == len(s)-1 || strings.IndexByte(s[at+1:], '@') >= 0 {
		return false
	}
	for i := 0; i < at; i++ {
		c := s[i]
		if !(isAlnum(c) || strings.IndexByte(".!#$%&'*+/=?^_`{|}~-", c) >= 0) {
			return false
		}
	}
	for _, lab := range strings.Split(s[at+1:], ".") {
		if lab == "" || len(lab) > 63 || lab[0] == '-' || lab[len(lab)-1] == '-' {
			return false
		}
		for i := 0; i < len(lab); i++ {
			if !isAlnum(lab[i]) && lab[i] != '-' {
				return false
			}
		}
	}
	return true
}

func isAlnum(c byte) bool {
	return c >= 'a' && c <= 'z' || c >= 'A' && c <= 'Z' || c >= '0' && c <= '9'
}

// validURL reports whether s is a valid absolute URL, approximating the
// URL Standard as Chromium applies it: a scheme, and a host for the
// special schemes.
func validURL(s string) bool {
	u, err := url.Parse(s)
	if err != nil || u.Scheme == "" {
		return false
	}
	c := u.Scheme[0]
	if !(c >= 'a' && c <= 'z' || c >= 'A' && c <= 'Z') {
		return false
	}
	switch strings.ToLower(u.Scheme) {
	case "http", "https", "ws", "wss", "ftp":
		return u.Host != ""
	}
	return true
}

// fieldRestyled follows a new style of f: a change of what its text is
// laid out with lays the field out again; anything else only repaints.
func (t *Tree) fieldRestyled(f *field, old, cs *style.ComputedStyle) {
	f.markPaint()
	if old == nil {
		return
	}
	ots, nts := old.TextStyle(), cs.TextStyle()
	ots.Paint, nts.Paint = nil, nil // colors do not lay out
	ots.Decoration.Paint, nts.Decoration.Paint = nil, nil
	if !sameTextStyle(&ots, &nts) || f.paraStyle(old) != f.paraStyle(cs) || old.FieldSizing != cs.FieldSizing ||
		old.Placeholder != cs.Placeholder || t.classicBars(old) != t.classicBars(cs) {
		t.engine.MarkDirty(f.bx)
		f.ph = nil
	}
}

// ---------------------------------------------------------------------
// Layout.

// paraStyle returns the paragraph style of the field's text: an Input's
// line never wraps (white-space: pre); neither truncates.
func (f *field) paraStyle(cs *style.ComputedStyle) text.ParagraphStyle {
	ps := cs.ParagraphStyle()
	ps.TextOverflow, ps.LineClamp = text.TextOverflow(0), 0
	if !f.multi {
		ps.WhiteSpace = text.WhiteSpacePre
	}
	return ps
}

// layout returns the layout of the editor's Display, rebuilt when the
// display or the style changed and broken (aligned, for an Input) at the
// width of the field's text area; nil before the field has a style.
func (f *field) layout() *text.Layout {
	cs := f.bx.cs
	if cs == nil {
		return nil
	}
	return f.layoutAt(cs, f.textWidth())
}

// layoutAt is layout at width w.
func (f *field) layoutAt(cs *style.ComputedStyle, w float32) *text.Layout {
	t := f.t
	ts, ps := cs.TextStyle(), f.paraStyle(cs)
	if f.lay == nil || f.layVer != f.ed.DisplayVersion() || f.ps != ps || !sameTextStyle(&f.ts, &ts) {
		f.lay = text.NewLayout(t.fonts, &text.Paragraph{Text: f.ed.Display(), Style: ps, Default: ts})
		t.stats.Shapes++
		f.layVer, f.ts, f.ps = f.ed.DisplayVersion(), ts, ps
		f.brokeAt = nan32()
	}
	if f.brokeAt != w {
		switch {
		case f.multi:
			f.lay.BreakLines(w)
			t.stats.Breaks++
		case math.IsInf(float64(w), 1):
			return f.lay // a measurement: the line is never broken
		default:
			f.lay.AlignTo(max(w, f.lay.MaxContentWidth()))
		}
		f.brokeAt = w
	}
	return f.lay
}

// placeholder returns the layout of the placeholder at width w, nil
// without one.
func (f *field) placeholder(cs *style.ComputedStyle, w float32) *text.Layout {
	s := f.props().placeholder
	if s == "" {
		f.ph = nil
		return nil
	}
	if !f.multi {
		s = strings.Map(func(r rune) rune {
			if r == '\n' || r == '\r' {
				return -1 // HTML strips line breaks from an input's placeholder
			}
			return r
		}, s)
	}
	ts, ps := f.placeholderStyle(cs), f.paraStyle(cs)
	if f.ph == nil || f.phText != s || f.phPS != ps || !sameTextStyle(&f.phTS, &ts) {
		f.ph = text.NewLayout(f.t.fonts, &text.Paragraph{Text: s, Style: ps, Default: ts})
		f.phText, f.phTS, f.phPS = s, ts, ps
		f.phBroke = nan32()
	}
	if f.phBroke != w {
		switch {
		case f.multi:
			f.ph.BreakLines(w)
		case math.IsInf(float64(w), 1):
			return f.ph
		default:
			f.ph.AlignTo(max(w, f.ph.MaxContentWidth()))
		}
		f.phBroke = w
	}
	return f.ph
}

// placeholderStyle returns the placeholder's text style: its
// placeholder: classes, or the field's text in its color at half opacity
// (Tailwind's Preflight: color-mix(in oklab, currentcolor 50%,
// transparent)).
func (f *field) placeholderStyle(cs *style.ComputedStyle) text.TextStyle {
	if cs.Placeholder != nil {
		c := style.ComputedStyle{Text: *cs.Placeholder}
		return c.TextStyle()
	}
	ts := cs.TextStyle()
	col := cs.Text.Color
	col.A = float32(col.A * 0.5)
	ts.Paint = col
	return ts
}

// lineHeight returns the height of one line of the field's text: the
// line box of an empty line in its style.
func (f *field) lineHeight(cs *style.ComputedStyle) float32 {
	ts, ps := cs.TextStyle(), f.paraStyle(cs)
	if f.empty == nil || f.emptyPS != ps || !sameTextStyle(&f.emptyTS, &ts) {
		f.empty = text.NewLayout(f.t.fonts, &text.Paragraph{Text: "", Style: ps, Default: ts})
		f.emptyTS, f.emptyPS = ts, ps
	}
	return f.empty.Line(0).Height
}

// contentBox returns the field's content box size from its last layout,
// and the width its text breaks at (a Textarea's classic scrollbar
// gutter left out).
func (f *field) textWidth() float32 {
	l := f.bx.ld.Layout()
	w := l.Size.W - l.Border.Left - l.Border.Right - l.Padding.Left - l.Padding.Right - l.ScrollbarSize.W
	return max(w, 0)
}

// charWidths returns Chromium's average and maximum character widths of
// the field's primary font (see the comment at the top of field.go).
func (f *field) charWidths(cs *style.ComputedStyle) (avg, maxW float32) {
	ts := cs.TextStyle()
	size := cs.Text.FontSize
	face := f.t.fonts.Face(text.Query{Families: ts.Families, Weight: ts.Weight, Style: ts.Style, Stretch: ts.Stretch}, size)
	if face == nil {
		return float32(size * 0.5), float32(size)
	}
	m := face.Metrics().Scale(size)
	avg = m.AvgCharWidth
	if r := float32(math.Round(float64(avg))); r > avg {
		avg = r
	}
	return avg, float32(math.Floor(float64(m.MaxCharWidth) + 0.5))
}

// textareaRows returns the Rows of a Textarea (2 by default).
func (f *field) textareaRows() int {
	if r := f.props().rows; r > 0 {
		return r
	}
	return 2
}

// caretWidth is the width of the caret and of the room an Input keeps for
// it after its text (Chromium's).
const caretWidth = 1

// measure implements layout.Measurer for a text field (see the comment at
// the top of field.go).
func (f *field) measure(in layout.MeasureInput) layout.Measurement {
	cs := f.bx.cs
	if cs == nil {
		return knownOnly(in)
	}
	lh := f.lineHeight(cs)
	content := cs.FieldSizing == style.FieldSizingContent
	var m layout.Measurement
	switch {
	case in.HasWidth:
		m.Width = in.Width
	case content:
		w := float32(0)
		if lay := f.layoutAt(cs, float32(math.Inf(1))); lay != nil {
			w = lay.MaxContentWidth()
		}
		if ph := f.placeholder(cs, float32(math.Inf(1))); ph != nil && f.ed.RawValue() == "" {
			w = max(w, ph.MaxContentWidth())
		}
		// Chromium: the text's width, the caret's when there is none.
		m.Width = max(w, caretWidth)
		if f.multi && in.AvailableWidth.Kind == layout.Definite {
			m.Width = min(m.Width, in.AvailableWidth.Value)
		}
	default:
		avg, mx := f.charWidths(cs)
		if f.multi {
			m.Width = float32(math.Ceil(float64(avg) * float64(20)))
			if f.t.classicBars(cs) {
				m.Width += classicGutter
			}
		} else {
			m.Width = float32(math.Ceil(float64(avg)*19 + float64(mx)))
		}
	}
	switch {
	case in.HasHeight:
		m.Height = in.Height
	case !f.multi:
		m.Height = lh
	case content:
		h := float32(0)
		if lay := f.layoutAt(cs, m.Width); lay != nil {
			h = lay.Height()
		}
		if ph := f.placeholder(cs, m.Width); ph != nil && f.ed.RawValue() == "" {
			h = max(h, ph.Height())
		}
		m.Height = max(h, lh)
	default:
		m.Height = float32(lh * float32(f.textareaRows()))
	}
	if !f.multi {
		// The line is centered in the content box: in a full layout the
		// available height is the final content box.
		h := m.Height
		if in.Final && !in.HasHeight && in.AvailableHeight.Kind == layout.Definite {
			h = in.AvailableHeight.Value
		}
		if lay := f.layoutAt(cs, max(m.Width, 0)); lay != nil {
			m.Baseline, m.HasBaseline = (h-lh)/2+lay.Line(0).Baseline, true
		}
	}
	return m
}

// lineOffset returns the offset of an Input's line in its content box:
// centered vertically.
func (f *field) lineOffset() float32 {
	cs := f.bx.cs
	if cs == nil || f.multi {
		return 0
	}
	l := f.bx.ld.Layout()
	ch := l.Size.H - l.Border.Top - l.Border.Bottom - l.Padding.Top - l.Padding.Bottom
	return (ch - f.lineHeight(cs)) / 2
}

// textOrigin returns the origin of the field's text layout relative to
// its border box, before a Textarea's scroll offset: the content box,
// moved by an Input's horizontal scroll and centered line.
func (f *field) textOrigin() (x, y float32) {
	l := f.bx.ld.Layout()
	x, y = l.Border.Left+l.Padding.Left, l.Border.Top+l.Padding.Top
	if !f.multi {
		x -= f.scrollX
		y += f.lineOffset()
	}
	return x, y
}

// ---------------------------------------------------------------------
// Keeping the caret in view.

// reveal scrolls the field so that the selection's focus (the caret) is
// visible: an Input's own horizontal scroll, a Textarea's scroll
// container (at once, "nearest").
func (f *field) reveal() {
	lay := f.layout()
	if lay == nil || f.bx.cs == nil {
		return
	}
	c := lay.Caret(f.ed.Selection().Focus)
	if !f.multi {
		w := f.textWidth()
		maxX := max(lay.MaxContentWidth()+caretWidth-w, 0)
		x := f.scrollX
		switch {
		case c.X < x:
			x = c.X
		case c.X+caretWidth > x+w:
			x = c.X + caretWidth - w
		}
		x = min(max(x, 0), maxX)
		if x != f.scrollX {
			f.scrollX = x
			f.markPaint()
		}
		return
	}
	s := f.bx.scr
	if s == nil || !f.t.layoutCurrent() {
		return
	}
	l := f.bx.ld.Layout()
	top := l.Padding.Top + c.Top
	d := nearest(top, top+c.Height, s.pos.Y, s.pos.Y+s.client.H)
	if d != 0 {
		f.t.scrollDirect(s, s.clamp(s.pos.Add(gfx.Point{Y: d})))
	}
}

// fieldExtents returns the largest scroll offsets of a Textarea's scroll
// container from its text (extents, scroll.go), without a layout of the
// tree.
func (f *field) fieldExtents() (maxX, maxY float32) {
	lay := f.layout()
	if lay == nil {
		return 0, 0
	}
	l := f.bx.ld.Layout()
	h := max(lay.Height(), f.lineHeight(f.bx.cs))
	if ph := f.placeholder(f.bx.cs, f.textWidth()); ph != nil && f.ed.RawValue() == "" {
		h = max(h, ph.Height())
	}
	clientH := l.Size.H - l.Border.Top - l.Border.Bottom - l.ScrollbarSize.H
	return 0, max(h+l.Padding.Top+l.Padding.Bottom-clientH, 0)
}

func nan32() float32 { return float32(math.NaN()) }
