# signup

A "Create your account" screen in shadcn style: a gradient brand panel on the left (hidden below `md:`) and a form on the right with a display name, an e-mail, a password, a bio and a terms checkbox. It is the example to imitate for forms.

What it shows:

- **Controlled fields with `Bind`.** Name, e-mail and password are `h.Bind(value, setter)` (= `Value` + `OnChange`), because the validation and the password strength meter read them. `Values.Valid()` decides whether the submit button is `Disabled`.
- **`BindValue` for the bio, and why.** The bio is a `tela.UseValue`. Typing in it writes the Value without rebuilding the page, and its counter is an `h.TextFn` that reads the same Value, so a keystroke repaints one paragraph and builds nothing (`TestBio` checks `Stats().Builds` does not move). `OnSubmit` reads it with `bio.Latest()`.
- **Invalid states.** The e-mail is `Type("email")`: its border uses the `invalid:` variant and its message uses `peer-invalid:` (the Input has the `peer` class and the message follows it), so Tela's own HTML-syntax check drives both. They wait for the first blur (`OnBlur`) so nobody is scolded for the first letter. `ValidEmail` uses the same syntax, so the button never disagrees with the border.
- **Password toggle.** A button over the end of the field with `AriaPressed`; `Type` flips between `"password"` and `"text"` and the field keeps its text. A 4-bar strength meter reads the same state.
- **A growing Textarea.** `field-sizing-content min-h-[4.5rem] max-h-40`: 72 px when empty, taller with every line up to 160 px, then it scrolls. `MaxLength(160)` counts UTF-16 code units as HTML does (an emoji is 2).
- **Labels.** `Label(For("email"))` + `Input(Id("email"))`: clicking the label focuses the field, and the label is the field's accessible name (`textbox "E-mail"`). A Button does not take the name of its Label yet, so the checkbox (a `Role("checkbox")` button with `AriaChecked`) also has an `AriaLabel`.
- **Focus and placeholder styling.** `focus:border-* focus:ring-[3px] focus:ring-*/25`, `placeholder:text-*`, `selection:bg-*`, and `disabled:` on the button. Text fields match `focus-visible:` on every focus; buttons only on keyboard focus.
- **Forms.** `Form(OnSubmit(...))`: Enter in a field clicks the first submit button (and does nothing while it is disabled, as in a browser), the button submits too. A confirmation banner (`Role("status")`) appears, and a form-level `OnChange` (it bubbles) removes it at the next edit.
- **Any language.** The name field is plain text input; with an input method (IME) it composes Japanese, Chinese or Korean like any browser field. The goldens use Latin text only: `telatest` and `tela snapshot` shape with the embedded fonts, which have no CJK glyphs (ADR-031); a real window uses the system's fonts.
- **Dark mode.** Every color comes in a pair; render it with `--dark`.

## Run it

- In a window: `CGO_ENABLED=0 go run ./examples/signup` (1100x860).
- Without a window, one image per state: `CGO_ENABLED=0 go run ./cmd/tela snapshot --size 1100x860 --scale 2 [--dark] --preview "3 Invalid" -o signup.png ./examples/signup`. The previews are `1 Empty`, `2 Filled`, `3 Invalid` and `4 Created` (`Previews` in `main.go`; each is `Page` with `Props` that start it in that state).
- Its aria snapshot: `CGO_ENABLED=0 go run ./cmd/tela inspect --size 1100x860 ./examples/signup` (`testdata/signup.aria.yml`).
- Tests: `CGO_ENABLED=0 go test ./examples/signup`. They fill the form with `Fill`, type with `Type` and Tab, check `ExpectValue`, the `invalid:` border by pixel, submission by button and by Enter, the password toggle, the bio's growth, `MaxLength` and builds, the labels, the keyboard on the checkbox, and the goldens of the four previews at scale 1 and 2 plus a dark one (`testdata/signup_*.png`; regenerate with `TELA_UPDATE=1 CGO_ENABLED=0 go test -run 'TestGolden' ./examples/signup`).

Below 768 px the brand panel hides and the form takes the whole window; a window shorter than the form scrolls like a page.
