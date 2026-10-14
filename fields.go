package tela

import "github.com/kuronosu/tela/internal/core"

// Text fields (h.Input, h.Textarea) and forms (h.Form) fire their own
// events, after the DOM's and React's.
//
//	email, setEmail := tela.UseState("")
//	return Form(OnSubmit(func(e tela.SubmitEvent) { login(email) }),
//	    Label(For("email"), Text("Email")),
//	    Input(Id("email"), Type("email"), Placeholder("you@example.com"), Bind(email, setEmail),
//	        Class("w-full rounded-md border border-slate-300 px-3 py-2 placeholder:text-slate-400 "+
//	            "focus:outline-2 focus:outline-indigo-500 invalid:border-red-500")),
//	    Button(Class("rounded-md bg-indigo-600 px-4 py-2 text-white"), Text("Sign in")))
//
// A field is controlled (h.Value with h.OnChange, or h.Bind: after every
// event it shows Value again unless the app changed it, as in React),
// uncontrolled (h.DefaultValue; read it with [ElementRef].Value) or bound
// to a [Value] (h.BindValue: typing writes the Value without rebuilding
// the component that owns it).

// InputEvent is passed to h.OnBeforeInput before a text field's value
// changes by an edit of the user (the DOM's beforeinput): InputType names
// the edit with the W3C Input Events names ("insertText",
// "insertFromPaste", "insertLineBreak", "deleteContentBackward",
// "deleteWordBackward", "deleteByCut", "historyUndo",
// "insertCompositionText"...), Data is the inserted text (empty for
// deletions), IsComposing reports an IME composition. PreventDefault
// cancels the edit, except during a composition and for the text an IME
// deletes around the caret. It bubbles.
//
//	// Digits only.
//	Input(OnBeforeInput(func(e tela.InputEvent) {
//	    if strings.Trim(e.Data, "0123456789") != "" {
//	        e.PreventDefault()
//	    }
//	}))
type InputEvent = core.InputEvent

// ChangeEvent is passed to h.OnChange after every edit of a text field's
// value, like React's onChange (the DOM's input event, not its change on
// blur): Value is the new value (with the text an IME is composing),
// InputType, Data and IsComposing describe the edit as in [InputEvent].
// It bubbles, so a Form's OnChange sees the edits of all its fields.
//
//	name, setName := tela.UseState("")
//	Input(Value(name), OnChange(func(e tela.ChangeEvent) { setName.Set(e.Value) }))
//	// the same: Input(Bind(name, setName))
type ChangeEvent = core.ChangeEvent

// CompositionEvent is passed to h.OnCompositionStart,
// h.OnCompositionUpdate and h.OnCompositionEnd while an input method
// (IME: Japanese, Chinese, Korean...) composes text in a text field: Data
// is the text being composed. It bubbles.
type CompositionEvent = core.CompositionEvent

// SelectEvent is passed to h.OnSelect when the user selects text in a text
// field or code changes its selection: Start and End are the selection in
// UTF-8 byte offsets of the value. It bubbles.
type SelectEvent = core.SelectEvent

// SubmitEvent is passed to h.OnSubmit when a h.Form is submitted: Enter in
// a single-line field of the form, or a click on a submit button inside
// it. Tela does not navigate; PreventDefault exists for familiarity.
//
//	Form(OnSubmit(func(e tela.SubmitEvent) {
//	    e.PreventDefault()
//	    login(email, password)
//	}), ...)
type SubmitEvent = core.SubmitEvent
