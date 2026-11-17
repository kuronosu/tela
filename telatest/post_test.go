package telatest_test

import (
	"context"
	"fmt"
	"testing"

	"github.com/kuronosu/tela"
	"github.com/kuronosu/tela/desktop"
	. "github.com/kuronosu/tela/h"
	"github.com/kuronosu/tela/telatest"
)

// TestPostsReachTheirAppInParallel: apps of tests running in parallel each
// get the posts, the scheme switches and the desktop requests that their
// own handlers and UseGo goroutines make (ADR-292). The counters are plain
// variables: a post run by another test's app would race (-race) or be
// missing when the test checks.
func TestPostsReachTheirAppInParallel(t *testing.T) {
	for i := range 6 {
		t.Run(fmt.Sprint(i), func(t *testing.T) {
			t.Parallel()
			title := fmt.Sprint("app ", i)
			posts, fromGo := 0, false
			app := telatest.MountFunc(t, func() tela.Node {
				done, setDone := tela.UseState(false)
				tela.UseGo(func(ctx context.Context) {
					tela.Post(func() { fromGo = true; setDone.Set(true) })
				}, tela.Once)
				return Div(
					Button(OnClick(func() {
						tela.Post(func() { posts++ })
						if i%2 == 1 {
							tela.SetColorScheme(tela.Dark)
						}
					}), Text("Go")),
					Button(OnClick(func() {
						if err := desktop.ShowMessage(desktop.Message{Title: title, Text: "?"}, func(desktop.Answer) {}); err != nil {
							t.Errorf("ShowMessage: %v", err)
						}
					}), Text("Ask")),
					If(done, Text("posted from its goroutine")),
					If(tela.UseColorScheme() == tela.Dark, Text("dark")),
				)
			}, telatest.Size(200, 100))
			for range 20 {
				app.Click(telatest.ByRole("button", "Go"))
			}
			app.ExpectText("posted from its goroutine")
			if posts != 20 || !fromGo {
				t.Errorf("the app ran %d of its 20 posts, and its goroutine's: %v", posts, fromGo)
			}
			if dark := app.Count(telatest.ByText("dark")) == 1; dark != (i%2 == 1) {
				t.Errorf("dark %v, want it only in the apps that switched", dark)
			}
			app.Click(telatest.ByRole("button", "Ask"))
			if boxes := app.MessageBoxes(); len(boxes) != 1 || boxes[0].Title != title {
				t.Fatalf("message boxes %+v, want the one this app asked for", boxes)
			}
			app.AnswerMessage(desktop.AnswerOK)
		})
	}
}
