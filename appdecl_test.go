package tela

import (
	"fmt"
	"strings"
	"testing"
)

func TestCheckDeclaration(t *testing.T) {
	ok := App{
		Schemes: []string{"notes"},
		FileTypes: []FileType{
			{Ext: ".note", MIME: "application/x-notes-note", Description: "Notes document"},
			{Ext: ".bak", MIME: "application/x-notes-bak", Description: "Backup", Role: HandlerViewer},
		},
	}
	ok.checkDeclaration()
	App{}.checkDeclaration()

	for _, tc := range []struct {
		name string
		app  App
		want string
	}{
		{"colon", App{Schemes: []string{"notes:"}}, `"notes:" has a ':'; give the scheme without it ("notes")`},
		{"slashes", App{Schemes: []string{"notes://"}}, `give the scheme without it ("notes")`},
		{"standard", App{Schemes: []string{"https"}}, "standard scheme"},
		{"no dot", App{FileTypes: []FileType{{Ext: "note", MIME: "application/x-n", Description: "N"}}}, `write it as ".note"`},
		{"empty ext", App{FileTypes: []FileType{{MIME: "application/x-n", Description: "N"}}}, "Ext is empty"},
		{"no mime", App{FileTypes: []FileType{{Ext: ".note", Description: "N"}}}, "needs a MIMEType"},
		{"no description", App{FileTypes: []FileType{{Ext: ".note", MIME: "application/x-n"}}}, "needs a Description"},
		{"bad role", App{FileTypes: []FileType{{Ext: ".note", MIME: "application/x-n", Description: "N", Role: 9}}}, "invalid HandlerRole"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			defer func() {
				msg := fmt.Sprint(recover())
				if !strings.Contains(msg, tc.want) {
					t.Errorf("panic %q, want it to contain %q", msg, tc.want)
				}
			}()
			tc.app.checkDeclaration()
			t.Error("no panic")
		})
	}
}

// Run checks the declaration in dev mode, before it opens anything.
func TestRunChecksDeclarationInDevMode(t *testing.T) {
	on := true
	defer func() {
		if msg := fmt.Sprint(recover()); !strings.Contains(msg, "has a ':'") {
			t.Errorf("panic %q", msg)
		}
	}()
	_ = Run(App{Root: C(func() Node { return nil }), Dev: &on, Schemes: []string{"notes:"}})
	t.Error("no panic")
}
