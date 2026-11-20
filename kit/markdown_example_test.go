package kit_test

import (
	"fmt"

	"github.com/kuronosu/tela"
	"github.com/kuronosu/tela/kit"
	"github.com/kuronosu/tela/telatest"
)

func ExampleMarkdown() {
	t := &exampleT{}
	defer t.close()
	// A note whose tasks the app keeps in its own source: OnToggleTask asks
	// for a change and the app edits the text; a link goes to OnLink.
	const note = "# Groceries\n\n- [ ] Bread\n- [x] Coffee\n\nSee the [list](https://example.org/list).\n"
	var opened string
	app := telatest.MountFunc(t, func() tela.Node {
		src, setSrc := tela.UseState(note)
		return kit.Markdown{
			Source: src,
			OnLink: func(href string) { opened = href },
			OnToggleTask: func(index int, checked bool) {
				if index == 0 && checked {
					setSrc.Set(note[:len("# Groceries\n\n- [")] + "x" + note[len("# Groceries\n\n- [ "):])
				}
			},
		}
	})
	app.Click(telatest.ByRole("checkbox", "Bread"))
	app.Click(telatest.ByRole("link", "list"))
	fmt.Println(app.Count(telatest.ByRole("heading", "Groceries")), opened)
	app.Expect(telatest.ByRole("checkbox", "Bread"), telatest.Checked(true))
	// Output: 1 https://example.org/list
}
