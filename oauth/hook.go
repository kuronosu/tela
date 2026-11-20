package oauth

import (
	"context"
	"errors"

	"github.com/kuronosu/tela"
)

// Status is where [UseSession] is.
type Status int

const (
	// Loading: the stored tokens are being read.
	Loading Status = iota
	// SignedOut: no account. Show a sign-in button.
	SignedOut
	// SigningIn: the browser is open (or the device code is shown) and the
	// app waits for the user. Offer State.Cancel.
	SigningIn
	// SignedIn: there is an account; State.Session gives the client.
	SignedIn
	// Failed: State.Err says why the sign-in failed (it matches
	// ErrAccessDenied for a user who said no, ErrExpired for a device code
	// that expired, ErrStateMismatch, or is an error of the network or the
	// server).
	Failed
)

// operation is what the hook's goroutine does next.
type operation uint8

const (
	opLoad operation = iota
	opSignIn
	opDevice
	opSignOut
	opCancel
)

// request is one command to the hook's goroutine; a new one cancels the
// one in progress.
type request struct {
	n  int
	op operation
}

// State is what [UseSession] returns. It is comparable, so a component
// can pass it as a prop.
type State struct {
	Status Status
	// Err is why the status is Failed. With SignedIn it means the
	// tokens could not be stored in the secure storage: the account works
	// until the app quits.
	Err error
	// Device is the code to show while a device sign-in is in progress
	// (SigningIn after SignInWithDevice); nil otherwise.
	Device *DeviceCode

	session *Session
	set     tela.Setter[request]
}

// Session returns the account, for Client and Token: use them from
// UseAsync or UseGo, not from a handler or a build.
func (s State) Session() *Session { return s.session }

func (s State) send(op operation) {
	s.set.Update(func(r request) request { return request{n: r.n + 1, op: op} })
}

// SignIn starts the authorization code flow in the browser; the status
// goes to SigningIn and then to SignedIn or Failed. Calling it again
// starts over.
func (s State) SignIn() { s.send(opSignIn) }

// SignInWithDevice starts the device flow (Config.DeviceURL); State.Device
// has the code to show.
func (s State) SignInWithDevice() { s.send(opDevice) }

// Cancel gives up a sign-in in progress: the listener closes and the
// status goes to SignedOut.
func (s State) Cancel() { s.send(opCancel) }

// SignOut forgets the account and revokes the tokens where the server
// allows it.
func (s State) SignOut() { s.send(opSignOut) }

// hookState is the part of State that changes by itself; comparable.
type hookState struct {
	Status Status
	Err    error
	Device *DeviceCode
}

// UseSession keeps one [Session] for the component's life and returns
// where it is, for a sign-in screen:
//
//	func (Root) Build() tela.Node {
//	    acct := oauth.UseSession(github) // a package-level oauth.Config
//	    switch acct.Status {
//	    case oauth.SignedIn:
//	        return Div(Button(OnClick(acct.SignOut), Text("Sign out")), Profile{acct.Session()})
//	    case oauth.SigningIn:
//	        return Div(Text("Waiting for the browser…"), Button(OnClick(acct.Cancel), Text("Cancel")))
//	    }
//	    return Button(OnClick(acct.SignIn), Text("Sign in with GitHub"))
//	}
//
// The stored tokens are read when the component mounts. SignIn, SignOut
// and the rest run on a goroutine and update the state safely; unmounting
// the component cancels the sign-in in progress. The session also follows
// changes made elsewhere: a refresh the server refuses signs the account
// out. cfg is read on mount.
func UseSession(cfg Config) State {
	sess := tela.UseOnce(func() *Session { return New(cfg) })
	hs, setHS := tela.UseState(hookState{Status: Loading})
	req, setReq := tela.UseState(request{})

	tela.UseMount(func() func() {
		return sess.watch(func(signedIn bool) {
			if !signedIn {
				setHS.Update(func(h hookState) hookState {
					if h.Status == SignedIn {
						return hookState{Status: SignedOut}
					}
					return h
				})
			}
		})
	})

	tela.UseGo(func(ctx context.Context) {
		switch req.op {
		case opLoad:
			ok, err := sess.SignedIn(ctx)
			switch {
			case ctx.Err() != nil:
			case err != nil:
				setHS.Set(hookState{Status: Failed, Err: err})
			case ok:
				setHS.Set(hookState{Status: SignedIn})
			default:
				setHS.Set(hookState{Status: SignedOut})
			}
		case opSignIn, opDevice:
			setHS.Set(hookState{Status: SigningIn})
			var err error
			if req.op == opSignIn {
				err = sess.SignIn(ctx)
			} else {
				err = sess.DeviceSignIn(ctx, func(dc DeviceCode) {
					setHS.Set(hookState{Status: SigningIn, Device: &dc})
				})
			}
			if ctx.Err() != nil {
				return // canceled, or replaced by a newer request, which sets the state
			}
			if err != nil {
				if ok, _ := sess.SignedIn(ctx); ok { // signed in, but the tokens were not stored
					setHS.Set(hookState{Status: SignedIn, Err: err})
					return
				}
				setHS.Set(hookState{Status: Failed, Err: err})
				return
			}
			setHS.Set(hookState{Status: SignedIn})
		case opSignOut:
			err := sess.SignOut(ctx)
			if ctx.Err() != nil {
				return
			}
			if err != nil && !errors.Is(err, ErrSignedOut) {
				setHS.Set(hookState{Status: SignedOut, Err: err})
				return
			}
			setHS.Set(hookState{Status: SignedOut})
		case opCancel:
			setHS.Set(hookState{Status: SignedOut})
		}
	}, req)

	return State{Status: hs.Status, Err: hs.Err, Device: hs.Device, session: sess, set: setReq}
}
