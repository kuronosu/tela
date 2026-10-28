package h

import (
	"reflect"

	"github.com/kuronosu/tela"
	"github.com/kuronosu/tela/internal/core"
	"github.com/kuronosu/tela/internal/style"
)

// Input returns a single-line text field. Like a browser's, it edits its
// value with the keyboard (typing, arrows and Ctrl+arrows, Home/End,
// Backspace/Delete, Ctrl+A, Ctrl+Z/Ctrl+Y, the clipboard), the mouse
// (click, drag, double and triple click) and input methods (IME), and
// submits its Form on Enter. Give it a value with Value and OnChange (or
// Bind: controlled), DefaultValue (uncontrolled) or BindValue, a Type, a
// Placeholder and a Label.
//
// It has Tailwind's Preflight defaults: no border, padding or background
// (style it with classes), the inherited font, and a width of about 20
// characters (Chromium's size=20) unless a class sets one; its text is
// centered vertically in its height. field-sizing-content sizes it to its
// text. It takes no children.
//
//	Input(Type("email"), Placeholder("you@example.com"), Bind(email, setEmail),
//	    Class("w-full rounded-md border border-slate-300 px-3 py-2 placeholder:text-slate-400 "+
//	        "focus:outline-2 focus:outline-indigo-500"))
func Input(children ...tela.Child) tela.Node { return core.Primitive(style.KindInput, children...) }

// Textarea returns a multi-line text field: like Input, but Enter
// inserts a line break, its text wraps, and it scrolls vertically (overflow
// auto) when the text is taller than it. With text-nowrap (or
// whitespace-pre) its lines do not wrap, and it also scrolls horizontally
// when a line is wider than it. It is Rows lines tall (2 by
// default) and about 20 characters wide unless classes size it;
// field-sizing-content makes it grow with its text (between min-h-* and
// max-h-*). It takes no children: give its text with Value, DefaultValue
// or BindValue.
//
//	Textarea(Rows(4), Placeholder("Write a message"), Bind(msg, setMsg),
//	    Class("w-full rounded-md border border-slate-300 p-2"))
func Textarea(children ...tela.Child) tela.Node {
	return core.Primitive(style.KindTextarea, children...)
}

// Form returns a form: a block that groups fields and buttons. Enter in
// one of its single-line fields, or a click on a Button inside it, submits
// it (OnSubmit); a Button with Type("reset") restores the DefaultValue of
// its uncontrolled fields. Buttons inside a form are submit buttons
// unless Type says "button".
//
//	Form(OnSubmit(func() { save() }),
//	    Input(Bind(name, setName)),
//	    Button(Type("submit"), Text("Save")))
func Form(children ...tela.Child) tela.Node { return core.Primitive(style.KindForm, children...) }

// Label returns a label for a field: inline text that gives the field its
// accessible name and focuses it when clicked. It labels the element
// whose ID is its For, or else the Input, Textarea or Button inside it.
//
//	Label(For("name"), Text("Name")), Input(ID("name"))
//	Label(Text("Name "), Input())
//
// A Label that labels nothing (For naming no mounted Input, Textarea or
// Button, and no such element inside) names nothing and focuses nothing;
// dev mode warns about it once. For a caption that labels no field, use
// Span or P.
func Label(children ...tela.Child) tela.Node { return core.Primitive(style.KindLabel, children...) }

// Value makes a text field controlled: it shows s, and after every event
// it shows s again unless the app rebuilt it with a new value (React's
// controlled inputs). Pair it with OnChange, or use Bind.
func Value(s string) tela.Prop { return core.FieldValue(s) }

// DefaultValue gives an uncontrolled text field its initial value; later
// values are ignored (React's defaultValue). Read the value with
// tela.ElementRef's Value.
func DefaultValue(s string) tela.Prop { return core.DefaultValue(s) }

// Bind makes a text field controlled by a state: Value(text) plus an
// OnChange that sends every new value to set, a tela.Setter or any
// func(string) (of a named func type too), such as one that dispatches an
// action to a reducer. A nil function panics.
//
//	name, setName := tela.UseState("")
//	Input(Bind(name, setName))
//
//	form, d := tela.UseReducer(reduce, Form{})
//	Input(Bind(form.Email, func(s string) { d.Dispatch(setEmail(s)) }))
func Bind[S tela.Setter[string] | ~func(string)](text string, set S) tela.Prop {
	switch s := any(set).(type) {
	case tela.Setter[string]:
		return core.Bind(text, s)
	case func(string):
		return core.BindFunc(text, s)
	default:
		// A named func type: its underlying type is func(string).
		f := reflect.ValueOf(s).Convert(reflect.TypeFor[func(string)]()).Interface().(func(string))
		return core.BindFunc(text, f)
	}
}

// BindValue binds a text field to v, from tela.UseValue: the field shows
// v's value and writes every edit to it without rebuilding the component
// (only what reads v rebuilds or repaints), and follows v.Set from
// anywhere.
//
//	query := tela.UseValue("")
//	Input(Type("search"), BindValue(query))
func BindValue(v *tela.Value[string]) tela.Prop { return core.BindValue(v) }

// Type sets the type of an Input: "text" (the default), "password" (the
// value shows as dots and cannot be copied), "email", "search", "url",
// "tel" or "number" (a hint for input methods; the value stays text, with
// no spinner). "email" and "url" values that are not valid match
// invalid:, unless the Input is ReadOnly or disabled (HTML bars those
// from validation). On a Button it sets "submit" (the default inside a Form),
// "button" or "reset". An unknown type panics in dev mode.
func Type(t string) tela.Prop { return core.FieldType(t) }

// Rows sets how many lines tall a Textarea is (2 by default).
func Rows(n int) tela.Prop { return core.Rows(n) }

// Placeholder sets the text a text field shows while it is empty, in the
// field's color at half opacity unless placeholder: classes style it.
func Placeholder(s string) tela.Prop { return core.Placeholder(s) }

// ReadOnly makes a text field read-only: it can be focused, selected and
// copied, but not edited; it matches read-only:.
func ReadOnly(on bool) tela.Prop { return core.ReadOnly(on) }

// MaxLength limits what the user can type or paste into a text field to n
// UTF-16 code units (HTML's maxlength: "😀" counts 2); a value set by the
// app may be longer. 0 means no limit.
func MaxLength(n int) tela.Prop { return core.MaxLength(n) }

// AutoComplete is HTML's autocomplete hint ("email", "current-password"...).
// It is accepted for familiarity; Tela has no autofill.
func AutoComplete(s string) tela.Prop { return core.AutoComplete(s) }

// SpellCheck is HTML's spellcheck hint. It is accepted and ignored: Tela
// has no spell checker.
func SpellCheck(on bool) tela.Prop { return core.SpellCheck(on) }

// Name names a text field or a button in its form (HTML's name).
func Name(s string) tela.Prop { return core.Name(s) }

// ID gives an element an identifier, unique in the window, that a Label's
// For refers to (HTML's id). tela.UseID makes one for a reusable field.
func ID(id string) tela.Prop { return core.ID(id) }

// For makes a Label label the element whose ID is id.
func For(id string) tela.Prop { return core.For(id) }

// OnChange calls handler after every edit of a text field's value (React's
// onChange: each keystroke, paste, cut, undo, and the text an IME
// composes, with e.IsComposing). It bubbles: on a Form it sees every
// field.
//
//	Input(Value(name), OnChange(func(e tela.ChangeEvent) { setName.Set(e.Value) }))
func OnChange[F ~func() | ~func(tela.ChangeEvent)](handler F) tela.Prop {
	return onEvent[tela.ChangeEvent]("change", "OnChange", handler)
}

// OnBeforeInput calls handler before a text field's value changes by an
// edit of the user (the DOM's beforeinput); e.PreventDefault() cancels
// the edit (except an IME composition's). It bubbles.
func OnBeforeInput[F ~func() | ~func(tela.InputEvent)](handler F) tela.Prop {
	return onEvent[tela.InputEvent]("beforeinput", "OnBeforeInput", handler)
}

// OnCompositionStart calls handler when an input method (IME) starts
// composing text in a text field. It bubbles.
func OnCompositionStart[F ~func() | ~func(tela.CompositionEvent)](handler F) tela.Prop {
	return onEvent[tela.CompositionEvent]("compositionstart", "OnCompositionStart", handler)
}

// OnCompositionUpdate calls handler when the text an input method
// composes changes (e.Data). It bubbles.
func OnCompositionUpdate[F ~func() | ~func(tela.CompositionEvent)](handler F) tela.Prop {
	return onEvent[tela.CompositionEvent]("compositionupdate", "OnCompositionUpdate", handler)
}

// OnCompositionEnd calls handler when an input method ends a
// composition, committed (e.Data) or canceled (""). It bubbles.
func OnCompositionEnd[F ~func() | ~func(tela.CompositionEvent)](handler F) tela.Prop {
	return onEvent[tela.CompositionEvent]("compositionend", "OnCompositionEnd", handler)
}

// OnSelect calls handler when the user selects text in a text field, or
// code changes its selection (e.Start, e.End). It bubbles.
func OnSelect[F ~func() | ~func(tela.SelectEvent)](handler F) tela.Prop {
	return onEvent[tela.SelectEvent]("select", "OnSelect", handler)
}

// OnSubmit calls handler when a Form is submitted: Enter in one of its
// single-line fields, or a click on a submit Button inside it. It bubbles.
func OnSubmit[F ~func() | ~func(tela.SubmitEvent)](handler F) tela.Prop {
	return onEvent[tela.SubmitEvent]("submit", "OnSubmit", handler)
}
