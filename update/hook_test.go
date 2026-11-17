package update

import (
	"fmt"
	"testing"
	"time"

	"github.com/kuronosu/tela"
	. "github.com/kuronosu/tela/h"
	"github.com/kuronosu/tela/telatest"
)

func TestUseUpdate(t *testing.T) {
	f := newFixture(t, "1.0.0")
	p := appImage(t, "v1.0")
	f.publish(manifest(release("1.1.0", f.artifact("a", "appimage", []byte("v1.1")))))
	restarted := make(chan string, 1)
	setVar(t, &startProgram, func(exe string) error { restarted <- exe; return nil })
	setVar(t, &exitProcess, func(int) {})

	cfg := f.config()
	cfg.Policy = ActivateByApp
	app := telatest.MountFunc(t, func() tela.Node {
		up := UseUpdate(cfg)
		label := map[Status]string{Checking: "checking", UpToDate: "up to date", Downloading: "downloading", Ready: "ready", Failed: "failed"}[up.Status]
		if up.Update != nil {
			label += " " + up.Update.Version
		}
		if up.Err != nil {
			label += ": " + up.Err.Error()
		}
		return Div(Text(label), Button(OnClick(up.Restart), Text("Restart")))
	}, telatest.Timeout(10*time.Second))
	app.ExpectText("ready 1.1.0")
	if got := readFile(t, p); got != "v1.1" {
		t.Errorf("the install holds %q after Ready", got)
	}
	// Outside Run, Restart starts the program at once.
	app.Click(telatest.ByRole("button", "Restart"))
	select {
	case exe := <-restarted:
		if exe != p {
			t.Errorf("restarted %s, want %s", exe, p)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("Restart did not start the program")
	}

	// The next start is the new version, which UseUpdate marks healthy.
	f.setVersion("1.1.0")
	starts(t, 1)
	app2 := telatest.MountFunc(t, func() tela.Node {
		up := UseUpdate(cfg)
		return Text(fmt.Sprint(up.Status == UpToDate))
	}, telatest.Timeout(10*time.Second))
	app2.ExpectText("true")
	app2.WaitFor("the new version marked healthy", func() bool {
		stateMu.Lock()
		defer stateMu.Unlock()
		in, err := findSelf()
		if err != nil {
			return false
		}
		st, err := in.loadState()
		return err == nil && st.Pending == nil
	})
}
