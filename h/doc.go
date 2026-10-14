// Package h holds the elements, props and helpers of a Tela UI. It is
// designed to be dot-imported, so that markup reads like JSX:
//
//	import (
//	    "github.com/kuronosu/tela"
//	    . "github.com/kuronosu/tela/h"
//	)
//
//	func Greeting(name string) tela.Node {
//	    return Div(Class("flex items-center gap-3 rounded-xl bg-white p-4 shadow-md"),
//	        H2(Class("text-lg font-semibold text-slate-900"), Textf("Hello, %s", name)),
//	        If(name == "", P(Class("text-slate-500"), Text("Who are you?"))),
//	    )
//	}
//
// To keep collisions with the importing package unlikely, h exports only
// element constructors (Div, Span, Text, H1 to H6, P, Button, Img, Strong,
// Em, Code, A, and the form elements Input, Textarea, Form and Label),
// props (Class, Style, Key, Disabled, Src, Alt, Href, Id, the focus props
// TabIndex, AutoFocus and Ref, the semantics props Role and Aria..., the
// field props Value, DefaultValue, Bind, BindValue, Type, Rows,
// Placeholder, ReadOnly, MaxLength, AutoComplete, SpellCheck, Name and
// For, and the event handlers OnClick, OnDoubleClick, OnContextMenu,
// OnMouse..., OnPointer..., OnDrag..., OnLongPress, OnKey..., OnFocus,
// OnBlur, OnWheel, OnScroll, OnChange, OnBeforeInput, OnComposition...,
// OnSelect and OnSubmit), the helpers If, IfElse (for nodes and props
// alike), Map, MapIndex, Fragment and TextFn (text that a Value changes
// without rebuilding the component), and VirtualList with its Overscan
// prop.
//
// Everything else (App, Run, hooks, events, colors, typed style props)
// lives in package tela.
//
// An element takes its props and children in one list, in any order:
//
//	Div(Class("p-4"), Key(id), Text("hi"), Span(Text("there")))
//
// Props apply in order, so a later Class wins over an earlier one and
// Style wins over every class. Children are nodes only; wrap text in Text
// or Textf. The parameters are tela.Child, so passing a bare string is a
// compile error.
//
// Elements have the defaults of Tailwind's Preflight: no margins, padding
// or borders, headings in the surrounding font size, block images. A Button
// has no background of its own, and neither has an Input or a Textarea:
// style fields with classes.
//
// Text fields are controlled, like React's:
//
//	func Signup() tela.Node {
//	    email, setEmail := tela.UseState("")
//	    return Form(Class("flex flex-col gap-2"), OnSubmit(func(tela.SubmitEvent) { register(email) }),
//	        Label(For("email"), Class("text-sm font-medium"), Text("Email")),
//	        Input(Id("email"), Type("email"), Bind(email, setEmail), Placeholder("you@example.com"),
//	            Class("rounded-md border border-slate-300 px-3 py-2 invalid:border-red-500")),
//	        Button(Class("rounded-md bg-indigo-600 px-4 py-2 text-white"), Text("Sign up")))
//	}
package h
