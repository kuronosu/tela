package main

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/kuronosu/tela"
	"github.com/kuronosu/tela/desktop"
	"github.com/kuronosu/tela/examples/notes/icons"
	. "github.com/kuronosu/tela/h"
	"github.com/kuronosu/tela/kit"
	"github.com/kuronosu/tela/oauth"
)

// The account and the sync of the rail. accountPanel is a component of its
// own, so that everything about the sign-in (oauth.UseSession) and the sync
// (tela.UseAsync) lives in one place.

// keyAccountName is the setting that remembers whose notes are synced
// (shown before the first sync of a session answers).
const keyAccountName = "accountName"

// syncDelay is how long after the last edit the changes are pushed.
const syncDelay = 2 * time.Second

// oauthConfig is the OAuth client of a server: the endpoints are at the
// paths a server of Notes serves (demo.go).
func oauthConfig(s serverInfo) oauth.Config {
	return oauth.Config{
		ClientID:   s.ClientID,
		AuthURL:    s.URL + "/authorize",
		TokenURL:   s.URL + "/token",
		RevokeURL:  s.URL + "/revoke",
		Scopes:     []string{"notes"},
		SecretName: "notes-account",
	}
}

// accountPanel is the sign-in and the sync status at the foot of the rail.
type accountPanel struct {
	// Server is the server of the account; the zero value runs the demo
	// server while the panel is mounted.
	Server serverInfo
	// Edits counts the edits that wait for the server (the screen's
	// libState.Edits); Pending is how many notes and deletions the server
	// does not have yet.
	Edits, Pending int
	// Snapshot returns the library as it is now, for a sync to read; Apply
	// puts the outcome of a sync in it.
	Snapshot func() snapshot
	Apply    func(syncResult)
	// Preview draws the signed-out card and nothing else.
	Preview bool
}

func (p accountPanel) Build() tela.Node {
	if p.Preview {
		// A snapshot has no running app, which the keyring and the sign-in
		// need: it draws the signed-out card.
		return accountCard(Span(Class(accountText), Text("Sign in to keep your notes on every device.")),
			kit.Button{Variant: kit.Outline, Size: kit.Sm, Icon: icons.LogIn, Children: Text("Sign in")})
	}
	return liveAccount(p)
}

// accountText is the small muted text of the account card.
const accountText = "text-[12px] leading-4 text-muted-foreground"

// accountCard is the card of the account at the foot of the rail.
func accountCard(children ...tela.Child) tela.Node {
	return Div(Class("flex flex-col gap-2 rounded-lg border border-border bg-background/60 p-2.5"), Role("group"), AriaLabel("Account"),
		Fragment(children...))
}

// liveAccount is accountPanel with the sign-in and the sync running.
type liveAccount accountPanel

func (p liveAccount) Build() tela.Node {
	toast := kit.UseToast()
	// Without a server, the panel runs the demo one (demo.go) for as long as
	// it is mounted, so that sign-in and sync work on one machine, offline.
	demoed := tela.UseOnce(func() *demoServer {
		if p.Server.URL != "" {
			return nil
		}
		d, err := startDemo()
		if err != nil {
			return nil // no demo: the sign-in will say it cannot reach the server
		}
		return d
	})
	tela.UseMount(func() func() {
		return func() {
			if demoed != nil {
				demoed.Close()
			}
		}
	})
	server := p.Server
	if demoed != nil {
		server = demoed.Info
	}
	acct := oauth.UseSession(oauthConfig(server))
	name, setName := desktop.UseSetting(keyAccountName, "")
	signedIn := acct.Status == oauth.SignedIn

	// The sync: when the sign-in is done, when the user asks (kick) and
	// syncDelay after the edits stop. A change of any of them cancels the one
	// in flight and starts again; the result of a canceled one is dropped.
	kick, setKick := tela.UseState(0)
	settled := tela.UseDebounced(p.Edits, syncDelay)
	run := tela.UseAsync(func(ctx context.Context) (syncResult, error) {
		if !signedIn {
			return syncResult{}, nil
		}
		return runSync(ctx, syncer{base: server.URL, hc: acct.Session().Client(ctx)}, p.Snapshot())
	}, signedIn, kick, settled)

	// A result is applied once, on the UI thread.
	tela.UseEffectDo(func() {
		if run.Data.Run != 0 {
			p.Apply(run.Data)
			setName.Set(run.Data.Name)
		}
	}, run.Data.Run)
	// A server that no longer knows the token ends the session.
	refused := errors.Is(run.Err, errUnauthorized)
	tela.UseEffectDo(func() {
		if refused {
			toast.Error("You were signed out", kit.ToastDescription("The server did not accept the sign-in any more."))
			acct.SignOut()
		}
	}, refused)
	status := acct.Status
	tela.UseEffectDo(func() {
		if status == oauth.SignedOut {
			setName.Set("")
		}
	}, status)

	card, small := accountCard, accountText
	switch acct.Status {
	case oauth.SigningIn:
		return card(
			P(Class(small), Text("Finish signing in in your browser…")),
			kit.Button{Variant: kit.Outline, Size: kit.Sm, OnClick: acct.Cancel, Children: Text("Cancel")},
		)
	case oauth.Failed:
		return card(
			P(Class("text-[12px] leading-4 text-gold"), Text(signInProblem(acct.Err))),
			kit.Button{Variant: kit.Outline, Size: kit.Sm, Icon: icons.LogIn, OnClick: acct.SignIn, Children: Text("Try again")},
		)
	case oauth.SignedIn:
		who := name
		if who == "" {
			who = "Signed in"
		}
		// The state of the connection, and under it what waits for the server.
		icon, line := icons.CloudCheck, "Signed in"
		switch {
		case run.Loading:
			icon, line = icons.RefreshCw, "Syncing…"
		case run.Err != nil && !refused:
			icon, line = icons.CloudOff, "Offline"
		case !run.Data.At.IsZero():
			line = "Synced " + run.Data.At.Format("15:04")
		}
		waiting := ""
		if p.Pending > 0 && !run.Loading {
			waiting = plural(p.Pending, "change") + " waiting"
			if icon == icons.CloudCheck {
				icon = icons.Cloud
			}
		}
		iconClass := "size-3 shrink-0"
		if run.Loading {
			iconClass += " animate-spin"
		}
		return card(
			Div(Class("flex items-center gap-2"),
				Span(Class("flex size-7 shrink-0 items-center justify-center rounded-full bg-gold/15 text-[12px] font-semibold text-gold"),
					AriaHidden(true), Text(initial(who))),
				Div(Class("flex min-w-0 flex-1 flex-col"),
					Span(Class("truncate text-[13px] leading-4 font-medium"), Text(who)),
					Span(Class("flex items-center gap-1 truncate", small), Icon(icon, Class(iconClass)), Span(Class("truncate"), Text(line))),
					If(waiting != "", Span(Class("truncate text-[11px] leading-4 text-gold"), Text(waiting))),
				),
			),
			Div(Class("flex gap-1"),
				kit.Button{Variant: kit.Outline, Size: kit.Sm, Class: "min-w-0 flex-1 px-2 text-[12px]", Disabled: run.Loading,
					OnClick: func() { setKick.Update(func(n int) int { return n + 1 }) }, Children: Text("Sync now")},
				kit.Button{Variant: kit.Ghost, Size: kit.Sm, Class: "min-w-0 flex-1 px-2 text-[12px]", OnClick: acct.SignOut, Children: Text("Sign out")},
			),
		)
	case oauth.SignedOut:
		return card(
			P(Class(small), Text("Sign in to keep your notes on every device.")),
			kit.Button{Variant: kit.Outline, Size: kit.Sm, Icon: icons.LogIn, OnClick: acct.SignIn, Children: Text("Sign in")},
		)
	}
	return nil // the stored tokens are being read
}

// signInProblem says why a sign-in failed, in a few words.
func signInProblem(err error) string {
	switch {
	case errors.Is(err, oauth.ErrAccessDenied):
		return "The sign-in was declined."
	case errors.Is(err, oauth.ErrExpired):
		return "The sign-in expired."
	case err == nil:
		return "The sign-in did not finish."
	}
	msg := err.Error()
	if len(msg) > 120 {
		msg = msg[:120] + "…"
	}
	return fmt.Sprintf("The sign-in failed: %s", msg)
}

// initial is the first letter of a name, upper case.
func initial(name string) string {
	r, _ := utf8.DecodeRuneInString(strings.TrimSpace(name))
	return strings.ToUpper(string(r))
}
