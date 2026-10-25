package aria_test

import (
	"fmt"

	"github.com/kuronosu/tela"
	"github.com/kuronosu/tela/aria"
	. "github.com/kuronosu/tela/h"
	"github.com/kuronosu/tela/telatest"
)

func ExampleUseRadioGroup() {
	t := &exampleT{}
	defer t.close()
	plans := []string{"free", "pro", "team"}
	page := func() tela.Node {
		g := aria.UseRadioGroup(aria.RadioGroupOptions[string]{Keys: plans, DefaultValue: new("free"), AriaLabel: "Plan"})
		return Div(g.Props, Class("flex flex-col gap-1"),
			Range(len(plans), func(i int) tela.Node {
				k := plans[i]
				return Div(Key(k), Class("flex items-center gap-2"),
					Button(g.RadioProps(k), Class("size-4 rounded-full border checked:bg-primary")),
					Label(For(g.RadioID(k)), Text(k)))
			}))
	}
	app := telatest.MountFunc(t, page)
	app.Focus(telatest.ByRole("radio", "free"))
	app.Press("ArrowDown") // moves the focus and checks
	fmt.Println(app.AriaSnapshot())
	// Output:
	// - radiogroup "Plan":
	//   - radio "free"
	//   - text: free
	//   - radio "pro" [checked]
	//   - text: pro
	//   - radio "team"
	//   - text: team
}

func ExampleUseToggleGroup() {
	t := &exampleT{}
	defer t.close()
	modes := []string{"list", "grid"}
	page := func() tela.Node {
		g := aria.UseToggleGroup(aria.ToggleGroupOptions[string]{
			Keys: modes, DefaultSelected: aria.SelectionOf("list"),
			DisallowEmptySelection: true, AriaLabel: "View",
		})
		return Div(g.Props, Class("inline-flex rounded-md border"),
			Range(len(modes), func(i int) tela.Node {
				return Button(g.ItemProps(modes[i]), Key(modes[i]), Class("px-3 py-1 aria-checked:bg-accent"), Text(modes[i]))
			}))
	}
	app := telatest.MountFunc(t, page)
	app.Click(telatest.ByRole("radio", "grid"))
	fmt.Println(app.AriaSnapshot())
	// Output:
	// - radiogroup "View":
	//   - radio "list"
	//   - radio "grid" [checked]
}

func ExampleUseSlider() {
	t := &exampleT{}
	defer t.close()
	page := func() tela.Node {
		s := aria.UseSlider(aria.SliderOptions{DefaultValues: []float64{40}, AriaLabel: "Volume"})
		track := tela.UseSize(s.TrackRef)
		return Div(s.Props, Class("w-64 p-4"),
			Div(s.TrackProps, Class("relative h-2 w-full rounded-full bg-muted"),
				Div(s.ThumbProps(0), Class("absolute -top-1.5 -ml-2.5 size-5 rounded-full bg-primary"),
					Style(tela.TranslateFn(func() (x, y float32) {
						return float32(s.ThumbPercent(0)) / 100 * track.Get().Width, 0
					})))))
	}
	app := telatest.MountFunc(t, page)
	vol := telatest.ByRole("slider", "Volume")
	app.Focus(vol)
	app.Press("PageUp") // a tenth of the range
	app.Press("ArrowLeft")
	fmt.Println(app.AriaSnapshot())
	app.Expect(vol, telatest.HasValue("49"))
	// Output:
	// - group "Volume":
	//   - slider "Volume"
}

func ExampleUseProgressBar() {
	t := &exampleT{}
	defer t.close()
	page := func() tela.Node {
		p := aria.UseProgressBar(aria.ProgressBarOptions{Value: 40, AriaLabel: "Upload"})
		return Div(p.Props, Class("h-2 w-64 overflow-hidden rounded-full bg-muted"),
			Div(Class("h-full bg-primary"), Class(fmt.Sprintf("w-[%g%%]", p.Percent))))
	}
	app := telatest.MountFunc(t, page)
	fmt.Println(app.AriaSnapshot())
	// Output: - progressbar "Upload"
}

func ExampleUseMeter() {
	t := &exampleT{}
	defer t.close()
	page := func() tela.Node {
		m := aria.UseMeter(aria.MeterOptions{Value: 128, Max: 512, AriaLabel: "Disk usage"})
		return Div(m.Props, Class("h-2 w-64 rounded-full bg-muted"),
			Div(Class("h-full bg-primary"), Class(fmt.Sprintf("w-[%g%%]", m.Percent))))
	}
	app := telatest.MountFunc(t, page)
	fmt.Println(app.AriaSnapshot())
	// Output: - meter "Disk usage"
}
