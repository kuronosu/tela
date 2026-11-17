package desktop

import (
	"testing"

	"github.com/kuronosu/tela/internal/platform"
)

// A click on an earlier run's notification (a Windows toast clicked after
// the app that showed it exited, ADR-297) carries that run's ID, which may
// be one of this run's: it runs none of this run's callbacks and leaves
// the notification shown.
func TestNotificationEarlierRun(t *testing.T) {
	clicked := ""
	ns := &notifications{shown: map[platform.NotificationID]Notification{
		1: {Title: "this run's", OnClick: func() { clicked = "body" }, Actions: []NotificationAction{{Label: "Open", OnClick: func() { clicked = "action" }}}},
	}}
	ns.event(platform.NotificationEvent{ID: 1, Kind: platform.NotificationActivated, Earlier: true})
	ns.event(platform.NotificationEvent{ID: 1, Kind: platform.NotificationActionInvoked, Action: "1", Earlier: true})
	if clicked != "" || len(ns.shown) != 1 {
		t.Errorf("an earlier run's click ran %q, %d shown; want nothing run and the notification kept", clicked, len(ns.shown))
	}
	ns.event(platform.NotificationEvent{ID: 1, Kind: platform.NotificationActionInvoked, Action: "1"})
	if clicked != "action" || len(ns.shown) != 0 {
		t.Errorf("this run's click ran %q, %d shown", clicked, len(ns.shown))
	}
}
