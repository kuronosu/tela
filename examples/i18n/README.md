# i18n

An order summary in English, Spanish and Arabic, with a language switcher that changes the language while the app runs and lays the screen out right to left in Arabic.

What it shows:

- **Catalogs.** `locales/en.json`, `es.json` and `ar.json` hold ICU MessageFormat messages, embedded with `//go:embed` and loaded by `i18n.Load` in `init`: arguments (`Hello, {name}!`), plurals (Arabic has six categories: `zero one two few many other`), an English ordinal (`selectordinal`) and nested keys (`summary.discount`).
- **Formatting.** `i18n.UseFormatter` gives the locale's numbers (`1,234.50`, `1.234,50`, `١٬٢٣٤٫٥٠`), percentages, full dates, short times and lists (`apples, pears, and figs`, `manzanas, peras y higos`, `تفاح وكمثرى وتين`).
- **Runtime switching.** The buttons call `i18n.SetLocale`; `Store` and `Switcher` read the locale through hooks, so they build again and nothing else does. Each button is named in its own language and pressed (`AriaPressed`) for the current one.
- **Right to left.** `Store` puts `Dir(f.Dir().String())` on its root, so under Arabic the header, the rows and the footer mirror, the progress bar (`absolute start-0`) fills from the right, the accent border (`border-s-4`) moves to the right edge, the values (`ms-auto text-end`) go to the left, Checkout (`ms-auto`) goes to the left and the back arrow turns with `rtl:rotate-180`. Arabic text uses the embedded Noto Sans Arabic.

## Run it

- In a window: `CGO_ENABLED=0 go run ./examples/i18n` (720x720). It starts in the system's language when it is English, Spanish or Arabic, else in English.
- Without a window: `CGO_ENABLED=0 go run ./cmd/tela snapshot --size 720x720 [--preview ar|en|es] -o i18n.png ./examples/i18n`; the previews fix their locale with `i18n.Provider`.
- Tests: `CGO_ENABLED=0 go test ./examples/i18n`. They check the texts and formats of each language, Arabic's plural categories as the counter changes, and switching from English to Arabic and Spanish with the buttons (the footer and the progress bar mirror and come back), plus the goldens (`testdata/i18n_en_x1.png`, `i18n_es_x1.png`, `i18n_ar_x1.png`, `i18n_ar_x2.png`; regenerate with `TELA_UPDATE=1 CGO_ENABLED=0 go test -run TestGolden ./examples/i18n`).
