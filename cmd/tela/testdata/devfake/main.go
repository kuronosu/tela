// Command devfake is a test app of tela dev that plays the part of the
// teladev child runtime (task T3) without Tela: it dials the remote backend
// itself, keeps a counter as its state (state.save and state.load), counts
// clicks, and draws a solid color from them. With DEVFAKE_EXIT_AFTER set
// (a duration) it exits with status 0 on its own after that long, like an
// app whose window was closed.
package main

import (
	"context"
	"fmt"
	"image"
	"os"
	"strconv"
	"time"

	"github.com/kuronosu/tela/internal/devloop"
	"github.com/kuronosu/tela/internal/platform"
	"github.com/kuronosu/tela/internal/platform/remote"
)

// hue is changed by hand during a manual run to see a reload.
const hue = 0x40

func main() {
	b, err := remote.DialEnv()
	if err != nil {
		fmt.Fprintln(os.Stderr, "devfake:", err)
		os.Exit(4)
	}
	count := 0
	if os.Getenv(devloop.EnvNoState) != "1" {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		if data, err := b.Request(ctx, devloop.KindStateLoad, nil); err == nil && len(data) > 0 {
			count, _ = strconv.Atoi(string(data))
			b.Notify(devloop.KindStateReport, devloop.StateReport{Kept: 1}.Encode())
		}
		cancel()
	}
	b.Handle(devloop.KindStateSave, func([]byte) ([]byte, error) { return []byte(strconv.Itoa(count)), nil })
	b.Handle(devloop.KindQuit, func([]byte) ([]byte, error) { b.Quit(); return nil, nil })
	w, err := b.NewWindow(platform.WindowOptions{Title: "devfake", Size: platform.Size{W: 320, H: 200}})
	if err != nil {
		fmt.Fprintln(os.Stderr, "devfake:", err)
		os.Exit(4)
	}
	if d, err := time.ParseDuration(os.Getenv("DEVFAKE_EXIT_AFTER")); err == nil {
		time.AfterFunc(d, func() { os.Exit(0) })
	}
	fmt.Println("devfake: running with count", count)
	dirty := true
	var fb *image.RGBA
	err = b.Run(platform.HandlerFunc(func(_ platform.Window, e platform.Event) {
		switch e := e.(type) {
		case platform.ConfigureEvent:
			dirty = true
		case platform.PointerEvent:
			if e.Action == platform.PointerDown {
				count++
				dirty = true
				w.RequestFrame()
			}
		case platform.CloseRequestedEvent:
			w.Close()
			b.Quit()
		case platform.FrameEvent:
			if !dirty {
				return
			}
			dirty = false
			p := w.Metrics().Physical()
			if fb == nil || fb.Rect.Max != p {
				fb = image.NewRGBA(image.Rectangle{Max: p})
			}
			for i := 0; i < len(fb.Pix); i += 4 {
				fb.Pix[i], fb.Pix[i+1], fb.Pix[i+2], fb.Pix[i+3] = hue, uint8(count*16), 0xc0, 0xff
			}
			w.Present(fb, nil)
		}
	}))
	if err != nil {
		os.Exit(1)
	}
}
