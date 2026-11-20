package main

import (
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/kuronosu/tela/kit"
	"github.com/kuronosu/tela/oauth/oauthtest"
	"github.com/kuronosu/tela/telatest"
)

// The account and the sync, against the demo server in the test's process:
// its OAuth half is package oauthtest's fake server, and the notes API is
// the stand-in of demo.go.

var (
	signInBtn  = telatest.ByRole("button", "Sign in")
	signOutBtn = telatest.ByRole("button", "Sign out")
	syncNowBtn = telatest.ByRole("button", "Sync now")
)

// demo starts a demo server for the test.
func demo(t *testing.T) *demoServer {
	t.Helper()
	d, err := startDemo()
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(d.Close)
	return d
}

// mountSynced mounts the app with the folders sandboxed, against d.
func mountSynced(t *testing.T, d *demoServer, opts ...telatest.Option) *telatest.App {
	t.Helper()
	return mountApp(t, appWith(kit.GNOME, d.Info), kit.GNOME, opts...)
}

// browse plays the user's browser: it follows the newest URL the app opened
// in the system's browser (the nth one: the authorization URL; oauth's default Open is
// tela.OpenURL, which the headless desktop records), as a browser does:
// the server's page redirects to the app's loopback port.
func browse(t *testing.T, a *telatest.App, d *demoServer, n int) {
	t.Helper()
	var urls []string
	a.WaitFor("the app to open the authorization URL", func() bool {
		urls = a.OpenedURLs()
		return len(urls) >= n
	})
	url := urls[len(urls)-1]
	if !strings.HasPrefix(url, d.URL+"/authorize?") {
		t.Fatalf("the app opened %q, want the server's authorization URL", url)
	}
	// The page is the server's, then a redirect to the app's loopback port.
	resp, err := http.Get(url)
	if err != nil {
		t.Fatal(err)
	}
	resp.Body.Close()
}

// pushed waits for the server to have n notes (deleted ones included).
func pushed(t *testing.T, a *telatest.App, d *demoServer, n int) {
	t.Helper()
	a.WaitFor("the server to have the notes", func() bool { return len(d.Notes()) == n })
}

func TestSignedOutRail(t *testing.T) {
	sandbox(t)
	d := demo(t)
	a := mountSynced(t, d)
	a.ExpectText("Sign in to keep your notes on every device.")
	a.ExpectCount(signInBtn, 1)
	a.ExpectNoText("Sign out")
	if n := len(d.Notes()); n != 0 {
		t.Errorf("the server has %d notes before the sign-in", n)
	}
}

// Signing in goes through the system browser: the authorization URL is what
// the app opened, and the notes are pushed once the account is there.
func TestSignInThroughTheBrowser(t *testing.T) {
	sandbox(t)
	d := demo(t)
	a := mountSynced(t, d)
	a.Click(signInBtn)
	a.ExpectText("Finish signing in in your browser")
	browse(t, a, d, 1)

	a.ExpectText("Demo user")
	a.ExpectCount(signOutBtn, 1)
	pushed(t, a, d, 4)
	a.ExpectText("Synced")
	if q := d.OAuth().AuthRequests(); len(q) != 1 || q[0].Get("code_challenge_method") != "S256" || q[0].Get("state") == "" {
		t.Errorf("the authorization request = %v, want PKCE with S256 and a state", q)
	}
}

func TestSignInDenied(t *testing.T) {
	sandbox(t)
	d := demo(t)
	d.OAuth().SetMode(oauthtest.Deny)
	a := mountSynced(t, d)
	a.Click(signInBtn)
	browse(t, a, d, 1)
	a.ExpectText("The sign-in was declined.")
	a.ExpectNoText("Demo user")

	// Try again, and this time the user approves.
	d.OAuth().SetMode(oauthtest.Approve)
	a.Click(telatest.ByRole("button", "Try again"))
	browse(t, a, d, 2)
	a.ExpectText("Demo user")
	if n := len(d.Notes()); n > 4 {
		t.Errorf("the server has %d notes", n)
	}
}

func TestSignOut(t *testing.T) {
	sandbox(t)
	d := demo(t)
	a := mountSynced(t, d)
	a.Click(signInBtn)
	browse(t, a, d, 1)
	a.ExpectText("Demo user")
	a.Click(signOutBtn)
	a.ExpectText("Sign in to keep your notes on every device.")
	a.ExpectNoText("Demo user")
	a.WaitFor("the server to revoke the token", func() bool { return d.OAuth().RevokeRequests() > 0 })
	// The notes stay on the device.
	a.ExpectRole("button", "Welcome to Notes")
}

// The tokens are in the keyring (desktop.SetSecret) while the account is
// there, and gone after signing out; a start that finds them is signed in.
func TestAccountIsKeptInTheKeyring(t *testing.T) {
	sandbox(t)
	d := demo(t)
	a := mountSynced(t, d)
	a.Click(signInBtn)
	browse(t, a, d, 1)
	a.ExpectText("Demo user")
	tokens, ok := a.StoredSecret("notes-account")
	if !ok || len(tokens) == 0 {
		t.Fatal("the tokens are not in the keyring after the sign-in")
	}
	a.Click(signOutBtn)
	a.ExpectText("Sign in to keep your notes on every device.")
	if _, ok := a.StoredSecret("notes-account"); ok {
		t.Error("the tokens are still in the keyring after signing out")
	}
}

// signedIn mounts the app against d, signs in and waits for the first sync
// to leave the welcome notes on the server.
func signedIn(t *testing.T, d *demoServer, opts ...telatest.Option) *telatest.App {
	t.Helper()
	a := mountSynced(t, d, opts...)
	a.Click(signInBtn)
	browse(t, a, d, 1)
	a.ExpectText("Demo user")
	pushed(t, a, d, 4)
	a.ExpectText("Synced")
	return a
}

// serverNote returns the note the server has under uid.
func serverNote(d *demoServer, uid string) (wireNote, bool) {
	for _, n := range d.Notes() {
		if n.UID == uid {
			return n, true
		}
	}
	return wireNote{}, false
}

// uidOf is the UID of the stored note id.
func uidOf(t *testing.T, id int) string {
	t.Helper()
	n, ok := find(storedNotes(t), id)
	if !ok || n.UID == "" {
		t.Fatalf("the library has no note %d with a UID: %+v", id, n)
	}
	return n.UID
}

// Notes another device made come in with the sign-in, and the local ones go
// out.
func TestSyncPullsAndPushes(t *testing.T) {
	sandbox(t)
	d := demo(t)
	d.Put(wireNote{UID: "laptop-1", Title: "From the laptop", Text: "Written on another device.\n\n#sync", Folder: "Work", Updated: fixedNow.Add(-time.Hour)})
	a := mountSynced(t, d)
	a.ExpectNoText("From the laptop")
	a.Click(signInBtn)
	browse(t, a, d, 1)

	a.ExpectRole("button", "From the laptop")
	a.ExpectRole("button", "Work") // the folder of the note that came in
	pushed(t, a, d, 5)
	if n, ok := serverNote(d, uidOf(t, grocery)); !ok || n.Title != "Groceries" || n.Folder != "Home" || n.Deleted {
		t.Errorf("the server's Groceries = %+v, %v", n, ok)
	}
	// The note that came in is a note like the others.
	a.Click(card("From the laptop"))
	a.ExpectValue(editor, "Written on another device.\n\n#sync")
	a.ExpectCount(unsaved, 0)
}

// An edit is pushed a moment after it stops; the later change wins in both
// directions.
func TestSyncLastWriterWins(t *testing.T) {
	sandbox(t)
	d := demo(t)
	a := signedIn(t, d)
	groceries, welcome := uidOf(t, grocery), uidOf(t, welcomeID)

	// Edited here, later than the server's copy: pushed after the pause.
	setClock(t, fixedNow.Add(time.Hour))
	a.Click(card("Groceries"))
	a.Click(editor)
	a.Type(" and saffron")
	a.ExpectText("1 change waiting")
	a.Advance(syncDelay + time.Second)
	a.WaitFor("the edit to reach the server", func() bool {
		n, _ := serverNote(d, groceries)
		return strings.HasSuffix(n.Text, "and saffron") && n.Updated.Equal(fixedNow.Add(time.Hour))
	})
	a.ExpectText("Synced")
	a.ExpectNoText("change waiting")

	// Edited elsewhere, later than the local copy: Sync now brings it in.
	d.Put(wireNote{UID: welcome, Title: "Welcome (edited on the laptop)", Text: "A new first line.", Folder: "Getting started", Pinned: true,
		Updated: fixedNow.Add(2 * time.Hour)})
	a.Click(syncNowBtn)
	a.ExpectRole("button", "Welcome (edited on the laptop)")
	a.Click(card("Welcome (edited on the laptop)"))
	a.ExpectValue(editor, "A new first line.")

	// Edited elsewhere, earlier than the local copy: the server's loses.
	d.Put(wireNote{UID: groceries, Title: "Groceries (stale)", Text: "stale", Updated: fixedNow.Add(30 * time.Minute)})
	a.Click(syncNowBtn)
	a.WaitFor("the local note to be pushed again", func() bool {
		n, _ := serverNote(d, groceries)
		return n.Title == "Groceries" && strings.HasSuffix(n.Text, "and saffron")
	})
	a.ExpectRole("button", "Groceries, unsaved changes")
}

// A note deleted here is deleted on the server, and the other way around.
func TestSyncDeletes(t *testing.T) {
	sandbox(t)
	d := demo(t)
	a := signedIn(t, d)
	weekend, groceries := uidOf(t, 4), uidOf(t, grocery)

	setClock(t, fixedNow.Add(time.Hour))
	a.Click(card("Weekend in the mountains"))
	a.Click(deleteBtn)
	a.ExpectText("Deleted Weekend in the mountains")
	a.Advance(syncDelay + time.Second)
	a.WaitFor("the server to learn of the deletion", func() bool {
		n, ok := serverNote(d, weekend)
		return ok && n.Deleted
	})

	// Deleted elsewhere, later: the local note goes.
	d.Put(wireNote{UID: groceries, Deleted: true, Updated: fixedNow.Add(2 * time.Hour)})
	a.Click(syncNowBtn)
	a.ExpectCount(card("Groceries"), 0)
	if _, ok := find(storedNotes(t), grocery); ok {
		t.Error("the library still has the note that was deleted on the server")
	}
}

// Undo after a sync brings the note back for good: it beats the deletion the
// server already has.
func TestSyncUndoBeatsTheDeletion(t *testing.T) {
	sandbox(t)
	d := demo(t)
	a := signedIn(t, d)
	weekend := uidOf(t, 4)

	setClock(t, fixedNow.Add(time.Hour))
	a.Click(card("Weekend in the mountains"))
	a.Click(deleteBtn)
	a.Advance(syncDelay + time.Second)
	a.WaitFor("the server to learn of the deletion", func() bool {
		n, ok := serverNote(d, weekend)
		return ok && n.Deleted
	})
	setClock(t, fixedNow.Add(2*time.Hour))
	a.Click(telatest.ByRole("button", "Undo"))
	a.Advance(syncDelay + time.Second)
	a.WaitFor("the note to be on the server again", func() bool {
		n, ok := serverNote(d, weekend)
		return ok && !n.Deleted && n.Title == "Weekend in the mountains"
	})
}

// Without the server the app keeps working and says what waits; the next
// sync pushes it.
func TestSyncOffline(t *testing.T) {
	sandbox(t)
	d := demo(t)
	a := signedIn(t, d)

	d.SetDown(true)
	setClock(t, fixedNow.Add(time.Hour))
	typeNote(a, "Written offline")
	a.Advance(syncDelay + time.Second)
	a.ExpectText("Offline")
	a.ExpectText("1 change waiting")
	a.ExpectValue(editor, "Written offline") // still typing
	a.ExpectRole("button", "Written offline, unsaved changes")

	d.SetDown(false)
	a.Click(syncNowBtn)
	a.WaitFor("the note to reach the server", func() bool { return len(d.Notes()) == 5 })
	a.ExpectNoText("Offline")
	a.ExpectNoText("change waiting")
}

// A server that no longer knows the sign-in ends the session, and the notes
// stay.
func TestSyncSignedOutByTheServer(t *testing.T) {
	sandbox(t)
	d := demo(t)
	a := signedIn(t, d)
	d.SetRejecting(true)
	a.Click(syncNowBtn)
	a.ExpectText("You were signed out")
	a.ExpectText("Sign in to keep your notes on every device.")
	a.ExpectRole("button", "Welcome to Notes")
}

// Locked notes never leave the device, and locking a note takes it off the
// server.
func TestSyncKeepsLockedNotesHome(t *testing.T) {
	sandbox(t)
	d := demo(t)
	a := signedIn(t, d)
	groceries := uidOf(t, grocery)
	if _, ok := serverNote(d, groceries); !ok {
		t.Fatal("the server does not have Groceries")
	}
	setClock(t, fixedNow.Add(time.Hour))
	lockGroceries(t, a)
	a.Advance(syncDelay + time.Second)
	a.WaitFor("the server to drop the note that was locked", func() bool {
		n, ok := serverNote(d, groceries)
		return ok && n.Deleted && n.Title == "" && n.Text == ""
	})
	for _, n := range d.Notes() {
		if strings.Contains(n.Text, "Coffee") || strings.Contains(n.Title, "Groceries") {
			t.Errorf("the server has the text of a locked note: %+v", n)
		}
	}
}
