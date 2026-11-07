//go:build darwin

package cocoa

import (
	"fmt"
	"image/color"
	"strings"
	"testing"
	"time"

	"github.com/kuronosu/tela/internal/darwin"
	"github.com/kuronosu/tela/internal/platform"
	"github.com/kuronosu/tela/internal/platform/internal/keyboard"
)

// The native menu and quit request tests. Menus are driven
// programmatically (performActionForItemAtIndex:, the main menu's key
// equivalents through synthetic key events, cancelTracking for popups)
// and terminate: is sent to NSApp, so they need no user. Every run has a
// deadline and fails instead of hanging.

// mtsel holds the selectors only these tests send.
var mtsel = struct {
	numberOfItems, itemAtIndex, title, action, target, tag, keyEquivalent darwin.SEL
	modifierMask, state, submenu, isSeparator, servicesMenu, windowsMenu  darwin.SEL
	helpMenu, sendAction, update, isEnabled, performAtIndex, terminate    darwin.SEL
}{
	numberOfItems:  darwin.Sel("numberOfItems"),
	itemAtIndex:    darwin.Sel("itemAtIndex:"),
	title:          darwin.Sel("title"),
	action:         darwin.Sel("action"),
	target:         darwin.Sel("target"),
	tag:            darwin.Sel("tag"),
	keyEquivalent:  darwin.Sel("keyEquivalent"),
	modifierMask:   darwin.Sel("keyEquivalentModifierMask"),
	state:          darwin.Sel("state"),
	submenu:        darwin.Sel("submenu"),
	isSeparator:    darwin.Sel("isSeparatorItem"),
	servicesMenu:   darwin.Sel("servicesMenu"),
	windowsMenu:    darwin.Sel("windowsMenu"),
	helpMenu:       darwin.Sel("helpMenu"),
	sendAction:     darwin.Sel("sendAction:to:from:"),
	update:         darwin.Sel("update"),
	isEnabled:      darwin.Sel("isEnabled"),
	performAtIndex: darwin.Sel("performActionForItemAtIndex:"),
	terminate:      darwin.Sel("terminate:"),
}

// entryLine writes an entry as menuLines writes an NSMenu item.
func entryLine(e menuEntry) string {
	s := e.Title
	if e.Action != "" {
		s += " [" + e.Action
		if e.App {
			s += fmt.Sprintf(" #%d", e.Tag)
		}
		s += "]"
	}
	if e.Key != "" {
		s += fmt.Sprintf(" %s%q", modsText(e.Mods), e.Key)
	}
	if e.Checkable && e.Checked {
		s += " checked"
	}
	return s
}

// planOf writes a plan's lines, indented by depth.
func planOf(p []menuEntry, depth int, out *[]string) {
	for _, e := range p {
		ind := strings.Repeat("  ", depth)
		if e.Separator {
			*out = append(*out, ind+"---")
			continue
		}
		*out = append(*out, ind+entryLine(e))
		planOf(e.Submenu, depth+1, out)
	}
}

// menuLines writes an NSMenu's items, indented by depth; app items are
// the ones whose target is the backend's.
func menuLines(b *Backend, menu darwin.ID, depth int, out *[]string) {
	n := darwin.SendInt(menu, mtsel.numberOfItems)
	for i := 0; i < n; i++ {
		it := darwin.Send(menu, mtsel.itemAtIndex, i)
		ind := strings.Repeat("  ", depth)
		if darwin.SendBool(it, mtsel.isSeparator) {
			*out = append(*out, ind+"---")
			continue
		}
		e := menuEntry{Title: darwin.GoString(darwin.Send(it, mtsel.title))}
		// AppKit gives an item with a submenu the action submenuAction:.
		if a := darwin.SEL(darwin.Send(it, mtsel.action)); a != 0 && darwin.Send(it, mtsel.submenu) == 0 {
			e.Action = darwin.SelName(a)
		}
		if darwin.Send(it, mtsel.target) == b.menus.target && b.menus.target != 0 {
			e.App, e.Tag = true, platform.MenuItemID(darwin.SendInt(it, mtsel.tag))
		}
		e.Key = darwin.GoString(darwin.Send(it, mtsel.keyEquivalent))
		if e.Key != "" {
			e.Mods = darwin.SendUint(it, mtsel.modifierMask)
		}
		e.Checkable = darwin.SendInt(it, mtsel.state) == darwin.NSControlStateValueOn
		e.Checked = e.Checkable
		*out = append(*out, ind+entryLine(e))
		if sub := darwin.Send(it, mtsel.submenu); sub != 0 {
			menuLines(b, sub, depth+1, out)
		}
	}
}

// checkBar compares NSApp's main menu with plan: every planned line, in
// order (AppKit adds items of its own: the window list, Emoji & Symbols),
// and NSApp's Services, Window and Help menus.
func checkBar(t *testing.T, b *Backend, what string, plan []menuEntry) {
	t.Helper()
	pool := darwin.PoolPush()
	defer darwin.PoolPop(pool)
	main := darwin.Send(app.ns, msel.mainMenu)
	var got, want []string
	menuLines(b, main, 0, &got)
	planOf(plan, 0, &want)
	t.Logf("%s: the main menu is\n  %s", what, strings.Join(got, "\n  "))
	i := 0
	for _, g := range got {
		if i < len(want) && g == want[i] {
			i++
		}
	}
	if i < len(want) {
		t.Errorf("%s: the main menu lacks %q (in order); want\n  %s", what, want[i], strings.Join(want, "\n  "))
	}
	// The special submenus.
	sub := func(path ...int) darwin.ID {
		m := main
		for _, p := range path {
			if m == 0 || p >= darwin.SendInt(m, mtsel.numberOfItems) {
				return 0
			}
			m = darwin.Send(darwin.Send(m, mtsel.itemAtIndex, p), mtsel.submenu)
		}
		return m
	}
	wantWindows, wantHelp, wantServices := darwin.ID(0), darwin.ID(0), darwin.ID(0)
	for i, top := range plan {
		switch top.Special {
		case specialWindows:
			wantWindows = sub(i)
		case specialHelp:
			wantHelp = sub(i)
		case specialApp:
			for j, e := range top.Submenu {
				if e.Special == specialServices {
					wantServices = sub(i, j)
				}
			}
		}
	}
	// AppKit keeps one Services menu: the Services item must open it.
	if s := darwin.Send(app.ns, mtsel.servicesMenu); s != wantServices || s == 0 {
		t.Errorf("%s: the Services item's submenu is %#x, NSApp.servicesMenu %#x; want the same menu", what, wantServices, s)
	}
	if w := darwin.Send(app.ns, mtsel.windowsMenu); w != wantWindows {
		t.Errorf("%s: NSApp.windowsMenu %#x, want %#x", what, w, wantWindows)
	}
	if h := darwin.Send(app.ns, mtsel.helpMenu); h != wantHelp {
		t.Errorf("%s: NSApp.helpMenu %#x, want %#x", what, h, wantHelp)
	}
}

// indexWhere returns the index of the first item of menu that match
// accepts, or -1.
func indexWhere(menu darwin.ID, match func(it darwin.ID) bool) int {
	n := darwin.SendInt(menu, mtsel.numberOfItems)
	for i := 0; i < n; i++ {
		if match(darwin.Send(menu, mtsel.itemAtIndex, i)) {
			return i
		}
	}
	return -1
}

func titled(title string) func(darwin.ID) bool {
	return func(it darwin.ID) bool { return darwin.GoString(darwin.Send(it, mtsel.title)) == title }
}

func sending(action string) func(darwin.ID) bool {
	return func(it darwin.ID) bool {
		a := darwin.SEL(darwin.Send(it, mtsel.action))
		return a != 0 && darwin.SelName(a) == action
	}
}

var testAppMenu = []platform.MenuItem{
	{Label: "File", Submenu: []platform.MenuItem{
		{ID: 1, Label: "New", Accelerator: "CmdOrCtrl+N"},
		{ID: 2, Label: "Autosave", Checkable: true, Checked: true},
		{ID: 3, Label: "Print", Disabled: true},
		{Separator: true},
		{ID: 4, Role: platform.RoleClose},
	}},
	{Label: "Edit", Role: platform.RoleEditMenu, Submenu: []platform.MenuItem{
		{ID: 10, Role: platform.RoleUndo},
		{ID: 11, Role: platform.RoleRedo},
		{Separator: true},
		{ID: 12, Role: platform.RoleCut},
		{ID: 13, Role: platform.RoleCopy},
		{ID: 14, Role: platform.RolePaste},
		{ID: 15, Role: platform.RoleSelectAll},
	}},
	{Label: "Tools", Submenu: []platform.MenuItem{{ID: 20, Label: "Run", Accelerator: "Cmd+K"}}},
	{Label: "Window", Role: platform.RoleWindowMenu, Submenu: []platform.MenuItem{{ID: 30, Role: platform.RoleMinimize}, {ID: 31, Role: platform.RoleZoom}}},
	{Label: "Help", Role: platform.RoleHelpMenu, Submenu: []platform.MenuItem{{ID: 40, Label: "Tela Help"}}},
}

var testAppItems = []platform.MenuItem{{ID: 50, Label: "Settings…", Accelerator: "CmdOrCtrl+,"}}

// TestAppMenuBar checks the default bar, the app's bar, its items'
// AppMenuEvents and SetAppMenu(nil, nil).
func TestAppMenuBar(t *testing.T) {
	b := newBackend(t)
	name := ""
	onMain(func() {
		pool := darwin.PoolPush()
		name = appName()
		checkBar(t, b, "the default bar at New", defaultMenuBarPlan(name))
		darwin.PoolPop(pool)
	})
	t.Logf("app name %q", name)
	var chosen []platform.MenuItemID
	step := 0
	desktopRun(t, b, 30*time.Second, func(w *Window) {
		d := b.Desktop()
		mustPanic(t, "SetAppMenu with a menu role among the app items", func() {
			_ = d.SetAppMenu(nil, []platform.MenuItem{{Label: "Edit", Role: platform.RoleEditMenu, Submenu: []platform.MenuItem{{ID: 1, Label: "x"}}}})
		})
		if err := d.SetAppMenu(testAppMenu, testAppItems); err != nil {
			t.Errorf("SetAppMenu: %v", err)
			step = 99
			return
		}
		checkBar(t, b, "the app's bar", appMenuBarPlan(name, testAppMenu, testAppItems))
		pool := darwin.PoolPush()
		defer darwin.PoolPop(pool)
		main := darwin.Send(app.ns, msel.mainMenu)
		file := darwin.Send(darwin.Send(main, mtsel.itemAtIndex, 1), mtsel.submenu)
		appMenu := darwin.Send(darwin.Send(main, mtsel.itemAtIndex, 0), mtsel.submenu)
		darwin.Send(file, mtsel.update) // AppKit's automatic enabling
		printItem := darwin.Send(file, mtsel.itemAtIndex, 2)
		t.Logf("File: New enabled %v, Print enabled %v",
			darwin.SendBool(darwin.Send(file, mtsel.itemAtIndex, 0), mtsel.isEnabled), darwin.SendBool(printItem, mtsel.isEnabled))
		if darwin.SendBool(printItem, mtsel.isEnabled) {
			t.Errorf("the disabled item Print is enabled")
		}
		darwin.Send(file, mtsel.performAtIndex, 0) // New: AppMenuEvent 1
		darwin.Send(file, mtsel.performAtIndex, 2) // Print, disabled: nothing
		settings := indexWhere(appMenu, titled("Settings…"))
		if settings < 0 {
			t.Errorf("the app menu has no Settings… item")
			step = 99
			return
		}
		darwin.Send(appMenu, mtsel.performAtIndex, settings) // AppMenuEvent 50
	}, func(w *Window, e platform.Event) bool {
		if me, ok := e.(platform.AppMenuEvent); ok {
			if eventWindow != nil {
				t.Errorf("AppMenuEvent %d delivered with a window", me.Item)
			}
			chosen = append(chosen, me.Item)
		}
		if _, ok := e.(platform.WakeEvent); !ok || step != 0 {
			return step != 0
		}
		if len(chosen) < 2 {
			return false
		}
		if fmt.Sprint(chosen) != "[1 50]" {
			t.Errorf("AppMenuEvents %v, want [1 50]", chosen)
		}
		if err := b.Desktop().SetAppMenu(nil, nil); err != nil {
			t.Errorf("SetAppMenu(nil, nil): %v", err)
		}
		checkBar(t, b, "after SetAppMenu(nil, nil)", defaultMenuBarPlan(name))
		step = 1
		return true
	})
	if len(chosen) != 2 {
		t.Errorf("AppMenuEvents %v, want [1 50]", chosen)
	}
	onMain(func() {
		if err := b.Desktop().SetAppMenu(testAppMenu, nil); err == nil {
			t.Errorf("SetAppMenu after Run = nil, want an error")
		}
		checkBar(t, b, "after Run", defaultMenuBarPlan(name))
	})
}

// menuSession runs steps like runSteps, recording every event but the
// frames (app-scoped ones too) in events.
type menuSession struct {
	*session
	events []string
}

func eventLine(w platform.Window, e platform.Event) string {
	switch e := e.(type) {
	case platform.KeyEvent:
		return "key " + keyString(e)
	case platform.AppMenuEvent:
		return fmt.Sprintf("menu %d (window %v)", e.Item, w != nil)
	case platform.QuitRequestedEvent:
		return "quit"
	}
	return ""
}

func runMenuSteps(t *testing.T, title string, steps ...func(s *menuSession)) *menuSession {
	b := newBackend(t)
	s := &menuSession{session: &session{t: t, b: b}}
	pause := 300 * time.Millisecond
	onMain(func() {
		pw, err := b.NewWindow(platform.WindowOptions{Title: title, Size: platform.Size{W: 200, H: 150}})
		if err != nil {
			t.Errorf("NewWindow: %v", err)
			return
		}
		s.w = pw.(*Window)
		time.AfterFunc(20*time.Second+time.Duration(len(steps))*pause, b.Quit) // a failure, not a hang
		stage, tries, scheduled := 0, 0, false
		_ = b.Run(platform.HandlerFunc(func(ew platform.Window, e platform.Event) {
			switch e.(type) {
			case platform.FrameEvent:
				presentSolid(t, ew, color.RGBA{0x80, 0x80, 0x90, 0xff})
				if !scheduled && s.w.shown {
					scheduled = true
					time.AfterFunc(150*time.Millisecond, b.Wake)
				}
				return
			case platform.WakeEvent:
				if stage == 0 && !s.w.focused && tries < 30 {
					tries++
					if tries%5 == 1 {
						s.askFocus()
					}
					time.AfterFunc(100*time.Millisecond, b.Wake)
					return
				}
				if stage == 0 {
					s.key = darwin.SendBool(s.w.ns, tsel.isKeyWindow) && darwin.Send(app.ns, isel.keyWindow) == s.w.ns
					if !s.w.focused {
						s.simulated = true
						darwin.Send(s.w.delegate, darwin.Sel("windowDidBecomeKey:"), darwin.ID(0))
					}
					t.Logf("key window %v, focus simulated %v", s.key, s.simulated)
					s.ts = uptime()
				}
				if stage < len(steps) {
					pool := darwin.PoolPush()
					s.events = append(s.events, fmt.Sprintf("-- step %d", stage))
					steps[stage](s)
					darwin.PoolPop(pool)
					stage++
					time.AfterFunc(pause, b.Wake)
					return
				}
				b.Quit()
				return
			}
			if stage >= 1 {
				if l := eventLine(ew, e); l != "" {
					s.events = append(s.events, l)
				}
			}
		}))
	})
	if s.w == nil {
		return nil
	}
	t.Logf("events:\n  %s", strings.Join(s.events, "\n  "))
	return s
}

// stepEvents returns the events recorded after step i's marker, up to the
// next marker.
func (s *menuSession) stepEvents(i int) []string {
	var out []string
	in := false
	for _, e := range s.events {
		if strings.HasPrefix(e, "-- step ") {
			in = e == fmt.Sprintf("-- step %d", i)
			continue
		}
		if in {
			out = append(out, e)
		}
	}
	return out
}

const (
	kvkC, kvkK, kvkJ = 0x08, 0x28, 0x26
	cmdFlag          = darwin.NSEventModifierFlagCommand
)

// tapCmd sends a Command key press and release to the window, and logs
// which matching chose a menu item for it (AppKit's or the backend's).
func (s *menuSession) tapCmd(code uint16, chars string) {
	before := menuKeyStats
	s.sendKey(s.keyEvent(true, code, cmdFlag, chars))
	s.sendKey(s.keyEvent(false, code, cmdFlag, chars))
	s.t.Logf("Cmd+%s: chosen by AppKit's matching %d, by the backend's %d",
		chars, menuKeyStats.appKit-before.appKit, menuKeyStats.own-before.own)
}

// TestAppMenuKeys: a key equivalent of the app's bar chooses its item and
// is not delivered; a key no item has is; the default bar's Cmd+C reaches
// the window once, through the copy: role; the role's action from the
// responder chain and from a menu click give the standard key press.
func TestAppMenuKeys(t *testing.T) {
	var edit darwin.ID
	s := runMenuSteps(t, "tela menu keys",
		func(s *menuSession) { // 0: the default bar's Cmd+C
			s.tapCmd(kvkC, "c")
		},
		func(s *menuSession) { // 1: copy: through the responder chain
			if s.key {
				darwin.Send(app.ns, mtsel.sendAction, darwin.Sel("copy:"), darwin.ID(0), darwin.ID(0))
			} else {
				darwin.Send(s.w.view, darwin.Sel("copy:"), darwin.ID(0))
			}
		},
		func(s *menuSession) { // 2: a click on the default Edit menu's Copy
			main := darwin.Send(app.ns, msel.mainMenu)
			edit = darwin.Send(darwin.Send(main, mtsel.itemAtIndex, 1), mtsel.submenu)
			darwin.Send(edit, mtsel.update)
			i := indexWhere(edit, sending("copy:"))
			if i < 0 {
				s.t.Errorf("the default Edit menu has no copy: item")
				return
			}
			copyItem := darwin.Send(edit, mtsel.itemAtIndex, i)
			s.t.Logf("Edit > %s enabled %v", darwin.GoString(darwin.Send(copyItem, mtsel.title)), darwin.SendBool(copyItem, mtsel.isEnabled))
			if s.key && !darwin.SendBool(copyItem, mtsel.isEnabled) {
				s.t.Errorf("Copy is disabled while the TelaView is key")
			}
			darwin.Send(edit, mtsel.performAtIndex, i)
		},
		func(s *menuSession) { // 3: the app's bar: Cmd+K is Tools > Run, Cmd+J nothing
			if err := s.b.Desktop().SetAppMenu(testAppMenu, testAppItems); err != nil {
				s.t.Errorf("SetAppMenu: %v", err)
			}
			main := darwin.Send(app.ns, msel.mainMenu)
			tools := darwin.Send(darwin.Send(main, mtsel.itemAtIndex, indexWhere(main, titled("Tools"))), mtsel.submenu)
			darwin.Send(tools, mtsel.update)
			run := darwin.Send(tools, mtsel.itemAtIndex, 0)
			s.t.Logf("Tools > %s: key equivalent %q mask %#x, enabled %v, target is the backend's %v",
				darwin.GoString(darwin.Send(run, mtsel.title)), darwin.GoString(darwin.Send(run, mtsel.keyEquivalent)),
				darwin.SendUint(run, mtsel.modifierMask), darwin.SendBool(run, mtsel.isEnabled),
				darwin.Send(run, mtsel.target) == s.b.menus.target)
			s.tapCmd(kvkK, "k")
			s.tapCmd(kvkJ, "j")
		},
	)
	if s == nil {
		return
	}
	keyLines := func(k keyboard.MacKey) (string, string) {
		down, _ := keyboard.MacKeyDown(k)
		up, _ := keyboard.MacKeyUp(k)
		return "key " + keyString(down), "key " + keyString(up)
	}
	press, release := keyLines(editKeys[platform.RoleCopy])
	pressJ, releaseJ := keyLines(keyboard.MacKey{KeyCode: kvkJ, Flags: cmdFlag, Characters: "j", CharactersIgnoringModifiers: "j"})
	check := func(i int, want ...string) {
		got := s.stepEvents(i)
		var keys []string
		for _, g := range got {
			if strings.HasPrefix(g, "key ") || strings.HasPrefix(g, "menu ") {
				keys = append(keys, g)
			}
		}
		if fmt.Sprint(keys) != fmt.Sprint(want) {
			t.Errorf("step %d: events %q, want %q", i, keys, want)
		}
	}
	if !s.key {
		t.Logf("the window was not AppKit's key window: key equivalents go to the window, not the main menu; checking the actions only")
		check(1, press, release)
		return
	}
	check(0, press, release)
	check(1, press, release)
	check(2, press, release)
	check(3, "menu 20 (window false)", pressJ, releaseJ)
}

// TestPopupMenu chooses an item of a popup menu while its tracking loop
// runs, then dismisses a second one.
func TestPopupMenu(t *testing.T) {
	b := newBackend(t)
	items := []platform.MenuItem{
		{ID: 1, Label: "Open", Accelerator: "CmdOrCtrl+O"},
		{ID: 2, Label: "Copy", Role: platform.RoleCopy},
		{Separator: true},
		{ID: 3, Label: "Off", Disabled: true},
	}
	var answers []string
	var tracked []string
	round := 0
	desktopRun(t, b, 30*time.Second, func(w *Window) {
		mustPanic(t, "PopupMenu with an empty menu", func() { _ = w.PopupMenu(platform.Point{}, nil, func(platform.MenuItemID, error) {}) })
		if err := w.SetMenu([]platform.MenuItem{{ID: 1, Label: "x"}}); err != platform.ErrUnsupported {
			t.Errorf("SetMenu = %v, want ErrUnsupported", err)
		}
		for i := 0; i < 2; i++ {
			if err := w.PopupMenu(platform.Point{X: 20, Y: 30}, items, func(id platform.MenuItemID, err error) {
				answers = append(answers, fmt.Sprintf("%d %v", id, err))
			}); err != nil {
				t.Errorf("PopupMenu: %v", err)
			}
		}
	}, func(w *Window, e platform.Event) bool {
		if _, ok := e.(platform.WakeEvent); !ok {
			return false
		}
		if r := b.menus.popup; r != nil && r.menu != 0 && len(tracked) == round {
			n := darwin.SendInt(r.menu, mtsel.numberOfItems)
			tracked = append(tracked, fmt.Sprintf("round %d: tracking a menu of %d items", round, n))
			if round == 0 {
				darwin.Send(r.menu, mtsel.performAtIndex, 1) // Copy: answered with its ID
			}
			darwin.Send(r.menu, msel.cancelTracking)
			round++
		}
		return len(answers) == 2
	})
	t.Logf("%s; answers %q", strings.Join(tracked, "; "), answers)
	if fmt.Sprint(answers) != "[2 <nil> 0 <nil>]" {
		t.Errorf("answers %q, want [2 <nil> 0 <nil>]", answers)
	}
}

// TestQuitRequest sends terminate: outside a pass (the app keeps running:
// declined), from inside a pass (passed on), and outside a pass with a
// handler that quits (accepted: Run returns, and this test goes on, so
// AppKit called no exit()).
func TestQuitRequest(t *testing.T) {
	b := newBackend(t)
	var quits int
	var answers []string
	phase := 0
	terminate := func() {
		b.onMain(func() {
			darwin.Send(app.ns, mtsel.terminate, darwin.ID(0))
			answers = append(answers, b.menus.quit.last.String())
			b.Wake()
		})
	}
	desktopRun(t, b, 30*time.Second, func(w *Window) {
		terminate()
	}, func(w *Window, e platform.Event) bool {
		if _, ok := e.(platform.QuitRequestedEvent); ok {
			quits++
			if eventWindow != nil {
				t.Errorf("QuitRequestedEvent delivered with a window")
			}
			if phase == 2 {
				return true // quits from the request's handler
			}
			return false
		}
		if _, ok := e.(platform.WakeEvent); !ok {
			return false
		}
		switch {
		case phase == 0 && len(answers) == 1:
			// From inside a pass: delivered in the next one.
			darwin.Send(app.ns, mtsel.terminate, darwin.ID(0))
			answers = append(answers, b.menus.quit.last.String())
			phase = 1
		case phase == 1 && quits == 2:
			phase = 2
			terminate()
		}
		return false
	})
	t.Logf("%d quit requests, answers %q", quits, answers)
	if quits != 3 || fmt.Sprint(answers) != "[declined passed accepted]" {
		t.Errorf("quit requests %d, answers %q; want 3 and [declined passed accepted]", quits, answers)
	}
	if b.menus.quit.requests != 3 {
		t.Errorf("terminate: counted %d requests, want 3", b.menus.quit.requests)
	}
}

// TestQuitRequestDialog: OnQuit asks with an app-modal alert. terminate:
// waits while the alert is open; Cancel declines, OK quits.
func TestQuitRequestDialog(t *testing.T) {
	b := newBackend(t)
	var answers []string
	waitingAtClick := []bool{}
	round := 0
	terminate := func() {
		b.onMain(func() {
			darwin.Send(app.ns, mtsel.terminate, darwin.ID(0))
			answers = append(answers, b.menus.quit.last.String())
			b.Wake()
		})
	}
	clicked := false
	desktopRun(t, b, 40*time.Second, func(w *Window) {
		terminate()
	}, func(w *Window, e platform.Event) bool {
		if _, ok := e.(platform.QuitRequestedEvent); ok {
			clicked = false
			r := round
			if err := b.Desktop().MessageBox(platform.MessageBoxOptions{Title: "Quit?", Text: "tela test: unsaved changes", Buttons: platform.ButtonsOKCancel},
				func(a platform.MessageAnswer, err error) {
					if r == 1 && a == platform.AnswerOK {
						b.Quit()
					}
				}); err != nil {
				t.Errorf("MessageBox: %v", err)
			}
			return false
		}
		if _, ok := e.(platform.WakeEvent); !ok {
			return false
		}
		if !clicked && len(b.desk.dialogs) == 1 && b.desk.dialogs[0].detached && darwin.SendBool(b.desk.dialogs[0].window(), psel.isVisible) {
			clicked = true
			waitingAtClick = append(waitingAtClick, b.menus.quit.waiting)
			if round == 0 {
				clickAlert(1)(b.desk.dialogs[0]) // Cancel
			} else {
				clickAlert(0)(b.desk.dialogs[0]) // OK
			}
		}
		if round == 0 && len(answers) == 1 {
			round = 1
			terminate()
		}
		return false
	})
	t.Logf("answers %q, terminate: waiting when the alert was answered %v", answers, waitingAtClick)
	if fmt.Sprint(answers) != "[declined accepted]" {
		t.Errorf("answers %q, want [declined accepted]", answers)
	}
	if fmt.Sprint(waitingAtClick) != "[true true]" {
		t.Errorf("terminate: waiting at the clicks %v, want [true true]", waitingAtClick)
	}
}
