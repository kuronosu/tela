package desktop

import (
	"fmt"

	"github.com/kuronosu/tela"
	"github.com/kuronosu/tela/internal/core"
	"github.com/kuronosu/tela/internal/platform"
)

// MessageKind is the icon and tone of a message box.
type MessageKind uint8

const (
	MessageInfo     MessageKind = iota // information (the default)
	MessageWarning                     // a warning: something may go wrong
	MessageError                       // an error: something went wrong
	MessageQuestion                    // a question the buttons answer
)

var messageKindNames = [...]string{"info", "warning", "error", "question"}

func (k MessageKind) String() string {
	if int(k) < len(messageKindNames) {
		return messageKindNames[k]
	}
	return fmt.Sprintf("MessageKind(%d)", uint8(k))
}

// Buttons is the set of buttons of a message box: the sets every native
// message box has.
type Buttons uint8

const (
	ButtonsOK          Buttons = iota // OK (the default)
	ButtonsOKCancel                   // OK and Cancel
	ButtonsYesNo                      // Yes and No
	ButtonsYesNoCancel                // Yes, No and Cancel
)

// Answer is the button the user chose in a message box.
type Answer uint8

const (
	AnswerOK Answer = iota
	AnswerCancel
	AnswerYes
	AnswerNo
)

var answerNames = [...]string{"ok", "cancel", "yes", "no"}

func (a Answer) String() string {
	if int(a) < len(answerNames) {
		return answerNames[a]
	}
	return fmt.Sprintf("Answer(%d)", uint8(a))
}

// Has reports whether the set b has the button a.
func (b Buttons) Has(a Answer) bool { return platform.MessageButtons(b).Has(platform.MessageAnswer(a)) }

// DismissAnswer is the answer of a box with buttons b that the user closes
// without a button (Escape, the close button): Cancel when the set has it,
// OK for ButtonsOK and No for ButtonsYesNo.
func (b Buttons) DismissAnswer() Answer {
	return Answer(platform.MessageButtons(b).DismissAnswer())
}

// DefaultAnswer is the button Enter chooses: OK, or Yes for the sets with
// Yes.
func (b Buttons) DefaultAnswer() Answer {
	if b == ButtonsYesNo || b == ButtonsYesNoCancel {
		return AnswerYes
	}
	return AnswerOK
}

// Message is a message box: a short text with a fixed set of buttons.
type Message struct {
	// Kind is the icon and tone (MessageInfo by default).
	Kind MessageKind
	// Title is the box's title; empty uses the kind's ("Information",
	// "Warning", "Error", "Question").
	Title string
	// Text is the message. Keep it to a few sentences; a blank line
	// separates paragraphs.
	Text string
	// Buttons is the set of buttons (ButtonsOK by default).
	Buttons Buttons
	// Parent is the window the box is modal to. The zero handle is the
	// window the call comes from (the window whose handler or effect is
	// running, else the focused window, else the main window).
	Parent tela.WindowHandle
}

// kindTitles are the default titles, and the names of the drawn box's
// icons.
var kindTitles = [...]string{"Information", "Warning", "Error", "Question"}

func (m Message) title() string {
	if m.Title != "" {
		return m.Title
	}
	return kindTitles[m.Kind]
}

// ShowMessage shows a message box and calls done with the button the user
// chose (done may be nil). Closing the box without a button (Escape, its
// close button) answers [Buttons.DismissAnswer]; Enter chooses
// [Buttons.DefaultAnswer].
//
//	desktop.ShowMessage(desktop.Message{
//	    Kind:    desktop.MessageQuestion,
//	    Title:   "Unsaved changes",
//	    Text:    "Save the changes to \"Groceries\" before closing?",
//	    Buttons: desktop.ButtonsYesNoCancel,
//	}, func(a desktop.Answer) {
//	    switch a {
//	    case desktop.AnswerYes:
//	        saveAndClose()
//	    case desktop.AnswerNo:
//	        closeWithoutSaving()
//	    } // AnswerCancel: keep editing
//	})
//
// The box is the OS's where it has one (Windows); elsewhere (Linux, whose
// desktop portal has no message box) Tela draws it in a small modal
// window with the app's theme tokens, in light and dark. Call it on the
// UI thread (a handler, an effect; from a goroutine, wrap it in
// tela.Post). It returns at once and done runs later on the UI thread,
// exactly once. The error says no box could be shown (no running app, or
// no window to show a drawn box in); done is then never called. Tests
// answer it with telatest's App.AnswerMessage.
func ShowMessage(m Message, done func(Answer)) error {
	if m.Kind > MessageQuestion {
		panic(fmt.Sprintf("desktop.ShowMessage: invalid Kind %d; use desktop.MessageInfo, MessageWarning, MessageError or MessageQuestion", m.Kind))
	}
	if m.Buttons > ButtonsYesNoCancel {
		panic(fmt.Sprintf("desktop.ShowMessage: invalid Buttons %d; use desktop.ButtonsOK, ButtonsOKCancel, ButtonsYesNo or ButtonsYesNoCancel", m.Buttons))
	}
	if done == nil {
		done = func(Answer) {}
	}
	d, err := running("ShowMessage")
	if err != nil {
		return err
	}
	parent := window(d, m.Parent)
	opts := platform.MessageBoxOptions{
		Parent:  d.PlatformWindow(parent),
		Kind:    platform.MessageKind(m.Kind),
		Title:   m.title(),
		Text:    m.Text,
		Buttons: platform.MessageButtons(m.Buttons),
	}
	err = d.Services().MessageBox(opts, func(a platform.MessageAnswer, err error) {
		if err == nil {
			done(Answer(a))
			return
		}
		// The native box failed after it was asked for: draw one
		// instead, or answer as a dismissed box.
		if derr := drawMessage(d, parent, m, done); derr != nil {
			done(m.Buttons.DismissAnswer())
		}
	})
	if err == nil {
		return nil
	}
	// No native box (Linux, a backend without one): Tela draws it.
	return drawMessage(d, parent, m, done)
}

// drawMessage opens the drawn message box as a modal child window of
// parent.
func drawMessage(d core.Desktop, parent tela.WindowHandle, m Message, done func(Answer)) error {
	box := &drawnBox{msg: m, done: done}
	w, h := m.estimateSize()
	cw, err := core.OpenWindow(parent, core.ChildWindowSpec{
		WindowSpec: core.WindowSpec{
			Title: m.title(), Width: w, Height: h,
			MinWidth: w, MinHeight: h, MaxWidth: w, MaxHeight: h,
		},
		Modal:     true,
		RootClass: "bg-background text-foreground",
		OnClose:   func() { box.answer(m.Buttons.DismissAnswer()) },
		Root:      core.NodeOf(messageView{Message: m, box: box}),
	})
	if err != nil {
		return wrap("ShowMessage", err)
	}
	box.win = cw
	reg := drawnBoxes(d)
	reg.open = append(reg.open, box)
	box.reg = reg
	return nil
}

// drawnRegistry lists the drawn message boxes open in an app (for
// telatest, through the window titles and the semantics tree).
type drawnRegistry struct{ open []*drawnBox }

type drawnRegistryKey struct{}

func drawnBoxes(d core.Desktop) *drawnRegistry {
	return d.Local(drawnRegistryKey{}, func() any { return &drawnRegistry{} }).(*drawnRegistry)
}

// drawnBox is an open drawn message box.
type drawnBox struct {
	msg      Message
	done     func(Answer)
	win      *core.CodeWindow
	reg      *drawnRegistry
	answered bool
	height   float32 // the height last given to the window
}

// answer closes the box and calls done once.
func (b *drawnBox) answer(a Answer) {
	if b.answered {
		return
	}
	b.answered = true
	if b.reg != nil {
		for i, x := range b.reg.open {
			if x == b {
				b.reg.open = append(b.reg.open[:i], b.reg.open[i+1:]...)
				break
			}
		}
	}
	if b.win != nil {
		b.win.Close()
	}
	b.done(a)
}

// resize fits the window's height to the content's (measured after
// layout), when the estimate was off by a pixel or more.
func (b *drawnBox) resize(h float32) {
	if b.win == nil || b.answered || h <= 0 {
		return
	}
	cur := b.height
	if cur == 0 {
		_, cur = b.msg.estimateSize()
	}
	if d := h - cur; d > -1 && d < 1 {
		return
	}
	b.height = h
	w, _ := b.msg.estimateSize()
	b.win.Update(core.ChildWindowSpec{
		WindowSpec: core.WindowSpec{
			Title: b.msg.title(), Width: w, Height: h,
			MinWidth: w, MinHeight: h, MaxWidth: w, MaxHeight: h,
		},
		Modal:     true,
		RootClass: "bg-background text-foreground",
		OnClose:   func() { b.answer(b.msg.Buttons.DismissAnswer()) },
		Root:      core.NodeOf(messageView{Message: b.msg, box: b}),
	})
}

// The drawn box's metrics, in logical pixels: the width, the padding, the
// icon column and an estimate of the text's line height and average
// character width (text-sm Inter), from which the first height is
// guessed; the content's measured height corrects it after the first
// layout.
const (
	boxWidth   = 440
	boxPadding = 24
	boxIcon    = 40 + 16 // icon and gap
	boxTitleH  = 28
	boxLineH   = 20
	boxCharW   = 7
	boxButtons = 36 + 24 // button row and the gap above it
)

func (m Message) estimateSize() (w, h float32) {
	inner := boxWidth - 2*boxPadding - boxIcon
	perLine := max(inner/boxCharW, 1)
	lines := 0
	for _, para := range splitLines(m.Text) {
		n := (len([]rune(para)) + perLine - 1) / perLine
		lines += max(n, 1)
	}
	h = 2*boxPadding + boxTitleH + boxButtons
	if lines > 0 {
		h += 8 + float32(lines*boxLineH)
	}
	return boxWidth, h
}

func splitLines(s string) []string {
	if s == "" {
		return nil
	}
	var out []string
	start := 0
	for i := 0; i < len(s); i++ {
		if s[i] == '\n' {
			out = append(out, s[start:i])
			start = i + 1
		}
	}
	return append(out, s[start:])
}
