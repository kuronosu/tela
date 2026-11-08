package telatest

import (
	"testing"

	"github.com/kuronosu/tela"
	"github.com/kuronosu/tela/h"
	"github.com/kuronosu/tela/internal/platform"
	"github.com/kuronosu/tela/kit"
)

// Internal helpers for what an OS accessibility bridge receives
// (platform contract v5, ADR-246), through the headless window's fake
// bridge. There is no public API for it yet.

// accessOn starts a fake assistive technology on the app's window.
func (a *App) accessOn() {
	a.tb.Helper()
	a.win.Inject(platform.AccessibilityEvent{Active: true})
	a.settleAfter()
}

// announcements returns the announcements of the updates received from
// the from-th on.
func (a *App) announcements(from int) []platform.Announcement {
	var out []platform.Announcement
	for _, u := range a.win.AccessUpdates()[from:] {
		out = append(out, u.Announcements...)
	}
	return out
}

// TestAccessToastAnnounced: a kit toast that appears is announced
// politely, its title and description; an error toast assertively.
func TestAccessToastAnnounced(t *testing.T) {
	a := Mount(t, tela.C(func() tela.Node {
		return kit.Toaster{Children: tela.C(func() tela.Node {
			toast := kit.UseToast()
			return h.Div(
				kit.Button{Children: h.Text("Notify"), OnClick: func() {
					toast.Success("Event created", kit.ToastDescription("Sunday at 9:00"))
				}},
				kit.Button{Children: h.Text("Fail"), OnClick: func() { toast.Error("Upload failed") }})
		})}
	}))
	a.accessOn()
	n := len(a.win.AccessUpdates())
	if n == 0 || len(a.announcements(0)) != 0 {
		t.Fatalf("%d updates at the activation, announcements %+v", n, a.announcements(0))
	}
	a.Click(ByRole("button", "Notify"))
	got := a.announcements(n)
	if len(got) != 1 || got[0] != (platform.Announcement{Text: "Event created Sunday at 9:00"}) {
		t.Errorf("announced %+v, want the toast once, politely", got)
	}
	n = len(a.win.AccessUpdates())
	a.Click(ByRole("button", "Fail"))
	got = a.announcements(n)
	if len(got) != 1 || got[0] != (platform.Announcement{Text: "Upload failed", Assertive: true}) {
		t.Errorf("announced %+v, want the error toast once, assertively", got)
	}
}
