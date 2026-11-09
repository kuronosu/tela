package platform

import "testing"

func TestHandlerRoleChecks(t *testing.T) {
	file := Association{Extension: ".note", MIMEType: "application/x-note", Description: "Note"}
	scheme := Association{Scheme: "notes"}
	for _, r := range []HandlerRole{HandlerDefault, HandlerEditor, HandlerViewer, HandlerShell, HandlerNone} {
		f := file
		f.Role = r
		CheckAssociation("x", f)
		if r != HandlerNone {
			s := scheme
			s.Role = r
			CheckAssociation("x", s)
			CheckMakeDefault("x", s)
			CheckMakeDefault("x", f)
		}
	}
	none := file
	none.Role = HandlerNone
	expectPanic(t, "cannot be its default handler", func() { CheckMakeDefault("x", none) })
	expectPanic(t, "the handler of a scheme opens its links", func() { CheckAssociation("x", Association{Scheme: "notes", Role: HandlerNone}) })
	expectPanic(t, "invalid HandlerRole 5", func() { CheckAssociation("x", Association{Scheme: "notes", Role: HandlerNone + 1}) })
	var u Associations = UnsupportedAssociations{}
	expectPanic(t, "cannot be its default handler", func() {
		_, _ = u.MakeDefault(HandlerApp{ID: "n", Name: "N", Exec: "/n"}, none)
	})

	for a, want := range map[Association]HandlerRole{
		file:                                   HandlerEditor,
		scheme:                                 HandlerViewer,
		{Scheme: "notes", Role: HandlerEditor}: HandlerEditor,
		none:                                   HandlerNone,
	} {
		if got := a.ResolvedRole(); got != want {
			t.Errorf("%v.ResolvedRole() = %v, want %v", a, got, want)
		}
	}
	if HandlerShell.String() != "shell" || HandlerDefault.String() != "default" || HandlerRole(9).String() != "HandlerRole(invalid)" {
		t.Errorf("HandlerRole.String")
	}
	// The role is a field of a comparable value.
	if file == none {
		t.Errorf("the role is not compared")
	}
}
