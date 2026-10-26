package main

import (
	"fmt"
	"testing"
	"time"

	"github.com/kuronosu/tela/telatest"
)

const windowW, windowH = 1100, 720

// start is the time the tests open the app: the seed data is relative to it,
// and every time the app asks for after that is a minute later, so the
// messages of a test are stamped 2:33 PM, 2:34 PM and so on.
var start = time.Date(2026, time.October, 26, 14, 32, 0, 0, time.UTC)

// mount opens the screen as main does, in dev mode, where any dev message
// fails the test, with the clock of the tests.
func mount(t *testing.T, w, h float32, opts ...telatest.Option) *telatest.App {
	t.Helper()
	now := start.Add(-time.Minute)
	app := App
	app.Root = Chat{Clock: func() time.Time {
		now = now.Add(time.Minute)
		return now
	}}
	opts = append([]telatest.Option{telatest.Size(w, h)}, opts...)
	return telatest.MountApp(t, app, opts...)
}

// The parts of the screen.
var (
	search   = telatest.ByRole("searchbox", "Search conversations")
	composer = telatest.ByRole("textbox", "Message")
	send     = telatest.ByRole("button", "Send message")
	log      = telatest.ByRole("log", "")
	typing   = telatest.ByRole("status", "Maya Chen is typing")
	maya     = telatest.ByRole("button", "Maya Chen")
	dev      = telatest.ByRole("button", "Dev Patel, 2 unread")
	sofia    = telatest.ByRole("button", "Sofia Rossi, 1 unread")
)

// message locates a message of the open conversation by who sent it and
// when, as its accessible name says.
func message(from, at string) telatest.Locator {
	return telatest.ByRole("article", from+", "+at)
}

// atEnd reports whether the last message is at the bottom of the list: its
// bottom edge is in view and no more than the list's padding above the
// list's own.
func atEnd(a *telatest.App, last telatest.Locator) bool {
	l, m := a.Bounds(log), a.Bounds(last)
	gap := (l.Y + l.Height) - (m.Y + m.Height)
	return gap >= 0 && gap <= 40
}

func TestNoDevMessages(t *testing.T) {
	for _, s := range [][2]float32{{windowW, windowH}, {800, 600}, {1600, 900}} {
		mount(t, s[0], s[1])
	}
}

// TestGolden is the first screen in light and dark, at scales 1 and 2.
func TestGolden(t *testing.T) {
	for _, scale := range []float64{1, 2} {
		t.Run(fmt.Sprintf("x%g", scale), func(t *testing.T) {
			mount(t, windowW, windowH, telatest.Scale(scale)).Golden(fmt.Sprintf("chat_x%g.png", scale))
			mount(t, windowW, windowH, telatest.Scale(scale), telatest.Dark()).Golden(fmt.Sprintf("chat_dark_x%g.png", scale))
		})
	}
}

// TestGoldenStates are a message just sent, with the typing indicator and
// the typing status in the header, and another conversation opened with a
// draft left behind, in light and dark.
func TestGoldenStates(t *testing.T) {
	for _, scale := range []float64{1, 2} {
		t.Run(fmt.Sprintf("x%g", scale), func(t *testing.T) {
			a := mount(t, windowW, windowH, telatest.Scale(scale))
			a.Fill(composer, "Yes! I'll bring the new frames.")
			a.Press("Enter")
			a.ExpectRole("status", "Maya Chen is typing")
			a.Golden(fmt.Sprintf("chat_typing_x%g.png", scale))

			// Switch to Dev with a draft in the composer of Maya, whose reply
			// arrives meanwhile: her row says so, and shows the draft.
			a.Fill(composer, "Can we make it 3:30?")
			a.Click(dev)
			a.Advance(replyDelay)
			a.Golden(fmt.Sprintf("chat_switched_x%g.png", scale))

			d := mount(t, windowW, windowH, telatest.Scale(scale), telatest.Dark())
			d.Fill(composer, "Can we make it 3:30?")
			d.Click(dev)
			d.Golden(fmt.Sprintf("chat_switched_dark_x%g.png", scale))
		})
	}
}

// TestCurrentConversation checks the open conversation is the current page:
// it is the one h.AriaCurrent marks, and the mark moves when another is
// opened, by the pointer or by the keyboard.
func TestCurrentConversation(t *testing.T) {
	a := mount(t, windowW, windowH)
	a.Expect(maya, telatest.Current("page"))
	a.Expect(dev, telatest.Current("false"))
	a.ExpectRole("heading", "Maya Chen")

	a.Click(dev)
	a.Expect(telatest.ByRole("button", "Dev Patel"), telatest.Current("page"))
	a.Expect(maya, telatest.Current("false"))
	a.ExpectRole("heading", "Dev Patel")

	// Opening it read its two unread messages: the name no longer says so.
	a.ExpectCount(dev, 0)
	a.ExpectText("1 unread") // Sofia's, in the header of the list
}

// TestKeyboard reaches the conversations with Tab and opens one with Enter.
func TestKeyboard(t *testing.T) {
	a := mount(t, windowW, windowH)
	a.Focus(search)
	a.Press("Tab")
	// The newest first: Dev Patel.
	a.ExpectFocused(dev)
	a.Press("Enter")
	a.Expect(telatest.ByRole("button", "Dev Patel"), telatest.Current("page"))
	a.Press("Tab")
	a.Press("Enter")
	a.Expect(maya, telatest.Current("page"))
}

// TestSearch filters the conversations by the name and by the last message.
func TestSearch(t *testing.T) {
	a := mount(t, windowW, windowH)
	a.ExpectCount(telatest.ByRole("button", "Jonas Weber"), 1)

	a.Fill(search, "liam")
	a.ExpectCount(telatest.ByRole("button", "Liam O'Brien"), 1)
	a.ExpectCount(maya, 0)
	a.ExpectCount(dev, 0)

	// The last message counts too: Dev's says "ship it".
	a.Fill(search, "SHIP IT")
	a.ExpectCount(dev, 1)
	a.ExpectCount(telatest.ByRole("button", "Liam O'Brien"), 0)

	a.Fill(search, "zzz")
	a.ExpectText(`No conversation matches "zzz".`)

	// Clearing it brings them all back; the conversation that was open
	// stays open the whole time.
	a.Fill(search, "")
	a.ExpectCount(maya, 1)
	a.ExpectCount(dev, 1)
	a.Expect(maya, telatest.Current("page"))
}

// TestSendWithEnter sends with Enter: the message appears, the composer is
// empty, the list is at its end, and the reply comes after the typing
// indicator.
func TestSendWithEnter(t *testing.T) {
	a := mount(t, windowW, windowH)
	a.Expect(send, telatest.Disabled(true))
	a.ExpectCount(typing, 0)
	if !atEnd(a, message("Maya Chen", "1:58 PM")) {
		t.Fatalf("the list did not open at its end")
	}

	a.Fill(composer, "On my way")
	a.Expect(send, telatest.Disabled(false))
	a.Press("Enter")

	sent := message("You", "2:33 PM")
	a.ExpectCount(sent.Filter(telatest.HasText("On my way")), 1)
	a.ExpectValue(composer, "")
	a.Expect(send, telatest.Disabled(true))
	a.ExpectText("You: On my way") // the sidebar's preview, now the newest
	a.ExpectCount(typing, 1)       // the other side is writing
	a.ExpectText("typing…")
	a.ExpectNoText(scripts["maya"][0])
	a.ExpectCount(telatest.ByRole("img", "Sent"), 1)
	if !atEnd(a, typing) {
		t.Errorf("the list did not scroll to the typing indicator")
	}

	// Half the delay later they are still typing; after it, the reply has
	// come in place of the dots, and the message is marked as read.
	a.Advance(replyDelay / 2)
	a.ExpectCount(typing, 1)
	a.Advance(replyDelay)
	a.ExpectCount(typing, 0)
	reply := message("Maya Chen", "2:34 PM")
	a.ExpectCount(reply.Filter(telatest.HasText(scripts["maya"][0])), 1)
	a.ExpectCount(telatest.ByRole("img", "Sent"), 0)
	a.ExpectNoText("typing…")
	if !atEnd(a, reply) {
		t.Errorf("the list did not scroll to the reply")
	}
	// The conversation is open, so the reply is not unread.
	a.ExpectCount(telatest.ByRole("button", "Maya Chen, 1 unread"), 0)
}

// TestSendWithButton sends with the button, and the focus goes back to the
// composer for the next message.
func TestSendWithButton(t *testing.T) {
	a := mount(t, windowW, windowH)
	a.Fill(composer, "Sounds good")
	a.Click(send)
	a.ExpectCount(message("You", "2:33 PM").Filter(telatest.HasText("Sounds good")), 1)
	a.ExpectValue(composer, "")
	a.ExpectFocused(composer)
	a.ExpectCount(typing, 1)
}

// TestNewline checks that Shift+Enter writes a new line instead of sending,
// and that the composer grows with it.
func TestNewline(t *testing.T) {
	a := mount(t, windowW, windowH)
	one := a.Bounds(composer).Height
	a.Fill(composer, "first line")
	a.Press("Shift+Enter")
	a.Type("second line")
	a.ExpectValue(composer, "first line\nsecond line")
	a.ExpectCount(telatest.ByRole("article", "").Filter(telatest.HasText("second line")), 0)
	a.ExpectCount(typing, 0)
	if two := a.Bounds(composer).Height; two <= one {
		t.Errorf("the composer did not grow with a second line: %v -> %v", one, two)
	}

	// Enter sends both lines as one message.
	a.Press("Enter")
	a.ExpectCount(message("You", "2:33 PM").Filter(telatest.HasText("first line")), 1)
	a.ExpectValue(composer, "")
}

// TestEmptyMessage keeps the send button disabled while there is nothing to
// send, and Enter from sending nothing.
func TestEmptyMessage(t *testing.T) {
	a := mount(t, windowW, windowH)
	a.Expect(send, telatest.Disabled(true))
	a.Focus(composer)
	a.Press("Enter")
	a.ExpectCount(typing, 0)

	a.Fill(composer, "   ")
	a.Expect(send, telatest.Disabled(true))
	a.Press("Enter")
	a.ExpectCount(typing, 0)
	a.ExpectCount(telatest.ByRole("article", "").Filter(telatest.HasText("2:33 PM")), 0)

	a.Fill(composer, "hi")
	a.Expect(send, telatest.Disabled(false))
	a.Fill(composer, "")
	a.Expect(send, telatest.Disabled(true))
}

// TestDrafts keeps what you typed in a conversation when you open another,
// shows it in the list, and brings it back.
func TestDrafts(t *testing.T) {
	a := mount(t, windowW, windowH)
	a.Fill(composer, "Can we make it 3:30?")
	a.ExpectNoText("Draft:") // the open conversation shows no draft

	a.Click(dev)
	a.ExpectValue(composer, "")
	a.ExpectText("Draft: Can we make it 3:30?")
	a.Fill(composer, "Thanks, Dev!")

	a.Click(maya)
	a.ExpectValue(composer, "Can we make it 3:30?")
	a.ExpectText("Draft: Thanks, Dev!")
	a.ExpectNoText("Draft: Can we make it 3:30?")

	// Sending clears the draft.
	a.Focus(composer)
	a.Press("Enter")
	a.ExpectNoText("Draft: Can we make it 3:30?")
	a.ExpectValue(composer, "")
}

// TestMessagesKept keeps the messages of a conversation when another is open,
// and each conversation opens at its end.
func TestMessagesKept(t *testing.T) {
	a := mount(t, windowW, windowH)
	a.Fill(composer, "Ping")
	a.Press("Enter")
	a.Click(dev)
	a.Expect(maya, telatest.Current("false"))
	if !atEnd(a, message("Dev Patel", "2:21 PM")) {
		t.Errorf("Dev's conversation did not open at its end")
	}
	a.Click(telatest.ByRole("button", "Maya Chen"))
	a.ExpectCount(message("You", "2:33 PM").Filter(telatest.HasText("Ping")), 1)
	if !atEnd(a, typing) {
		t.Errorf("Maya's conversation did not open at its end, with her typing")
	}
}

// TestReplyInAnotherConversation shows that a reply keeps coming while
// another conversation is open: it counts as unread until opened.
func TestReplyInAnotherConversation(t *testing.T) {
	a := mount(t, windowW, windowH)
	a.Fill(composer, "Ping")
	a.Press("Enter")
	a.Click(sofia)
	a.Advance(replyDelay)

	unread := telatest.ByRole("button", "Maya Chen, 1 unread")
	a.ExpectCount(unread, 1)
	a.ExpectText(scripts["maya"][0]) // the preview of her row
	a.ExpectText("3 unread")         // Dev's two and her one; Sofia's was read
	a.Click(unread)
	a.ExpectCount(telatest.ByRole("button", "Maya Chen, 1 unread"), 0)
	a.ExpectCount(message("Maya Chen", "2:34 PM").Filter(telatest.HasText(scripts["maya"][0])), 1)
}

// TestMenu mutes a conversation and clears it from the header's menu, and
// the tooltips of the buttons say what they do.
func TestMenu(t *testing.T) {
	a := mount(t, windowW, windowH)
	a.Hover(telatest.ByRole("button", "Video call"))
	a.ExpectText("Video call") // the button has no text: this is its tooltip

	more := telatest.ByRole("button", "More actions")
	a.Click(more)
	a.Click(telatest.ByRole("menuitem", "Mute notifications"))
	a.ExpectNoText("Mute notifications")
	a.Click(more)
	a.ExpectRole("menuitem", "Unmute notifications")
	a.Click(telatest.ByRole("menuitem", "Clear messages"))
	a.ExpectText("No messages yet")
	a.ExpectCount(telatest.ByRole("article", ""), 0)
}

// TestRebuildsAreNarrow checks that typing in the composer, which changes
// the whole app's state on every key, builds the composer, the sidebar and
// the row of its draft, and leaves the message list and the header alone:
// the unchanged rows, the header and the list skip their build.
func TestRebuildsAreNarrow(t *testing.T) {
	a := mount(t, windowW, windowH)
	a.Focus(composer)
	before := a.Stats()
	a.Type("a")
	after := a.Stats()
	if builds := after.Builds - before.Builds; builds > 40 {
		t.Errorf("one key rebuilt %d components: the root, the sidebar with its search box and the row of the draft, and the composer with its kit parts; the list, the header and the other rows should skip", builds)
	}
	if skips := after.Skips - before.Skips; skips < 8 {
		t.Errorf("only %d builds skipped: the six other rows, the header and the message list should skip", skips)
	}
}
