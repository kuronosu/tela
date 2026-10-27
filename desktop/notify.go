package desktop

import (
	"fmt"
	"image"
	"strconv"

	"github.com/kuronosu/tela/internal/core"
	"github.com/kuronosu/tela/internal/platform"
)

// Notification is a desktop notification: a toast in the OS's
// notification area.
type Notification struct {
	// Title is the notification's summary line; required.
	Title string
	// Body is the text below it; may be empty.
	Body string
	// Icon is the notification's image, the same picture at one or more
	// sizes; empty uses the app's icon.
	Icon []image.Image
	// Actions are buttons on the notification. Windows shows none (its
	// balloon notifications have no buttons): keep every action reachable
	// from the app too.
	Actions []NotificationAction
	// OnClick runs when the user clicks the notification itself.
	OnClick func()
	// OnClose runs when the notification goes away without OnClick or an
	// action: the user dismissed it, or it expired, as far as the OS
	// reports it (not after [ShownNotification].Close).
	OnClose func()
}

// NotificationAction is a button of a notification.
type NotificationAction struct {
	// Label is the button's text.
	Label string
	// OnClick runs when the user clicks the button.
	OnClick func()
}

// ShownNotification is a notification on screen, returned by [Notify].
// The zero value is no notification.
type ShownNotification struct {
	d  core.Desktop
	id platform.NotificationID
}

// IsZero reports whether n is the zero ShownNotification.
func (n ShownNotification) IsZero() bool { return n.id == 0 }

// Close withdraws the notification; its callbacks are not called any
// more. Closing one that is gone does nothing. UI thread.
func (n ShownNotification) Close() error {
	if n.id == 0 || n.d.Ended() {
		return nil
	}
	notificationsOf(n.d).forget(n.id)
	return wrap("Close", n.d.Services().CloseNotification(n.id))
}

// Notify shows a desktop notification. Its callbacks run on the UI
// thread, so they may set state; clicking the notification usually brings
// the app forward:
//
//	win := tela.UseWindow()
//	...
//	desktop.Notify(desktop.Notification{
//	    Title:   "Export finished",
//	    Body:    "notes.pdf was saved in Documents.",
//	    OnClick: func() { win.Show() },
//	    Actions: []desktop.NotificationAction{
//	        {Label: "Open", OnClick: func() { tela.OpenPath(pdf) }},
//	    },
//	})
//
// Call it on the UI thread (a handler, an effect; from a goroutine, wrap
// it in tela.Post). The error says the notification could not be shown:
// no running app, or no notification service (errors.Is(err,
// [ErrUnsupported])), where the app should show the news itself. Tests
// read the notifications with telatest's App.Notifications and click them
// with App.ClickNotification.
func Notify(n Notification) (ShownNotification, error) {
	if n.Title == "" {
		panic("desktop.Notify: the Notification has no Title; give it a short summary, such as desktop.Notification{Title: \"Export finished\", Body: \"notes.pdf was saved.\"}")
	}
	pn := platform.Notification{Title: n.Title, Body: n.Body, Icon: n.Icon}
	for i, a := range n.Actions {
		if a.Label == "" {
			panic(fmt.Sprintf("desktop.Notify: Actions[%d] has no Label; an action is a button with a text", i))
		}
		pn.Actions = append(pn.Actions, platform.NotificationAction{ID: strconv.Itoa(i + 1), Label: a.Label})
	}
	d, err := running("Notify")
	if err != nil {
		return ShownNotification{}, err
	}
	id, err := d.Services().Notify(pn)
	if err != nil {
		return ShownNotification{}, wrap("Notify", err)
	}
	notificationsOf(d).shown[id] = n
	return ShownNotification{d: d, id: id}, nil
}

// notifications holds the callbacks of the notifications an app shows,
// until they close.
type notifications struct {
	shown map[platform.NotificationID]Notification
}

type notificationsKey struct{}

// notificationsOf returns the app's notifications, listening to their
// events from the first use on.
func notificationsOf(d core.Desktop) *notifications {
	return d.Local(notificationsKey{}, func() any {
		ns := &notifications{shown: map[platform.NotificationID]Notification{}}
		d.Listen(ns.event)
		return ns
	}).(*notifications)
}

func (ns *notifications) forget(id platform.NotificationID) { delete(ns.shown, id) }

// event runs the callback of a notification event. A click or an action
// ends the notification for the app (the OS closes it), so its OnClose
// does not follow.
func (ns *notifications) event(e platform.Event) {
	ne, ok := e.(platform.NotificationEvent)
	if !ok {
		return
	}
	n, ok := ns.shown[ne.ID]
	if !ok {
		return
	}
	delete(ns.shown, ne.ID)
	switch ne.Kind {
	case platform.NotificationActivated:
		if n.OnClick != nil {
			n.OnClick()
		}
	case platform.NotificationActionInvoked:
		if i, err := strconv.Atoi(ne.Action); err == nil && i >= 1 && i <= len(n.Actions) {
			if f := n.Actions[i-1].OnClick; f != nil {
				f()
			}
		}
	case platform.NotificationClosed:
		if n.OnClose != nil {
			n.OnClose()
		}
	}
}
