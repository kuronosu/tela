package kit_test

import (
	"fmt"
	"os"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/kuronosu/tela"
	"github.com/kuronosu/tela/h"
	"github.com/kuronosu/tela/kit"
	"github.com/kuronosu/tela/telatest"
)

// The Chromium side of the toast sheet is the DOM that sonner 2.0.7 renders
// in shadcn/ui's wrapper (new-york-v4/ui/sonner.tsx at kit.ShadcnCommit): a
// React page with the real library, whose scenarios are in
// testdata/sonner/capture.jsx and whose output (the <section> element after
// the toasts mounted) is in testdata/sonner/<scenario>.html, next to
// sonner's own styles.css (MIT, see THIRD_PARTY.md). To capture again:
//
//	npm i sonner@2.0.7 react@19 react-dom@19 lucide-react@0.474.0 esbuild
//	esbuild capture.jsx --bundle --outfile=bundle.js --define:process.env.NODE_ENV='"production"' --loader:.js=jsx
//
// and load a page with shadcn's CSS (a sheet's gallery.css) and bundle.js with
// ?s=<scenario> in Chromium, reading the element out of the DOM.
//
// A page is not a window, so the toaster is made absolute (sonner's own
// [data-sonner-toaster] is fixed) inside a box of the cell; the dark
// panel's colors are sonner's dark theme rules for the one attribute that
// differs per panel; and the font is Inter, the page's (sonner's own
// stack is the system's, which Tela, with embedded fonts, cannot use).
const sonnerOverrides = `
[data-sonner-toaster] { position: absolute; font-family: Inter; }
.dark [data-sonner-toaster] [data-description] { color: hsl(0, 0%, 91%); }
.dark [data-sonner-toast][data-styled='true'] [data-close-button] { border-color: var(--normal-border); color: var(--normal-text); }
.dark [data-sonner-toast][data-styled='true']:hover [data-close-button]:hover { background: hsl(0, 0%, 12%); border-color: hsl(0, 0%, 25%); }
`

func readFixture(t testing.TB, name string) string {
	t.Helper()
	b, err := os.ReadFile("testdata/sonner/" + name)
	if err != nil {
		t.Fatal(err)
	}
	return strings.TrimSpace(string(b))
}

// seedToast is a toast of a scenario, the same on both sides.
type seedToast struct {
	kind, title, desc, action string
}

// toastSeed shows the scenario's toasts when it mounts, in order.
type toastSeed struct{ toasts []seedToast }

func (s toastSeed) Build() tela.Node {
	toast := kit.UseToast()
	tela.UseMount(func() func() {
		for _, st := range s.toasts {
			var opts []kit.ToastOption
			if st.desc != "" {
				opts = append(opts, kit.ToastDescription(st.desc))
			}
			if st.action != "" {
				opts = append(opts, kit.ToastAction(st.action, func() {}))
			}
			switch st.kind {
			case "success":
				toast.Success(st.title, opts...)
			case "error":
				toast.Error(st.title, opts...)
			case "info":
				toast.Info(st.title, opts...)
			case "warning":
				toast.Warning(st.title, opts...)
			case "loading":
				toast.Loading(st.title, opts...)
			default:
				toast.Show(st.title, opts...)
			}
		}
		return nil
	})
	return h.Fragment()
}

// toastBox is the box of a cell the toaster is positioned in.
type toastBox struct {
	height  int
	toaster kit.Toaster
	seed    []seedToast
}

func (b toastBox) Build() tela.Node {
	b.toaster.Inline = true
	b.toaster.Duration = kit.ToastForever
	b.toaster.Children = toastSeed{b.seed}
	return h.Div(h.Class(fmt.Sprintf("relative h-[%dpx] w-full", b.height)), b.toaster)
}

func toastCell(t testing.TB, name, fixture string, height int, toaster kit.Toaster, seed []seedToast) cell {
	toaster.Props = h.TestID(name + "-viewport")
	markup := readFixture(t, fixture+".html")
	markup = strings.Replace(markup, "<ol ", fmt.Sprintf("<ol data-probe=%q ", name+"-viewport"), 1)
	return cell{
		name: name, width: 380, probes: []string{name + "-viewport"},
		tela: toastBox{height: height, toaster: toaster, seed: seed},
		html: fmt.Sprintf(`<div class="relative h-[%dpx] w-full">%s</div>`, height, markup),
	}
}

var (
	stackSeed = []seedToast{
		{"", "Event has been created", "Sunday, December 03, 2023 at 9:00 AM", ""},
		{"success", "Saved", "Your changes were saved.", ""},
		{"error", "Something went wrong", "", ""},
		{"", "Undo available", "", "Undo"},
	}
	kindsSeed = []seedToast{
		{"loading", "Uploading file", "", ""},
		{"warning", "Low disk space", "Only 2 GB are left.", ""},
		{"info", "A new version is available", "", ""},
		{"success", "Profile updated", "", ""},
	}
)

func toastSheet(t testing.TB) *sheet {
	return &sheet{name: "toast", width: 840, cells: []cell{
		toastCell(t, "single", "single", 130, kit.Toaster{}, stackSeed[:1]),
		toastCell(t, "stack", "stack", 130, kit.Toaster{}, stackSeed),
		toastCell(t, "expanded", "expanded", 280, kit.Toaster{Expand: true}, stackSeed),
		toastCell(t, "kinds", "kinds", 330, kit.Toaster{Expand: true, VisibleToasts: 4}, kindsSeed),
		toastCell(t, "top", "top", 200, kit.Toaster{Position: kit.ToastTopCenter}, []seedToast{stackSeed[1], stackSeed[3]}),
		toastCell(t, "close", "close", 200, kit.Toaster{CloseButton: true, Expand: true}, []seedToast{stackSeed[0], stackSeed[2]}),
	}, css: readFixture(t, "styles.css") + sonnerOverrides}
}

func TestToastGolden(t *testing.T)   { toastSheet(t).golden(t) }
func TestChromiumToast(t *testing.T) { toastSheet(t).chromium(t) }

var _ tela.Component = kit.Toaster{}

// toastApp is an app with a Toaster and a few buttons that show toasts.
type toastApp struct {
	got *kit.ToastHandle
	cfg kit.Toaster
}

func (a toastApp) Build() tela.Node {
	cfg := a.cfg
	cfg.Children = toastButtons{a.got}
	return h.Fragment(cfg)
}

type toastButtons struct{ got *kit.ToastHandle }

func (b toastButtons) Build() tela.Node {
	toast := kit.UseToast()
	*b.got = toast
	return h.Div(h.Class("p-4"),
		kit.Button{OnClick: func() { toast.Show("Event created", kit.ToastDescription("Sunday at 9:00")) }, Children: h.Text("Show")},
		kit.Button{OnClick: func() { toast.Error("Failed") }, Children: h.Text("Fail")},
	)
}

func newToastApp(t *testing.T, cfg kit.Toaster, opts ...telatest.Option) (*telatest.App, *kit.ToastHandle) {
	t.Helper()
	var handle kit.ToastHandle
	opts = append([]telatest.Option{telatest.Size(480, 360), telatest.Theme(kit.Theme()), telatest.RootClass("bg-background text-foreground")}, opts...)
	return telatest.Mount(t, toastApp{&handle, cfg}, opts...), &handle
}

func TestToastShowFromAHandlerAndAutoDismiss(t *testing.T) {
	app, _ := newToastApp(t, kit.Toaster{}, telatest.NoSettle())
	app.Click(telatest.ByRole("button", "Show"))
	app.Advance(500 * time.Millisecond) // the entrance
	app.ExpectText("Event created")
	app.ExpectText("Sunday at 9:00")
	app.ExpectCount(telatest.ByRole("status", ""), 1)
	// It stays for 4 seconds of the frame clock, then slides out.
	app.Advance(3 * time.Second)
	app.ExpectCount(telatest.ByRole("status", ""), 1)
	app.Advance(600 * time.Millisecond)
	app.Advance(time.Second)
	app.ExpectNoText("Event created")
}

func TestToastFromAGoroutine(t *testing.T) {
	app, handle := newToastApp(t, kit.Toaster{Duration: kit.ToastForever})
	var wg sync.WaitGroup
	ids := make([]kit.ToastID, 8)
	for i := range ids {
		wg.Add(1)
		go func() {
			defer wg.Done()
			ids[i] = handle.Show(fmt.Sprintf("From goroutine %d", i))
		}()
	}
	wg.Wait()
	app.ExpectText("From goroutine 7")
	app.ExpectCount(telatest.ByRole("status", ""), 8)
	seen := map[kit.ToastID]bool{}
	for _, id := range ids {
		if id == 0 || seen[id] {
			t.Errorf("ids %v are not distinct and non-zero", ids)
		}
		seen[id] = true
	}
	// Dismissing from a goroutine too.
	wg.Add(1)
	go func() { defer wg.Done(); handle.DismissAll() }()
	wg.Wait()
	app.ExpectCount(telatest.ByRole("status", ""), 0)
}

func TestToastHandleIsComparable(t *testing.T) {
	_, a := newToastApp(t, kit.Toaster{})
	_, b := newToastApp(t, kit.Toaster{})
	same := *a
	if same != *a || *a == *b {
		t.Error("a handle equals itself and no other toaster's")
	}
	m := map[kit.ToastHandle]bool{*a: true}
	if !m[*a] {
		t.Error("a handle is a map key")
	}
}

func TestToastStacksNewestFirstAndShowsThree(t *testing.T) {
	app, handle := newToastApp(t, kit.Toaster{Duration: kit.ToastForever})
	for i := 1; i <= 5; i++ {
		handle.Show(fmt.Sprintf("Toast %d", i))
	}
	app.ExpectCount(telatest.ByRole("status", ""), 5)
	front := app.Bounds(telatest.ByText("Toast 5", telatest.Exact()))
	behind := app.Bounds(telatest.ByText("Toast 4", telatest.Exact()))
	if front.Y <= behind.Y {
		t.Errorf("the newest toast is at y %v, the one before at %v: want it below (bottom right stack)", front.Y, behind.Y)
	}
	// Collapsed, the ones behind are drawn small and without content: the
	// stack as a whole is as tall as the front toast plus the peeks.
	snap := app.AriaSnapshot()
	if strings.Index(snap, "Toast 5") > strings.Index(snap, "Toast 4") {
		t.Errorf("the newest toast comes first in the tree:\n%s", snap)
	}
}

func TestToastPauseOnHover(t *testing.T) {
	app, handle := newToastApp(t, kit.Toaster{}, telatest.NoSettle())
	handle.Show("Hover me")
	app.Advance(500 * time.Millisecond)
	app.Advance(2 * time.Second) // 2 of the 4 seconds
	app.Hover(telatest.ByText("Hover me", telatest.Exact()))
	app.Advance(10 * time.Second) // paused
	app.ExpectText("Hover me")
	app.MouseMove(5, 5) // off the toasts: the rest of the time runs
	app.Advance(1500 * time.Millisecond)
	app.ExpectText("Hover me")
	app.Advance(time.Second)
	app.Advance(time.Second)
	app.ExpectNoText("Hover me")
}

func TestToastActionAndClose(t *testing.T) {
	var undone int
	var dismissed []string
	app, handle := newToastApp(t, kit.Toaster{Duration: kit.ToastForever, CloseButton: true})
	handle.Show("Archived", kit.ToastAction("Undo", func() { undone++ }), kit.ToastOnDismiss(func() { dismissed = append(dismissed, "archived") }))
	app.Click(telatest.ByRole("button", "Undo"))
	if undone != 1 {
		t.Errorf("the action ran %d times, want 1", undone)
	}
	app.ExpectNoText("Archived")
	handle.Show("Closed by hand", kit.ToastOnDismiss(func() { dismissed = append(dismissed, "closed") }))
	app.Click(telatest.ByRole("button", "Close toast"))
	app.ExpectNoText("Closed by hand")
	if strings.Join(dismissed, ",") != "archived,closed" {
		t.Errorf("OnDismiss saw %v, want archived then closed", dismissed)
	}
}

func TestToastDismissAndAutoCloseCallbacks(t *testing.T) {
	var log []string
	app, handle := newToastApp(t, kit.Toaster{}, telatest.NoSettle())
	a := handle.Show("A", kit.ToastOnDismiss(func() { log = append(log, "dismiss A") }))
	handle.Show("B", kit.ToastDuration(time.Second), kit.ToastOnAutoClose(func() { log = append(log, "auto B") }), kit.ToastOnDismiss(func() { log = append(log, "dismiss B") }))
	app.Advance(500 * time.Millisecond)
	handle.Dismiss(a)
	app.Advance(1200 * time.Millisecond)
	handle.Dismiss(a) // gone already
	app.Advance(time.Second)
	if strings.Join(log, ",") != "dismiss A,auto B" {
		t.Errorf("callbacks %v, want dismiss A then auto B (an auto-close is not a dismiss)", log)
	}
}

func TestToastReplaceKeepsPlace(t *testing.T) {
	app, handle := newToastApp(t, kit.Toaster{})
	id := handle.Loading("Uploading")
	handle.Show("Other")
	app.ExpectText("Uploading")
	// A loading toast does not time out, and a replacement does.
	app.Advance(10 * time.Second)
	app.ExpectText("Uploading")
	handle.Success("Uploaded", kit.ToastReplace(id))
	app.ExpectText("Uploaded")
	app.ExpectNoText("Uploading")
	app.ExpectCount(telatest.ByRole("status", ""), 1) // "Other" went with the clock
	app.Advance(5 * time.Second)
	app.ExpectNoText("Uploaded")
}

func TestToastRoles(t *testing.T) {
	app, handle := newToastApp(t, kit.Toaster{Duration: kit.ToastForever})
	handle.Show("Plain")
	handle.Success("Fine")
	handle.Error("Broken")
	app.ExpectCount(telatest.ByRole("status", ""), 2)
	app.ExpectCount(telatest.ByRole("alert", ""), 1)
	app.ExpectRole("region", "Notifications")
}

func TestToastPositions(t *testing.T) {
	var ys []float32
	for _, pos := range []kit.ToastPosition{kit.ToastTopLeft, kit.ToastTopCenter, kit.ToastTopRight, kit.ToastBottomLeft, kit.ToastBottomCenter, kit.ToastBottomRight} {
		app, handle := newToastApp(t, kit.Toaster{Position: pos, Duration: kit.ToastForever})
		handle.Show("Where")
		b := app.Bounds(telatest.ByText("Where", telatest.Exact()))
		ys = append(ys, b.Y)
		switch {
		case strings.HasSuffix(string(pos), "left") && b.X > 60,
			strings.HasSuffix(string(pos), "right") && b.X < 100,
			strings.HasSuffix(string(pos), "center") && (b.X < 60 || b.X > 100):
			t.Errorf("%s: the toast's text is at x %v", pos, b.X)
		}
	}
	if !(ys[0] < 100 && ys[1] < 100 && ys[2] < 100 && ys[3] > 200 && ys[4] > 200 && ys[5] > 200) {
		t.Errorf("tops %v: want the top positions near the top and the bottom ones near the bottom", ys)
	}
}

func TestToastWithoutAToasterPanics(t *testing.T) {
	_, err := telatest.Render(toastButtons{new(kit.ToastHandle)})
	if err == nil || !strings.Contains(err.Error(), "wrap the app in kit.Toaster") {
		t.Errorf("error %v, want the missing Toaster reported", err)
	}
	defer func() {
		if r := recover(); r == nil || !strings.Contains(fmt.Sprint(r), "zero value") {
			t.Errorf("recovered %v, want the zero handle reported", r)
		}
	}()
	var zero kit.ToastHandle
	zero.Show("x")
}

func TestToastIdleRequestsNoFramesBeyondTheTimer(t *testing.T) {
	app, handle := newToastApp(t, kit.Toaster{}, telatest.NoSettle())
	handle.Show("Waiting")
	app.Advance(time.Second) // entrance and measurement done
	before := app.Stats()
	app.Advance(2 * time.Second)
	after := app.Stats()
	if after.Frames != before.Frames {
		t.Errorf("a waiting toast drew %d frames in 2 s, want 0", after.Frames-before.Frames)
	}
	app.ExpectText("Waiting")
}

func TestToastStyleAndClass(t *testing.T) {
	s := kit.DefaultStyle()
	s.Toast.Item += " rounded-none"
	app := telatest.Mount(t, kit.StyleProvider{Style: &s, Children: toastApp{new(kit.ToastHandle), kit.Toaster{ToastClass: "bg-black", Class: "w-96"}}})
	_ = app
}

// TestToastHoverFansOutTheStack looks at the stack under the pointer: the
// toasts spread, each in full, and the timers stop.
func TestToastHoverFansOutTheStack(t *testing.T) {
	app, handle := newToastApp(t, kit.Toaster{})
	handle.Show("First", kit.ToastDescription("The oldest one."))
	handle.Success("Second")
	handle.Show("Third", kit.ToastAction("Undo", func() {}))
	collapsed := app.Bounds(telatest.ByText("First", telatest.Exact()))
	app.Hover(telatest.ByText("Third", telatest.Exact()))
	app.GoldenScales("toast_hover", 1, 2)
	fanned := app.Bounds(telatest.ByText("First", telatest.Exact()))
	if fanned.Y >= collapsed.Y-20 {
		t.Errorf("the oldest toast is at y %v hovered and %v collapsed: want it well above", fanned.Y, collapsed.Y)
	}
	app.MouseMove(5, 5)
	back := app.Bounds(telatest.ByText("First", telatest.Exact()))
	if back.Y != collapsed.Y {
		t.Errorf("the stack did not collapse again: y %v, want %v", back.Y, collapsed.Y)
	}
}

// seedOnClick shows a toast from a button, as an app does.
type seedOnClick struct{}

func (seedOnClick) Build() tela.Node {
	toast := kit.UseToast()
	return h.Fragment(kit.Button{
		OnClick:  func() { toast.Success("Saved", kit.ToastDescription("Your changes are live.")) },
		Children: h.Text("Save"),
	})
}

func ExampleToaster() {
	t := &exampleT{}
	defer t.close()
	// The Toaster goes once around the app; any component below it shows
	// toasts with the handle of kit.UseToast.
	app := telatest.Mount(t, kit.Toaster{Children: seedOnClick{}})
	app.Click(telatest.ByRole("button", "Save"))
	app.ExpectText("Your changes are live.")
	fmt.Println("toast shown")
	// Output: toast shown
}
