package core

import (
	"strings"
	"testing"

	"github.com/kuronosu/tela/internal/platform"
)

// useLabelSource installs src for the test.
func useLabelSource(t *testing.T, src LabelSource) {
	t.Helper()
	SetLabelSource(src)
	t.Cleanup(func() { SetLabelSource(nil) })
}

func TestLabelDefaultsAndSource(t *testing.T) {
	if got := Label("tela.toast.close"); got != "Close toast" {
		t.Errorf("Label = %q, want the English text", got)
	}
	if got := Label("tela.menu.quit", "name", "Notes"); got != "Quit Notes" {
		t.Errorf("Label with a name = %q", got)
	}
	useLabelSource(t, func(id string, args ...any) (string, bool) {
		switch id {
		case "tela.toast.close":
			return "Cerrar aviso", true
		case "tela.menu.quit":
			return "Salir de " + args[1].(string), true
		}
		return "", false
	})
	if got := Label("tela.toast.close"); got != "Cerrar aviso" {
		t.Errorf("Label with a source = %q", got)
	}
	if got := Label("tela.menu.quit", "name", "Notas"); got != "Salir de Notas" {
		t.Errorf("Label with a source and a name = %q", got)
	}
	if got := Label("tela.toast.region"); got != "Notifications" {
		t.Errorf("Label the source lacks = %q, want the English text", got)
	}
	// The platform gets the answers, the names as placeholders to fill.
	want := map[string]string{"tela.toast.close": "Cerrar aviso", "tela.menu.quit": "Salir de {name}"}
	got := LabelOverrides()
	if len(got) != len(want) || got["tela.toast.close"] != want["tela.toast.close"] || got["tela.menu.quit"] != want["tela.menu.quit"] {
		t.Errorf("LabelOverrides = %v, want %v", got, want)
	}
}

func TestLabelUnknownIDPanics(t *testing.T) {
	defer func() {
		r := recover()
		if s, _ := r.(string); !strings.Contains(s, `unknown label ID "tela.nope"`) {
			t.Errorf("panic = %v, want one naming the ID", r)
		}
	}()
	Label("tela.nope")
}

// Every ID a backend reads is a label, with the same English text a
// backend would show (Close is the macOS "Close Window" and differs).
func TestPlatformStringIDsAreLabels(t *testing.T) {
	for _, id := range platform.StringIDs {
		if _, ok := LabelDefault(id); !ok {
			t.Errorf("platform.StringIDs has %q, which is not a label", id)
		}
	}
}

// The fallback title bar's names follow the source, and rebuild when the
// language changes (LabelsChanged).
func TestFallbackTitleBarLabels(t *testing.T) {
	x := newChromeHarness(t, true, platform.SystemSettings{}, Options{Window: WindowSpec{Title: "App"}, Root: div()})
	if snap := x.snapshot(x.windows()[0]); !strings.Contains(snap, `button "Close"`) || !strings.Contains(snap, `group "Window controls"`) {
		t.Fatalf("the bar before a source:\n%s", snap)
	}
	spanish := map[string]string{
		"tela.titlebar.minimize": "Minimizar", "tela.titlebar.maximize": "Maximizar",
		"tela.titlebar.restore": "Restaurar", "tela.titlebar.close": "Cerrar", "tela.titlebar.controls": "Controles de ventana",
	}
	useLabelSource(t, func(id string, _ ...any) (string, bool) { s, ok := spanish[id]; return s, ok })
	LabelsChanged()
	x.b.Wake()
	x.drain()
	snap := x.snapshot(x.windows()[0])
	for _, want := range []string{`button "Minimizar"`, `button "Maximizar"`, `button "Cerrar"`, `group "Controles de ventana"`} {
		if !strings.Contains(snap, want) {
			t.Errorf("the bar lacks %s after LabelsChanged:\n%s", want, snap)
		}
	}
}

// The backend is given the translated texts when the host starts and again
// when LabelsChanged runs.
func TestHostGivesTheBackendItsStrings(t *testing.T) {
	useLabelSource(t, func(id string, _ ...any) (string, bool) {
		if id == "tela.messagebox.ok" {
			return "Aceptar", true
		}
		return "", false
	})
	x := newChromeHarness(t, false, platform.SystemSettings{}, Options{Window: WindowSpec{Title: "App"}, Root: div()})
	if got := x.b.DesktopStrings()["tela.messagebox.ok"]; got != "Aceptar" {
		t.Fatalf("strings at start = %v", x.b.DesktopStrings())
	}
	defer x.h.tree.ReceivePosts()() // what Run does
	defer x.h.WatchStrings()()
	useLabelSource(t, func(id string, _ ...any) (string, bool) {
		if id == "tela.messagebox.ok" {
			return "Accept", true
		}
		return "", false
	})
	LabelsChanged()
	x.b.Wake()
	x.drain()
	if got := x.b.DesktopStrings()["tela.messagebox.ok"]; got != "Accept" {
		t.Errorf("strings after LabelsChanged = %v", x.b.DesktopStrings())
	}
}

// UseLabels subscribes once per mount: a rebuild neither registers its
// listener again nor queues an update that wakes the UI thread, and a
// change still builds the component again.
func TestUseLabelsSubscribesOnce(t *testing.T) {
	var set Setter[int]
	builds := 0
	comp := func() Node {
		n, s := UseState(0)
		set = s
		builds++
		label := UseLabels()
		return Textf("%d %s", n, label("tela.toast.close"))
	}
	tr := devTree(t, Component(comp), nil)
	frameAndFlush(tr)
	labels.mu.Lock()
	seq := labels.seq
	labels.mu.Unlock()
	for i := 1; i <= 3; i++ {
		set.Set(i)
		frameAndFlush(tr)
		if tr.Dirty() {
			t.Fatalf("after rebuild %d the tree has work queued", i)
		}
	}
	labels.mu.Lock()
	again := labels.seq - seq
	labels.mu.Unlock()
	if again != 0 {
		t.Errorf("three rebuilds registered %d more listeners, want none", again)
	}
	before := builds
	LabelsChanged()
	if !tr.Dirty() {
		t.Fatal("LabelsChanged queued no update")
	}
	frameAndFlush(tr)
	if builds != before+1 {
		t.Errorf("LabelsChanged built the component %d times, want once", builds-before)
	}
}

// A change between a component's first build and its mount is caught up:
// the component builds again with the new texts.
func TestUseLabelsCatchesUpBeforeMount(t *testing.T) {
	builds := 0
	comp := func() Node {
		builds++
		label := UseLabels()
		if builds == 1 {
			LabelsChanged() // before the mount effect subscribes
		}
		return Text(label("tela.toast.close"))
	}
	tr := devTree(t, Component(comp), nil)
	frameAndFlush(tr)
	frameAndFlush(tr)
	if builds != 2 {
		t.Errorf("built %d times, want 2 (the change caught up once)", builds)
	}
}
