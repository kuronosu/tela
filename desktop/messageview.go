package desktop

import (
	"github.com/kuronosu/tela"
	. "github.com/kuronosu/tela/h"
)

// The drawn message box: plain h elements with the theme's semantic tokens
// (desktop sits below kit), so it follows App.Theme and the color scheme.
// Its look follows shadcn/ui's AlertDialog: a title in text-lg
// font-semibold, the text in text-sm text-muted-foreground, the buttons
// at the end, the default one filled with bg-primary. The buttons go in
// the order of GNOME's dialogs (the drawn box is the Linux one): Cancel
// first, the affirmative button last.

// The icons, drawn for this box (24×24, stroked with currentColor).
var (
	iconInfo = tela.Icon{SVG: `<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round">` +
		`<circle cx="12" cy="12" r="9.5"/><path d="M12 11v5.5"/><circle cx="12" cy="7.5" r="1.1" fill="currentColor" stroke="none"/></svg>`}
	iconWarning = tela.Icon{SVG: `<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round">` +
		`<path d="M10.3 3.9 2.6 17.5A2 2 0 0 0 4.3 20.5h15.4a2 2 0 0 0 1.7-3L13.7 3.9a2 2 0 0 0-3.4 0Z"/><path d="M12 9.5v4.5"/><circle cx="12" cy="17" r="1.1" fill="currentColor" stroke="none"/></svg>`}
	iconError = tela.Icon{SVG: `<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round">` +
		`<circle cx="12" cy="12" r="9.5"/><path d="m9 9 6 6M15 9l-6 6"/></svg>`}
	iconQuestion = tela.Icon{SVG: `<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round">` +
		`<circle cx="12" cy="12" r="9.5"/><path d="M9.6 9.3a2.5 2.5 0 0 1 4.8.9c0 1.7-2.4 2.3-2.4 3.8"/><circle cx="12" cy="17" r="1.1" fill="currentColor" stroke="none"/></svg>`}
)

// kindLook is the icon and the tone classes of each MessageKind.
var kindLook = [...]struct {
	icon tela.Icon
	tone string
}{
	MessageInfo:     {iconInfo, "bg-sky-500/15 text-sky-600 dark:text-sky-400"},
	MessageWarning:  {iconWarning, "bg-amber-500/15 text-amber-600 dark:text-amber-400"},
	MessageError:    {iconError, "bg-destructive/15 text-destructive dark:text-red-400"},
	MessageQuestion: {iconQuestion, "bg-primary/10 text-primary"},
}

const (
	buttonBase = "inline-flex h-9 min-w-20 shrink-0 items-center justify-center rounded-md px-4 text-sm font-medium whitespace-nowrap " +
		"outline-none transition-colors focus-visible:ring-[3px] focus-visible:ring-ring/50"
	buttonPrimary = "bg-primary text-primary-foreground hover:bg-primary/90"
	buttonOutline = "border border-border bg-background shadow-xs hover:bg-accent hover:text-accent-foreground " +
		"dark:border-input dark:bg-input/30 dark:hover:bg-input/50"
)

// answerLabels are the button labels.
var answerLabels = [...]string{AnswerOK: "OK", AnswerCancel: "Cancel", AnswerYes: "Yes", AnswerNo: "No"}

// buttonOrder returns the buttons of a set in the order they are drawn.
func buttonOrder(b Buttons) []Answer {
	switch b {
	case ButtonsOKCancel:
		return []Answer{AnswerCancel, AnswerOK}
	case ButtonsYesNo:
		return []Answer{AnswerNo, AnswerYes}
	case ButtonsYesNoCancel:
		return []Answer{AnswerCancel, AnswerNo, AnswerYes}
	}
	return []Answer{AnswerOK}
}

// messageView is the content of a drawn message box's window. box is nil
// in a preview (MessagePreview).
type messageView struct {
	Message Message
	box     *drawnBox
}

func (v messageView) Build() tela.Node {
	m := v.Message
	box := v.box
	ref := tela.UseElementRef()
	size := tela.UseSize(ref)
	height := size.Get().Height
	tela.UseEffect(func() func() {
		if box != nil {
			box.resize(height)
		}
		return nil
	}, height)
	answer := func(a Answer) {
		if box != nil {
			box.answer(a)
		}
	}

	look := kindLook[m.Kind]
	def := m.Buttons.DefaultAnswer()
	buttons := make([]tela.Child, 0, 4)
	buttons = append(buttons, Class("flex justify-end gap-2"))
	for _, a := range buttonOrder(m.Buttons) {
		cls := buttonOutline
		if a == def {
			cls = buttonPrimary
		}
		buttons = append(buttons, Button(Key(int(a)), Class(buttonBase, cls), AutoFocus(a == def),
			OnClick(func() { answer(a) }), Text(answerLabels[a])))
	}

	text := make([]tela.Child, 0, 4)
	text = append(text, Class("flex min-w-0 grow flex-col gap-2 pt-1.5"),
		H2(Class("text-lg leading-7 font-semibold"), Text(m.title())))
	for i, p := range splitLines(m.Text) {
		if p == "" {
			continue
		}
		text = append(text, P(Key(i), Class("text-sm leading-5 text-muted-foreground"), Text(p)))
	}

	return Div(Ref(ref), Role("alertdialog"), AriaLabel(m.title()), AriaDescription(m.Text),
		Class("flex flex-col gap-6 p-6"),
		OnKeyDown(func(e tela.KeyboardEvent) {
			if e.Key == tela.KeyEscape {
				e.PreventDefault()
				answer(m.Buttons.DismissAnswer())
			}
		}),
		Div(Class("flex items-start gap-4"),
			Div(Class("flex size-10 shrink-0 items-center justify-center rounded-full", look.tone),
				Icon(look.icon, Class("size-5"), AriaLabel(kindTitles[m.Kind]))),
			Div(text...),
		),
		Div(buttons...),
	)
}

// MessagePreview is the drawn message box of m as a component, without a
// window: what the box shows on Linux. tela snapshot renders it in a
// preview (the window is 440 pixels wide), and a test can mount it:
//
//	func Preview() tela.Component {
//	    return desktop.MessagePreview(desktop.Message{Kind: desktop.MessageWarning, Text: "Disk almost full."})
//	}
//
// Its buttons do nothing.
func MessagePreview(m Message) tela.Component { return messageView{Message: m} }
