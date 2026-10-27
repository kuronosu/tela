//go:build linux

package freedesktop

import (
	"errors"
	"image"
	"os"
	"path/filepath"
	"slices"
	"strconv"
	"testing"
	"time"

	"github.com/godbus/dbus/v5"

	"github.com/kuronosu/tela/internal/platform"
)

// fakeWindow is a parent window for the dialogs (the harness's Parent
// returns "x11:2a" for any window).
type fakeWindow struct{ platform.Window }

func TestNoBus(t *testing.T) {
	h := newHarness(t, "unix:path=/nonexistent/tela-test-bus")
	if _, err := h.d.Notify(platform.Notification{Title: "x"}); !errors.Is(err, platform.ErrUnsupported) {
		t.Errorf("Notify without a bus = %v, want ErrUnsupported", err)
	}
	icon := []image.Image{image.NewNRGBA(image.Rect(0, 0, 16, 16))}
	if _, err := h.d.NewTray(platform.TrayOptions{Icon: icon}); !errors.Is(err, platform.ErrUnsupported) {
		t.Errorf("NewTray without a bus = %v, want ErrUnsupported", err)
	}
	if err := h.d.OpenFiles(platform.FileDialogOptions{}, func([]string, error) { t.Error("callback called") }); !errors.Is(err, platform.ErrUnsupported) {
		t.Errorf("OpenFiles without a bus = %v, want ErrUnsupported", err)
	}
	if err := h.d.MessageBox(platform.MessageBoxOptions{}, func(platform.MessageAnswer, error) {}); !errors.Is(err, errors.ErrUnsupported) {
		t.Errorf("MessageBox = %v, want ErrUnsupported", err)
	}
	// OpenURI falls back to xdg-open (the fake opener).
	if err := h.d.OpenURI(nil, "https://example.com/a?b=c"); err != nil {
		t.Fatalf("OpenURI without a bus = %v, want the xdg-open fallback", err)
	}
	waitOpened(t, h, "https://example.com/a?b=c")
}

func waitOpened(t *testing.T, h *harness, want string) {
	t.Helper()
	var got []byte
	eventually(t, "the fallback opener", func() bool {
		var err error
		got, err = os.ReadFile(h.opener)
		return err == nil
	})
	if string(got) != want {
		t.Errorf("the opener got %q, want %q", got, want)
	}
	os.Remove(h.opener)
}

func TestNoServices(t *testing.T) {
	addr := privateBus(t)
	h := newHarness(t, addr)
	if _, err := h.d.Notify(platform.Notification{Title: "x"}); !errors.Is(err, platform.ErrUnsupported) {
		t.Errorf("Notify without a server = %v, want ErrUnsupported", err)
	}
	icon := []image.Image{image.NewNRGBA(image.Rect(0, 0, 16, 16))}
	if _, err := h.d.NewTray(platform.TrayOptions{Icon: icon}); !errors.Is(err, platform.ErrUnsupported) {
		t.Errorf("NewTray without a watcher = %v, want ErrUnsupported", err)
	}
	if err := h.d.SaveFile(platform.FileDialogOptions{}, func(string, error) { t.Error("callback called") }); !errors.Is(err, platform.ErrUnsupported) {
		t.Errorf("SaveFile without a portal = %v, want ErrUnsupported", err)
	}
	dir := t.TempDir()
	if err := h.d.OpenPath(nil, dir); err != nil {
		t.Fatalf("OpenPath without a portal = %v, want the xdg-open fallback", err)
	}
	waitOpened(t, h, dir)
	if err := h.d.OpenPath(nil, filepath.Join(dir, "missing")); err == nil {
		t.Error("OpenPath of a missing file = nil, want an error")
	}

	// A portal without a FileChooser (xdg-desktop-portal-wlr alone).
	startPortal(t, addr, 0)
	if err := h.d.OpenFiles(platform.FileDialogOptions{}, func([]string, error) { t.Error("callback called") }); !errors.Is(err, platform.ErrUnsupported) {
		t.Errorf("OpenFiles without a FileChooser = %v, want ErrUnsupported", err)
	}
	h.d.Close()
	if _, err := h.d.Notify(platform.Notification{}); !errors.Is(err, platform.ErrUnsupported) {
		t.Errorf("Notify after Close = %v, want ErrUnsupported", err)
	}
}

func TestOpenURIPortal(t *testing.T) {
	addr := privateBus(t)
	p := startPortal(t, addr, 4)
	h := newHarness(t, addr)
	if err := h.d.OpenURI(fakeWindow{}, "mailto:ana@example.com"); err != nil {
		t.Fatal(err)
	}
	eventually(t, "OpenURI", func() bool { return len(p.callsSoFar()) == 1 })
	c := p.callsSoFar()[0]
	if c.Method != "OpenURI" || c.URI != "mailto:ana@example.com" || c.Parent != "x11:2a" {
		t.Errorf("portal call %+v", c)
	}

	file := filepath.Join(t.TempDir(), "report 1.txt")
	if err := os.WriteFile(file, []byte("x"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := h.d.OpenPath(nil, file); err != nil {
		t.Fatal(err)
	}
	eventually(t, "OpenFile", func() bool { return len(p.callsSoFar()) == 2 })
	want, _ := filepath.EvalSymlinks(file)
	if c := p.callsSoFar()[1]; c.Method != "OpenFile" || c.FDPath != want || c.Parent != "" {
		t.Errorf("portal call %+v, want OpenFile of %s", c, want)
	}

	// A portal that refuses falls back to xdg-open.
	p.mu.Lock()
	p.refuseOpens = true
	p.mu.Unlock()
	if err := h.d.OpenURI(nil, "https://example.com/"); err != nil {
		t.Fatal(err)
	}
	waitOpened(t, h, "https://example.com/")

	if err := h.d.OpenURI(nil, "file:///etc/hosts"); err == nil {
		t.Error("OpenURI of a file: URI = nil, want an error")
	}
}

func TestFileDialogs(t *testing.T) {
	addr := privateBus(t)
	p := startPortal(t, addr, 4)
	h := newHarness(t, addr)
	dir := t.TempDir()

	// OpenFiles: options, a Response before the method reply, and
	// percent-decoded paths.
	p.setResponder(true, func(portalCall) (uint32, []string) {
		return 0, []string{pathURI("/home/ana/My Files/a#1.png"), pathURI("/tmp/ñ.jpg")}
	})
	var got []string
	answered := false
	err := h.d.OpenFiles(platform.FileDialogOptions{
		Parent:    fakeWindow{},
		Title:     "Pick images",
		Directory: dir,
		Multiple:  true,
		Filters: []platform.FileFilter{
			{Name: "Images", Patterns: []string{"*.png", "*.jpg"}},
			{Patterns: []string{"*"}},
		},
	}, func(paths []string, err error) {
		if err != nil {
			t.Errorf("OpenFiles: %v", err)
		}
		got, answered = paths, true
	})
	if err != nil {
		t.Fatal(err)
	}
	if answered {
		t.Fatal("the callback ran inside OpenFiles")
	}
	h.pump("the OpenFiles answer", func() bool { return answered })
	if !slices.Equal(got, []string{"/home/ana/My Files/a#1.png", "/tmp/ñ.jpg"}) {
		t.Errorf("paths %q", got)
	}
	c := p.callsSoFar()[0]
	if c.Method != "OpenFile" || c.Title != "Pick images" || c.Parent != "x11:2a" {
		t.Errorf("call %+v", c)
	}
	var filters []portalFilter
	if err := c.Options["filters"].Store(&filters); err != nil {
		t.Fatalf("filters: %v (%v)", err, c.Options["filters"])
	}
	wantFilters := []portalFilter{
		{"Images", []portalRule{{0, "*.png"}, {0, "*.jpg"}}},
		{"*", []portalRule{{0, "*"}}},
	}
	if len(filters) != 2 || filters[0].Name != "Images" || !slices.Equal(filters[0].Rules, wantFilters[0].Rules) ||
		filters[1].Name != "*" || !slices.Equal(filters[1].Rules, wantFilters[1].Rules) {
		t.Errorf("filters %+v, want %+v", filters, wantFilters)
	}
	var current portalFilter
	if err := c.Options["current_filter"].Store(&current); err != nil || current.Name != "Images" {
		t.Errorf("current_filter %+v, %v", current, err)
	}
	if b, _ := c.Options["current_folder"].Value().([]byte); string(b) != dir+"\x00" {
		t.Errorf("current_folder %q, want %q with a NUL", b, dir)
	}
	if v, _ := c.Options["multiple"].Value().(bool); !v {
		t.Error("multiple is not set")
	}
	if v, _ := c.Options["modal"].Value().(bool); !v {
		t.Error("modal is not set for a dialog with a parent")
	}

	// A single-file dialog keeps one path; a late Response.
	p.setResponder(false, func(portalCall) (uint32, []string) {
		return 0, []string{pathURI("/a"), pathURI("/b")}
	})
	answered = false
	h.d.OpenFiles(platform.FileDialogOptions{}, func(paths []string, err error) { got, answered = paths, true })
	h.pump("the single OpenFiles answer", func() bool { return answered })
	if !slices.Equal(got, []string{"/a"}) {
		t.Errorf("single-file dialog paths %q", got)
	}

	// SaveFile: current_name; cancel is "" and nil.
	p.setResponder(false, func(portalCall) (uint32, []string) { return 1, nil })
	saved, saveErr := "x", error(nil)
	answered = false
	if err := h.d.SaveFile(platform.FileDialogOptions{Name: "report.pdf"}, func(path string, err error) {
		saved, saveErr, answered = path, err, true
	}); err != nil {
		t.Fatal(err)
	}
	h.pump("the SaveFile answer", func() bool { return answered })
	if saved != "" || saveErr != nil {
		t.Errorf("canceled SaveFile = %q, %v; want \"\", nil", saved, saveErr)
	}
	c = p.callsSoFar()[2]
	if c.Method != "SaveFile" || c.Options["current_name"].Value() != "report.pdf" {
		t.Errorf("SaveFile call %+v", c)
	}
	if v, _ := c.Options["modal"].Value().(bool); v {
		t.Error("modal is set for a dialog without a parent")
	}

	// PickFolder: directory mode, no filters; response 2 is an error.
	p.setResponder(false, func(portalCall) (uint32, []string) { return 2, nil })
	answered = false
	if err := h.d.PickFolder(platform.FileDialogOptions{Filters: []platform.FileFilter{{Patterns: []string{"*.x"}}}}, func(path string, err error) {
		saved, saveErr, answered = path, err, true
	}); err != nil {
		t.Fatal(err)
	}
	h.pump("the PickFolder answer", func() bool { return answered })
	if saved != "" || saveErr == nil {
		t.Errorf("PickFolder with response 2 = %q, %v; want an error", saved, saveErr)
	}
	c = p.callsSoFar()[3]
	if v, _ := c.Options["directory"].Value().(bool); c.Method != "OpenFile" || !v {
		t.Errorf("PickFolder call %+v", c)
	}
	if _, ok := c.Options["filters"]; ok {
		t.Error("PickFolder sent filters")
	}

	// Every request has its own token.
	tokens := map[string]bool{}
	for _, c := range p.callsSoFar() {
		tok, _ := c.Options["handle_token"].Value().(string)
		if tok == "" || tokens[tok] {
			t.Errorf("handle_token %q missing or reused", tok)
		}
		tokens[tok] = true
	}
}

func TestFileDialogOldHandles(t *testing.T) {
	addr := privateBus(t)
	p := startPortal(t, addr, 4)
	p.mu.Lock()
	p.oldHandles = true
	p.mu.Unlock()
	p.setResponder(false, func(portalCall) (uint32, []string) { return 0, []string{pathURI("/x/y")} })
	h := newHarness(t, addr)
	var got string
	if err := h.d.SaveFile(platform.FileDialogOptions{}, func(path string, err error) { got = path }); err != nil {
		t.Fatal(err)
	}
	h.pump("the answer at the portal's own handle", func() bool { return got != "" })
	if got != "/x/y" {
		t.Errorf("path %q", got)
	}
}

func TestPickFolderOldPortal(t *testing.T) {
	addr := privateBus(t)
	startPortal(t, addr, 2)
	h := newHarness(t, addr)
	if err := h.d.PickFolder(platform.FileDialogOptions{}, func(string, error) { t.Error("callback called") }); !errors.Is(err, platform.ErrUnsupported) {
		t.Errorf("PickFolder with FileChooser 2 = %v, want ErrUnsupported", err)
	}
}

func TestDialogAfterClose(t *testing.T) {
	addr := privateBus(t)
	p := startPortal(t, addr, 4)
	h := newHarness(t, addr)
	if err := h.d.OpenFiles(platform.FileDialogOptions{}, func([]string, error) { t.Error("callback after Close") }); err != nil {
		t.Fatal(err)
	}
	eventually(t, "the dialog", func() bool { return len(p.callsSoFar()) == 1 })
	h.d.Close()
	c := p.callsSoFar()[0]
	p.conn.Emit(c.Handle, ifaceRequest+".Response", uint32(0), map[string]dbus.Variant{"uris": dbus.MakeVariant([]string{pathURI("/a")})})
	time.Sleep(50 * time.Millisecond)
	h.d.Dispatch()
	if len(h.posted) != 0 {
		t.Errorf("%d callbacks posted after Close", len(h.posted))
	}
}

func TestNotifications(t *testing.T) {
	addr := privateBus(t)
	n := startNotifier(t, addr, "actions", "body-markup")
	h := newHarness(t, addr)
	icon := image.NewNRGBA(image.Rect(0, 0, 32, 32))
	id, err := h.d.Notify(platform.Notification{
		Title:   "Build done",
		Body:    "3 < 4 & ok",
		Icon:    []image.Image{image.NewNRGBA(image.Rect(0, 0, 16, 16)), icon, image.NewNRGBA(image.Rect(0, 0, 512, 512))},
		Actions: []platform.NotificationAction{{ID: "open", Label: "Open"}, {ID: "later", Label: "Later"}},
	})
	if err != nil || id != 1 {
		t.Fatalf("Notify = %d, %v", id, err)
	}
	eventually(t, "the notification", func() bool { return len(n.all()) == 1 })
	got := n.all()[0]
	if got.AppName != "tela-test" || got.Summary != "Build done" || got.Body != "3 &lt; 4 &amp; ok" {
		t.Errorf("notification %+v", got)
	}
	if want := []string{"default", "", "open", "Open", "later", "Later"}; !slices.Equal(got.Actions, want) {
		t.Errorf("actions %q, want %q", got.Actions, want)
	}
	if got.Hints["desktop-entry"].Value() != "tela-test" {
		t.Errorf("desktop-entry hint %v", got.Hints["desktop-entry"])
	}
	var img imageData
	if err := got.Hints["image-data"].Store(&img); err != nil || img.Width != 512 || img.Channels != 4 {
		// PickIcon(128) takes the smallest image of at least 128.
		t.Errorf("image-data %dx%d, %v; want the 512 image (the smallest of at least %d)", img.Width, img.Height, err, notificationIconSide)
	}

	server := got.ID
	n.emit("ActivationToken", server, "token-1")
	n.emit("ActionInvoked", server, "open")
	if e := h.event("ActionInvoked"); e != (platform.NotificationEvent{ID: id, Kind: platform.NotificationActionInvoked, Action: "open"}) {
		t.Errorf("event %+v", e)
	}
	if tok := h.d.TakeActivationToken(); tok != "token-1" {
		t.Errorf("activation token %q", tok)
	}
	n.emit("ActionInvoked", server, "default")
	if e := h.event("default action"); e != (platform.NotificationEvent{ID: id, Kind: platform.NotificationActivated}) {
		t.Errorf("event %+v", e)
	}
	n.emit("ActionInvoked", uint32(999), "open") // not ours
	n.emit("NotificationClosed", server, uint32(2))
	if e := h.event("NotificationClosed"); e != (platform.NotificationEvent{ID: id, Kind: platform.NotificationClosed}) {
		t.Errorf("event %+v", e)
	}
	if len(h.events) != 0 {
		t.Errorf("extra events %+v", h.events)
	}

	// CloseNotification: the server is asked; no event follows.
	id2, err := h.d.Notify(platform.Notification{Title: "second"})
	if err != nil || id2 != 2 {
		t.Fatalf("Notify = %d, %v", id2, err)
	}
	eventually(t, "the second notification", func() bool { return len(n.all()) == 2 })
	if err := h.d.CloseNotification(id2); err != nil {
		t.Fatal(err)
	}
	eventually(t, "CloseNotification", func() bool { return n.all()[1].Closed })
	if err := h.d.CloseNotification(id2); err != nil {
		t.Errorf("closing twice = %v", err)
	}
	if err := h.d.CloseNotification(77); err != nil {
		t.Errorf("closing an unknown ID = %v", err)
	}
	// A close before the server answered is sent with the answer.
	id3, _ := h.d.Notify(platform.Notification{Title: "third"})
	h.d.CloseNotification(id3)
	eventually(t, "the early close", func() bool { all := n.all(); return len(all) == 3 && all[2].Closed })
	time.Sleep(50 * time.Millisecond)
	h.d.Dispatch()
	if len(h.events) != 0 {
		t.Errorf("events after CloseNotification: %+v", h.events)
	}
}

func TestNotificationsPlain(t *testing.T) {
	addr := privateBus(t)
	n := startNotifier(t, addr) // no capabilities: no markup, no actions
	h := newHarness(t, addr)
	if _, err := h.d.Notify(platform.Notification{Body: "a < b", Actions: []platform.NotificationAction{{ID: "x", Label: "X"}}}); err != nil {
		t.Fatal(err)
	}
	eventually(t, "the notification", func() bool { return len(n.all()) == 1 })
	got := n.all()[0]
	if got.Body != "a < b" || !slices.Equal(got.Actions, []string{"default", ""}) {
		t.Errorf("notification %+v", got)
	}
	if _, ok := got.Hints["image-data"]; ok {
		t.Error("image-data without an icon")
	}
}

func TestTray(t *testing.T) {
	addr := privateBus(t)
	w := startWatcher(t, addr)
	h := newHarness(t, addr)
	host := connect(t, addr)
	sigs := make(chan *dbus.Signal, 32)
	host.Signal(sigs)

	icon := func(n int) image.Image { return image.NewNRGBA(image.Rect(0, 0, n, n)) }
	id, err := h.d.NewTray(platform.TrayOptions{Icon: []image.Image{icon(32), icon(16)}, Tooltip: "Tela", Menu: testMenu})
	if err != nil || id != 1 {
		t.Fatalf("NewTray = %d, %v", id, err)
	}
	eventually(t, "the registration", func() bool { return len(w.registered()) == 1 })
	reg := w.registered()[0]
	if reg.Service != "org.kde.StatusNotifierItem-"+strconv.Itoa(os.Getpid())+"-1" {
		t.Errorf("registered %q", reg.Service)
	}
	item := host.Object(reg.Service, itemPath)
	var props map[string]dbus.Variant
	if err := item.Call(propsIface+".GetAll", 0, itemIface).Store(&props); err != nil {
		t.Fatal(err)
	}
	var pixmaps []pixmap
	props["IconPixmap"].Store(&pixmaps)
	if len(pixmaps) != 2 || pixmaps[0].Width != 16 || len(pixmaps[1].Data) != 32*32*4 {
		t.Errorf("IconPixmap %d images", len(pixmaps))
	}
	var tip toolTip
	if err := props["ToolTip"].Store(&tip); err != nil || tip.Title != "Tela" {
		t.Errorf("ToolTip %+v, %v", tip, err)
	}
	if props["ItemIsMenu"].Value() != false || props["Menu"].Value() != menuPath || props["Id"].Value() != "tela-test" || props["Status"].Value() != "Active" {
		t.Errorf("properties %v", props)
	}
	if err := item.Call("org.freedesktop.DBus.Introspectable.Introspect", 0).Err; err != nil {
		t.Errorf("Introspect: %v", err)
	}

	if err := item.Call(itemIface+".Activate", 0, int32(10), int32(20)).Err; err != nil {
		t.Fatal(err)
	}
	if e := h.event("Activate"); e != (platform.TrayEvent{Tray: id, Kind: platform.TrayActivated}) {
		t.Errorf("event %+v", e)
	}
	item.Call(itemIface+".SecondaryActivate", 0, int32(0), int32(0))
	item.Call(itemIface+".ContextMenu", 0, int32(0), int32(0))

	// The menu.
	menu := host.Object(reg.Service, menuPath)
	var rev uint32
	var layout layoutItem
	if err := menu.Call(menuIface+".GetLayout", 0, int32(0), int32(-1), []string{}).Store(&rev, &layout); err != nil {
		t.Fatal(err)
	}
	if len(layout.Children) != 5 {
		t.Fatalf("layout has %d items, want 5", len(layout.Children))
	}
	var open, more layoutItem
	if err := layout.Children[0].Store(&open); err != nil {
		t.Fatal(err)
	}
	layout.Children[4].Store(&more)
	if open.Props["label"].Value() != "Open__File" || len(more.Children) != 2 {
		t.Errorf("layout: %v; submenu %d items", open.Props, len(more.Children))
	}
	var about, disabled layoutItem
	more.Children[0].Store(&disabled)
	more.Children[1].Store(&about)
	if err := menu.Call(menuIface+".Event", 0, about.ID, "clicked", dbus.MakeVariant(""), uint32(0)).Err; err != nil {
		t.Fatal(err)
	}
	if e := h.event("a menu click"); e != (platform.TrayEvent{Tray: id, Kind: platform.TrayMenuItem, Item: 12}) {
		t.Errorf("event %+v", e)
	}
	// Hovering, a disabled item and a submenu are not activations.
	menu.Call(menuIface+".Event", 0, open.ID, "hovered", dbus.MakeVariant(""), uint32(0))
	menu.Call(menuIface+".Event", 0, disabled.ID, "clicked", dbus.MakeVariant(""), uint32(0))
	menu.Call(menuIface+".Event", 0, more.ID, "clicked", dbus.MakeVariant(""), uint32(0))
	if err := menu.Call(menuIface+".Event", 0, int32(9999), "clicked", dbus.MakeVariant(""), uint32(0)).Err; err == nil {
		t.Error("Event on an unknown id = nil, want an error")
	}
	var idErrors []int32
	if err := menu.Call(menuIface+".EventGroup", 0, []menuEvent{
		{open.ID, "clicked", dbus.MakeVariant(""), 0},
		{int32(9999), "clicked", dbus.MakeVariant(""), 0},
	}).Store(&idErrors); err != nil || !slices.Equal(idErrors, []int32{9999}) {
		t.Errorf("EventGroup = %v, %v", idErrors, err)
	}
	if e := h.event("a grouped menu click"); e != (platform.TrayEvent{Tray: id, Kind: platform.TrayMenuItem, Item: 7}) {
		t.Errorf("event %+v", e)
	}
	var group []itemProps
	if err := menu.Call(menuIface+".GetGroupProperties", 0, []int32{open.ID, 9999}, []string{"label"}).Store(&group); err != nil ||
		len(group) != 1 || group[0].ID != open.ID || len(group[0].Props) != 1 {
		t.Errorf("GetGroupProperties = %+v, %v", group, err)
	}
	var label dbus.Variant
	if err := menu.Call(menuIface+".GetProperty", 0, open.ID, "label").Store(&label); err != nil || label.Value() != "Open__File" {
		t.Errorf("GetProperty = %v, %v", label, err)
	}
	var version dbus.Variant
	if err := menu.Call(propsIface+".Get", 0, menuIface, "Version").Store(&version); err != nil || version.Value() != uint32(3) {
		t.Errorf("Version = %v, %v", version, err)
	}

	// UpdateTray: a new tooltip and menu.
	if err := host.AddMatchSignal(dbus.WithMatchSender(reg.Service)); err != nil {
		t.Fatal(err)
	}
	menu2 := cloneMenu(testMenu)
	menu2[2].Checked = false
	if err := h.d.UpdateTray(id, platform.TrayOptions{Icon: []image.Image{icon(32), icon(16)}, Tooltip: "Tela (2)", Menu: menu2}); err != nil {
		t.Fatal(err)
	}
	seen := map[string]bool{}
	deadline := time.After(5 * time.Second)
	for !seen[menuIface+".LayoutUpdated"] || !seen[itemIface+".NewToolTip"] {
		select {
		case s := <-sigs:
			seen[s.Name] = true
			if s.Name == menuIface+".LayoutUpdated" && s.Body[0] != uint32(2) {
				t.Errorf("LayoutUpdated revision %v, want 2", s.Body[0])
			}
		case <-deadline:
			t.Fatalf("signals after UpdateTray: %v", seen)
		}
	}
	if seen[itemIface+".NewIcon"] {
		t.Error("NewIcon for an unchanged icon")
	}
	var layout2 layoutItem
	menu.Call(menuIface+".GetLayout", 0, int32(0), int32(-1), []string{}).Store(&rev, &layout2)
	var dark layoutItem
	layout2.Children[2].Store(&dark)
	if rev != 2 || dark.Props["toggle-state"].Value() != int32(0) {
		t.Errorf("after the update: revision %d, %v", rev, dark.Props)
	}
	if err := h.d.UpdateTray(99, platform.TrayOptions{Icon: []image.Image{icon(16)}}); err == nil {
		t.Error("UpdateTray of an unknown tray = nil, want an error")
	}

	// A second tray, and a watcher that restarts.
	id2, err := h.d.NewTray(platform.TrayOptions{Icon: []image.Image{icon(16)}})
	if err != nil || id2 != 2 {
		t.Fatalf("second NewTray = %d, %v", id2, err)
	}
	eventually(t, "the second registration", func() bool { return len(w.registered()) == 2 })
	var menuPathOf2 dbus.Variant
	host.Object(w.registered()[1].Service, itemPath).Call(propsIface+".Get", 0, itemIface, "Menu").Store(&menuPathOf2)
	if menuPathOf2.Value() != noMenuPath {
		t.Errorf("Menu of a tray without a menu = %v, want %s", menuPathOf2, noMenuPath)
	}
	w.conn.Close()
	w2 := startWatcher(t, addr)
	eventually(t, "registrations with the new watcher", func() bool { return len(w2.registered()) == 2 })

	// RemoveTray: the item's name goes away.
	if err := h.d.RemoveTray(id); err != nil {
		t.Fatal(err)
	}
	eventually(t, "the item to vanish", func() bool {
		var has bool
		host.BusObject().Call(busIface+".NameHasOwner", 0, reg.Service).Store(&has)
		return !has
	})
	if err := h.d.RemoveTray(id); err != nil {
		t.Errorf("removing twice = %v", err)
	}
	if err := h.d.UpdateTray(id, platform.TrayOptions{Icon: []image.Image{icon(16)}}); err == nil {
		t.Error("UpdateTray of a removed tray = nil, want an error")
	}
	// Close removes the rest.
	second := "org.kde.StatusNotifierItem-" + strconv.Itoa(os.Getpid()) + "-2"
	h.d.Close()
	eventually(t, "Close to remove the trays", func() bool {
		var has bool
		host.BusObject().Call(busIface+".NameHasOwner", 0, second).Store(&has)
		return !has
	})
}
