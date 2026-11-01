package main

import (
	"strings"

	"github.com/kuronosu/tela"
	"github.com/kuronosu/tela/examples/notes/icons"
	. "github.com/kuronosu/tela/h"
	"github.com/kuronosu/tela/kit"
)

// Editor is the third pane: a thin header with the folder, the tags and the
// state of the note, the title, the text, and a status line. Its fields are
// controlled: they show the note and report every edit. It is keyed by the
// note (see screen), so each note gets fields of its own, with their own
// caret and undo history.
type Editor struct {
	Note    Note
	Prefs   Prefs
	Folders []string
	// OnTitle and OnText report an edit of the title and of the text.
	OnTitle, OnText func(string)
	// OnFolder moves the note to a folder ("" for none); OnPin pins and
	// unpins it.
	OnFolder func(string)
	OnPin    func()
	// OnDelete asks to delete the note.
	OnDelete func()
	// OnLock is the lock button: it locks the note, or locks it again
	// while it is unlocked, or unlocks it while it is locked (the Unlock
	// button of the placeholder is the same). LockHint, when it is not
	// empty, is why locking is not possible and the controls are disabled
	// (no keyring); Busy is true while the keyring is being asked.
	OnLock   func()
	LockHint string
	Busy     bool
	// KeyGone is true when the keyring has no key for the locked notes.
	KeyGone bool
}

func (e Editor) Build() tela.Node {
	n := e.Note
	body := tela.UseElementRef()
	// A new note has the cursor in its title; the others, in their text.
	titleFirst := tela.UseOnce(func() bool { return n.Title == "" && n.Text == "" })

	wrap := ""
	if !e.Prefs.WordWrap {
		wrap = "text-nowrap overflow-x-auto"
	}
	where := "In your library"
	switch {
	case n.Hidden():
		where = "Encrypted in your library"
	case n.Path != "":
		where = n.Path
	}
	var content tela.Child
	if n.Hidden() {
		content = lockedPane(e.LockHint, e.Busy, e.KeyGone, e.OnLock)
	} else {
		content = Div(Class("flex min-h-0 flex-1 flex-col px-8 pt-7"),
			Input(AriaLabel("Title"), Placeholder("Untitled"), Bind(n.Title, e.OnTitle), AutoFocus(titleFirst),
				OnKeyDown(func(k tela.KeyboardEvent) {
					if k.Key == "Enter" {
						k.PreventDefault()
						body.Focus()
					}
				}),
				Class("w-full bg-transparent text-[28px] leading-9 font-semibold tracking-tight caret-gold outline-none placeholder:text-muted-foreground/50")),
			Textarea(Ref(body), AriaLabel("Note text"), Placeholder("Start typing…"), AutoFocus(!titleFirst), Bind(n.Text, e.OnText),
				Class("mt-4 min-h-0 w-full flex-1 resize-none bg-transparent pb-6 text-foreground/90 caret-gold outline-none",
					"selection:bg-gold/30 placeholder:text-muted-foreground/50",
					fontClass(e.Prefs.FontSize), spacingClass(e.Prefs.LineSpacing), wrap)),
		)
	}
	return Div(Class("flex min-w-0 flex-1 flex-col"),
		Div(Class("flex h-11 shrink-0 items-center gap-2 border-b border-border px-4 text-xs text-muted-foreground"),
			FolderMenu{Folder: n.Folder, Folders: e.Folders, OnFolder: e.OnFolder},
			Map(n.Tags(), func(t string) tela.Node {
				return Span(Key(t), Class("rounded-md bg-gold/10 px-1.5 py-0.5 font-medium text-gold"), Text("#"+t))
			}),
			Span(Class("ms-auto flex items-center gap-2"),
				If(n.Dirty(), Span(Class("flex items-center gap-1.5"),
					Span(Role("img"), AriaLabel("Unsaved changes"), Class("size-1.5 rounded-full bg-gold")), Text("Unsaved"))),
				lockButton(n, e.LockHint, e.Busy, e.OnLock),
				pinButton(n.Pinned, e.OnPin),
				kit.Tooltip{
					Trigger: kit.Button{Variant: kit.Ghost, Size: kit.IconSm, Icon: icons.Trash, AriaLabel: "Delete note",
						OnClick: e.OnDelete, Class: "size-7 text-muted-foreground"},
					Content: Text("Delete note"), Placement: "bottom",
				},
			),
		),
		content,
		Div(Class("flex h-9 shrink-0 items-center justify-between gap-4 border-t border-border px-6 text-[11px] text-muted-foreground"),
			Span(Class("min-w-0 truncate"), Text(where)),
			If(!n.Hidden(), Span(Class("shrink-0 tabular-nums"), Textf("%d words · %d characters", n.Words(), n.Characters()))),
		),
	)
}

// lockButton is the icon button that locks the note: a lock that is open
// for a note that is not locked, and in the accent color for a locked one
// (named "Lock now" while it is unlocked, "Unlock note" while it is hidden).
// A hint takes the button out of use and says why in its tooltip.
func lockButton(n Note, hint string, busy bool, onClick func()) tela.Node {
	label, icon, class := "Lock note", icons.LockOpen, "text-muted-foreground"
	switch {
	case n.Hidden():
		label, icon, class = "Unlock note", icons.Lock, "text-gold"
	case n.Locked:
		label, icon, class = "Lock now", icons.Lock, "text-gold"
	}
	button := kit.Button{Variant: kit.Ghost, Size: kit.IconSm, Icon: icon, AriaLabel: label,
		Disabled: busy, OnClick: onClick, Class: "size-7 " + class}
	if hint != "" {
		// Disabled for assistive technologies but not inert, so that the
		// tooltip that says why still opens.
		button.Disabled, button.OnClick, button.Props = false, nil, AriaDisabled(true)
		button.Class += " opacity-50"
		label = hint
	}
	return kit.Tooltip{Trigger: button, Content: Text(label), Placement: "bottom"}
}

// lockedPane takes the place of the fields of a locked note that is not
// unlocked. hint is why the note cannot be unlocked (no keyring), and
// keyGone says the keyring lost the key.
func lockedPane(hint string, busy, keyGone bool, onUnlock func()) tela.Node {
	desc := "Its title and text are encrypted in your library. Unlock it to read and edit; it locks again when you hide the window."
	button := "Unlock"
	switch {
	case hint != "":
		desc = hint
	case keyGone:
		desc = "The key for your locked notes is no longer in the system keyring, so this note cannot be opened. Its encrypted text is still in your library."
		button = "Try again"
	}
	if busy {
		button = "Unlocking…"
	}
	return Div(Class("flex min-h-0 flex-1 flex-col items-center justify-center gap-5 px-10 text-center"),
		Span(Class("flex size-12 items-center justify-center rounded-full bg-gold/10 text-gold"),
			Icon(icons.Lock, Class("size-5"), AriaHidden(true))),
		Div(Class("flex flex-col items-center gap-1.5"),
			H2(Class("text-lg font-semibold tracking-tight"), Text("This note is locked")),
			P(Class("max-w-sm text-[13px] leading-5 text-muted-foreground"), Text(desc)),
		),
		kit.Button{Icon: icons.LockOpen, Disabled: hint != "" || busy, OnClick: onUnlock, Children: Text(button)},
	)
}

// pinButton is the icon button that pins and unpins the note, pressed while
// it is pinned.
func pinButton(pinned bool, onClick func()) tela.Node {
	class := "text-muted-foreground"
	label := "Pin note"
	if pinned {
		class, label = "text-gold", "Unpin note"
	}
	return kit.Tooltip{
		Trigger: kit.Button{Variant: kit.Ghost, Size: kit.IconSm, Icon: icons.Pin, AriaLabel: "Pin note",
			OnClick: onClick, Class: "size-7 " + class, Props: AriaPressed(pinned)},
		Content: Text(label), Placement: "bottom",
	}
}

// folderKey names an entry of the folder menu: a folder (Name, "" for none),
// the radio group of the folders, or "New folder…".
type folderKey struct {
	Kind int
	Name string
}

const (
	keyFolder = iota
	keyFolderGroup
	keyNewFolder
)

// FolderMenu is the folder of the note in the editor's header, a button that
// opens a menu with the existing folders, "No folder" and "New folder…",
// which asks for a name in a dialog.
type FolderMenu struct {
	Folder   string
	Folders  []string
	OnFolder func(string)
}

func (m FolderMenu) Build() tela.Node {
	creating, setCreating := tela.UseState(false)
	chosen := folderKey{Kind: keyFolder, Name: m.Folder}
	radios := []kit.MenuItem[folderKey]{{Key: folderKey{Kind: keyFolder}, Label: "No folder"}}
	for _, f := range m.Folders {
		radios = append(radios, kit.MenuItem[folderKey]{Key: folderKey{Kind: keyFolder, Name: f}, Label: f})
	}
	label := m.Folder
	if label == "" {
		label = "No folder"
	}
	return Fragment(
		kit.DropdownMenu[folderKey]{
			Trigger: kit.Button{Variant: kit.Ghost, Size: kit.Sm, Icon: icons.Folder, IconEnd: icons.ChevronDown,
				AriaLabel: "Folder: " + label, Class: "h-7 gap-1.5 px-2 text-xs font-normal text-muted-foreground",
				Children: Text(label)},
			Align: "start",
			Items: []kit.MenuItem[folderKey]{
				{Kind: kit.MenuRadioGroup, Key: folderKey{Kind: keyFolderGroup}, Label: "Folder", Items: radios,
					Value: &chosen, OnValueChange: func(k folderKey) { m.OnFolder(k.Name) }},
				{Kind: kit.MenuSeparator},
				{Key: folderKey{Kind: keyNewFolder}, Label: "New folder…", Icon: icons.FolderPlus,
					OnSelect: func() { setCreating.Set(true) }},
			},
		},
		kit.Dialog{
			Open: &creating, OnOpenChange: setCreating.Set,
			Title:       Text("New folder"),
			Description: Text("The note moves into it."),
			Class:       "sm:max-w-sm",
			Children: newFolderForm{
				OnCreate: func(name string) {
					setCreating.Set(false)
					m.OnFolder(name)
				},
				OnCancel: func() { setCreating.Set(false) },
			},
		},
	)
}

// newFolderForm asks for the name of a folder. It exists only while the
// dialog is open, so the name starts empty each time.
type newFolderForm struct {
	OnCreate func(name string)
	OnCancel func()
}

func (f newFolderForm) Build() tela.Node {
	name, setName := tela.UseState("")
	valid := strings.TrimSpace(name) != ""
	return Form(Class("flex flex-col gap-4"),
		OnSubmit(func() {
			if valid {
				f.OnCreate(name)
			}
		}),
		kit.Input{AriaLabel: "Folder name", Placeholder: "Folder name", AutoFocus: true, Value: &name, OnValueChange: setName.Set},
		Div(Class("flex justify-end gap-2"),
			kit.Button{Variant: kit.Outline, Type: "button", OnClick: f.OnCancel, Children: Text("Cancel")},
			kit.Button{Type: "submit", Disabled: !valid, Children: Text("Create")},
		),
	)
}
