package main

import (
	"context"
	"errors"
	"strings"

	"github.com/kuronosu/tela"
	"github.com/kuronosu/tela/desktop"
	. "github.com/kuronosu/tela/h"
	"github.com/kuronosu/tela/kit"
)

// "Links and files" in Preferences: how the OS opens notes: links and .md
// files with Notes (desktop.Register and friends, ADR-209's conditions).
//
//   - Registering makes Notes a candidate: the link opens it when no other
//     application is the default, and ".md files" lists it in "Open with".
//     It writes only Notes's own entries (a .desktop file in
//     ~/.local/share/applications; HKCU\Software\Classes on Windows), and
//     nothing when nothing changes. It never takes what another application
//     holds.
//   - Becoming the default is another button, "Make default", which is only
//     there when Notes is registered and is not the default. On Windows the
//     OS lets only the user choose the default for a file type, so it opens
//     the Default apps settings instead (desktop.DefaultAsked).

// assocState is what the Preferences window knows about one association.
type assocState struct {
	// Loaded is true once the OS said what Reg is.
	Loaded bool
	Reg    desktop.Registration
	// Busy is true while a request runs.
	Busy bool
	// Err says why the last request failed; Msg is what the user should
	// know about one that worked.
	Err, Msg string
}

// assocActions are the requests the buttons make.
type assocActions struct{ Register, Unregister, MakeDefault func() }

// useAssociation tracks the registration of a: it reads the status when the
// component mounts and when its window gets the focus again (the user may
// have changed the default in the system's settings), and runs the requests
// off the UI thread, since they read and write files or the registry.
func useAssociation(a desktop.Association) (assocState, assocActions) {
	st, setSt := tela.UseState(assocState{})
	win := tela.UseWindow()
	live := !win.IsZero() // not a snapshot
	// refresh reads the status; run first does something, and shows its
	// outcome with the status after it.
	refresh := func(run func() (msg string, err error), quiet bool) {
		if !live {
			return
		}
		if !quiet {
			setSt.Update(func(s assocState) assocState { s.Busy, s.Err, s.Msg = true, "", ""; return s })
		}
		go func() {
			var msg string
			var err error
			if run != nil {
				msg, err = run()
			}
			reg, serr := desktop.RegistrationStatus(a)
			if err == nil {
				err = serr
			}
			setSt.Update(func(s assocState) assocState {
				s.Busy, s.Loaded, s.Reg = false, serr == nil, reg
				if run != nil || err != nil {
					s.Err, s.Msg = assocError(a, reg, err), msg
				}
				return s
			})
		}()
	}
	tela.UseGo(func(ctx context.Context) { refresh(nil, true) }, tela.Once)
	focused := win.State().Focused
	tela.UseUpdateEffect(func() func() {
		if focused && !st.Busy {
			refresh(nil, true)
		}
		return nil
	}, focused)

	return st, assocActions{
		Register: func() {
			refresh(func() (string, error) { _, err := desktop.Register(a); return "", err }, false)
		},
		Unregister: func() {
			refresh(func() (string, error) { _, err := desktop.Unregister(a); return "", err }, false)
		},
		MakeDefault: func() {
			refresh(func() (string, error) {
				r, err := desktop.MakeDefault(a)
				return defaultMessage(r, a), err
			}, false)
		},
	}
}

// defaultMessage is what the user is told after Make default: nothing when
// Notes is the default now, and where to go when Windows left the choice to
// them.
func defaultMessage(r desktop.DefaultResult, a desktop.Association) string {
	if r == desktop.DefaultAsked {
		return "Windows opened the Default apps settings. Choose Notes for " + a.String() + " there."
	}
	return ""
}

// assocError says what went wrong with a request, for the line under the
// row; "" for no error.
func assocError(a desktop.Association, reg desktop.Registration, err error) string {
	switch {
	case err == nil:
		return ""
	case errors.Is(err, desktop.ErrAssociationTaken):
		who := reg.Other
		if who == "" {
			who = "Another application"
		}
		return who + " holds " + a.String() + ". Notes does not take it over."
	case errors.Is(err, desktop.ErrDevExecutable):
		return "Not under tela dev: the program is a temporary build."
	case errors.Is(err, desktop.ErrUnsupported):
		return "This desktop cannot register applications."
	}
	return err.Error()
}

// assocNote joins the lines under a row.
func assocNote(status string, st assocState, blocked string) (text string, notice bool) {
	parts := []string{status}
	switch {
	case st.Err != "":
		parts, notice = append(parts, st.Err), true
	case st.Msg != "":
		parts = append(parts, st.Msg)
	case blocked != "":
		parts, notice = append(parts, blocked), true
	}
	return strings.Join(parts, " "), notice
}

// schemeStatus says how Notes stands with the notes: scheme.
func schemeStatus(st assocState) string {
	r := st.Reg
	switch {
	case !st.Loaded:
		return "Checking…"
	case r.Registered && r.Default:
		return "Registered, and the default for notes: links."
	case r.Registered && r.Other != "":
		return "Registered. " + r.Other + " is the default."
	case r.Registered:
		return "Registered: notes: links open Notes."
	case r.Other != "":
		return "Held by " + r.Other + "."
	}
	return "Not registered."
}

// markdownStatus says how Notes stands with .md files.
func markdownStatus(st assocState) string {
	r := st.Reg
	switch {
	case !st.Loaded:
		return "Checking…"
	case r.Registered && r.Default:
		return "Notes is the default app for .md files."
	case r.Registered && r.Other != "":
		return "Notes is in “Open with”. " + r.Other + " is the default."
	case r.Registered:
		return "Notes is in “Open with” for .md files."
	case r.Other != "":
		return r.Other + " opens .md files. Notes is not offered."
	}
	return "Notes is not offered for .md files."
}

// linksSection is the "Links and files" group of the General section.
type linksSection struct{}

func (linksSection) Build() tela.Node {
	return Div(Class("flex flex-col"),
		heading("Links and files", "How other applications open things in Notes."),
		schemeRow{},
		markdownRow{},
	)
}

// schemeRow is the notes: link: its status, Register or Unregister, and
// Make default.
type schemeRow struct{}

func (schemeRow) Build() tela.Node {
	st, act := useAssociation(notesScheme)
	_, setWanted := desktop.UseSetting(keyRegisterLinks, true)
	blocked := registrationBlocked()
	text, notice := assocNote(schemeStatus(st), st, blocked)
	registered := st.Reg.Registered
	toggle, label := act.Register, "Register"
	if registered {
		toggle, label = act.Unregister, "Unregister"
	}
	return stackRow("Open notes: links", text, notice, nil,
		If(registered && !st.Reg.Default && blocked == "",
			kit.Button{Variant: kit.Outline, Size: kit.Sm, Disabled: st.Busy, OnClick: act.MakeDefault, Children: Text("Make default")}),
		kit.Button{Variant: kit.Outline, Size: kit.Sm, Disabled: blocked != "" || !st.Loaded || st.Busy, Children: Text(label),
			OnClick: func() {
				// What the user asked for is kept for the next start: a
				// scheme they unregistered is not registered again.
				setWanted.Set(!registered)
				toggle()
			}},
	)
}

// markdownRow is "Open .md files with Notes": a switch that registers
// Notes as one of the apps that open them, and Make default.
type markdownRow struct{}

func (markdownRow) Build() tela.Node {
	st, act := useAssociation(markdownType)
	blocked := registrationBlocked()
	text, notice := assocNote(markdownStatus(st), st, blocked)
	registered := st.Reg.Registered
	return stackRow("Open .md files with Notes", text, notice,
		kit.Switch{AriaLabel: "Open .md files with Notes", Checked: &registered, Disabled: blocked != "" || !st.Loaded || st.Busy,
			OnCheckedChange: func(on bool) {
				if on {
					act.Register()
				} else {
					act.Unregister()
				}
			}},
		If(registered && !st.Reg.Default && blocked == "",
			kit.Button{Variant: kit.Outline, Size: kit.Sm, Disabled: st.Busy, OnClick: act.MakeDefault, Children: Text("Make default")}),
	)
}

// stackRow is a row of "Links and files": the label with an optional
// control at its end, the lines that tell where things stand, and the
// buttons under them (the window is narrow).
func stackRow(label, hint string, notice bool, trailing tela.Component, buttons ...tela.Child) tela.Node {
	hintClass := "text-xs text-muted-foreground"
	if notice {
		hintClass = "text-xs text-gold"
	}
	return Div(Key(label), Class("flex flex-col gap-1.5 border-t border-border py-3"),
		Div(Class("flex items-center justify-between gap-4"),
			Span(Class("text-[13px] font-medium"), Text(label)), If(trailing != nil, trailing)),
		Span(Class(hintClass), Text(hint)),
		Div(append([]tela.Child{Class("flex flex-wrap gap-2")}, buttons...)...),
	)
}
