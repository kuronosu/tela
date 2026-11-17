package main

import (
	"reflect"
	"strings"
	"testing"

	"github.com/kuronosu/tela"
)

func TestReadAppDeclaration(t *testing.T) {
	const head = "package main\n\nimport \"github.com/kuronosu/tela\"\n\n"
	for _, tc := range []struct {
		name  string
		src   string
		want  appSource // the declaration fields
		wantE map[string]string
	}{
		{
			name: "everything",
			src: head + `const company = "Example"

var schemes = []string{"notes", "notes" + "-dev"}

var App = tela.App{
	AppID: "dev.example.notes", Title: "Notes",
	Publisher: company + " Inc.", Description: "Take notes", Category: "Office",
	Schemes: schemes,
	FileTypes: []tela.FileType{
		{Ext: ".note", MIME: "application/x-notes-note", Description: "Notes document"},
		{Ext: ".bak", MIME: "application/x-notes-bak", Description: "Backup", Role: tela.HandlerViewer},
		tela.FileType{".idx", "application/x-notes-idx", "Index", tela.HandlerNone},
	},
	Services: tela.Services{Camera: "Scan a page", Location: "Tag notes" + " by place", Network: true, Files: true, Secrets: false},
}

func main() { tela.Main(App) }
`,
			want: appSource{
				Publisher: "Example Inc.", Description: "Take notes", Category: "Office",
				Schemes: []string{"notes", "notes-dev"},
				FileTypes: []tela.FileType{
					{Ext: ".note", MIME: "application/x-notes-note", Description: "Notes document"},
					{Ext: ".bak", MIME: "application/x-notes-bak", Description: "Backup", Role: tela.HandlerViewer},
					{Ext: ".idx", MIME: "application/x-notes-idx", Description: "Index", Role: tela.HandlerNone},
				},
				Services: tela.Services{Camera: "Scan a page", Location: "Tag notes by place", Network: true, Files: true},
			},
		},
		{
			name: "dot import, a variable of the services and a local list",
			src: `package main

import . "github.com/kuronosu/tela"

var svc = Services{Microphone: "Record a memo", Notifications: true, GlobalShortcuts: true, Bluetooth: "Pair", Contacts: "c", Calendars: "d", Photos: "p"}

func main() {
	types := []FileType{{Ext: ".x", MIME: "application/x-x", Description: "X", Role: HandlerShell}}
	Main(App{AppID: "dev.example.x", Services: svc, FileTypes: types})
}
`,
			want: appSource{
				FileTypes: []tela.FileType{{Ext: ".x", MIME: "application/x-x", Description: "X", Role: tela.HandlerShell}},
				Services:  tela.Services{Microphone: "Record a memo", Notifications: true, GlobalShortcuts: true, Bluetooth: "Pair", Contacts: "c", Calendars: "d", Photos: "p"},
			},
		},
		{
			name: "nothing declared",
			src:  head + "func main() { tela.Main(tela.App{AppID: \"dev.example.x\"}) }\n",
			want: appSource{},
		},
		{
			name: "not constant",
			src: head + `var on = true

var App = tela.App{
	AppID: "dev.example.x",
	Publisher: os.Getenv("P"),
	Description: "ok",
	Schemes: append([]string{"a"}, "b"),
	FileTypes: []tela.FileType{{Ext: ".a", MIME: mime(), Description: "A"}},
	Services: tela.Services{Camera: "ok", Network: on},
}

func main() { tela.Main(App) }
`,
			want: appSource{Description: "ok"},
			wantE: map[string]string{
				"Publisher": "Publisher of the tela.App is not a constant string",
				"Schemes":   "Schemes of the tela.App is not a slice literal of constant strings that tela can read without running the program; write it as a slice literal",
				"FileTypes": "FileTypes of the tela.App has an entry (0) whose MIME is not a constant string",
				"Services":  "Services of the tela.App has a Network that is not true or false written in the source",
			},
		},
		{
			name: "an entry that is not constant, a role that is not a name, an unknown field",
			src: head + `var s = "x"
var App = tela.App{
	Schemes: []string{"a", s},
	FileTypes: []tela.FileType{{Ext: ".a", MIME: "a/b", Description: "A", Role: role}},
	Services: tela.Services{Fax: "no"},
}

func main() { tela.Main(App) }
`,
			want: appSource{},
			wantE: map[string]string{
				"Schemes":   "has an entry (1) that is not a constant string",
				"FileTypes": "whose Role is not one of tela.HandlerEditor",
				"Services":  "sets Fax, which tela.Services does not have",
			},
		},
		{
			name: "an entry that is not a tela.FileType",
			src: head + `var App = tela.App{FileTypes: []tela.FileType{mk()}}

func main() { tela.Main(App) }
`,
			wantE: map[string]string{"FileTypes": "has an entry (0) that is not a tela.FileType literal"},
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			target, deps := appPackage(t, map[string]string{"main.go": tc.src}, nil)
			got, err := readAppSource(target, deps)
			if err != nil {
				t.Fatal(err)
			}
			if !got.Found {
				t.Fatal("no App found")
			}
			if got.Publisher != tc.want.Publisher || got.Description != tc.want.Description || got.Category != tc.want.Category {
				t.Errorf("got Publisher=%q Description=%q Category=%q, want %q %q %q", got.Publisher, got.Description, got.Category, tc.want.Publisher, tc.want.Description, tc.want.Category)
			}
			if !reflect.DeepEqual(got.Schemes, tc.want.Schemes) {
				t.Errorf("Schemes %q, want %q", got.Schemes, tc.want.Schemes)
			}
			if !reflect.DeepEqual(got.FileTypes, tc.want.FileTypes) {
				t.Errorf("FileTypes %+v, want %+v", got.FileTypes, tc.want.FileTypes)
			}
			if got.Services != tc.want.Services {
				t.Errorf("Services %+v, want %+v", got.Services, tc.want.Services)
			}
			if len(got.Errs) != len(tc.wantE) {
				t.Errorf("errors %v, want %d of them (%v)", got.Errs, len(tc.wantE), tc.wantE)
			}
			for field, want := range tc.wantE {
				if err := got.Errs[field]; err == nil || !strings.Contains(err.Error(), want) {
					t.Errorf("%s: error %v, want it to contain %q", field, err, want)
				}
			}
			if len(tc.wantE) > 0 && got.DeclarationErr() == nil {
				t.Error("DeclarationErr is nil")
			}
			if len(tc.wantE) == 0 && got.DeclarationErr() != nil {
				t.Errorf("DeclarationErr: %v", got.DeclarationErr())
			}
		})
	}
}
