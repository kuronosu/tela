package kit_test

// mdDocument is a representative document: every construct the renderer
// draws, in the order a README would use them.
const mdDocument = "# Tela notes\n" +
	"\n" +
	"A **pure-Go** desktop UI framework with *React-like* hooks, ~~no cgo~~ and `Tailwind` classes.\n" +
	"Read the [docs](https://tela.dev/docs) or visit https://tela.dev.\n" +
	"\n" +
	"## Getting started\n" +
	"\n" +
	"Install the tool, then run it:\n" +
	"\n" +
	"```go\n" +
	"func main() {\n" +
	"\ttela.Main(tela.App{Title: \"Hello\", Root: Hello{}})\n" +
	"}\n" +
	"```\n" +
	"\n" +
	"> Tela fetches nothing over the network.\n" +
	"> Images come from the app's own assets.\n" +
	"\n" +
	"![A gradient from the app's assets](img/logo.png)\n" +
	"\n" +
	"### Checklist\n" +
	"\n" +
	"- [x] Parse the source\n" +
	"- [x] Render the blocks\n" +
	"- [ ] Write the tests\n" +
	"\n" +
	"### Lists\n" +
	"\n" +
	"1. First\n" +
	"2. Second\n" +
	"   - nested bullet\n" +
	"   - another one\n" +
	"     - deeper\n" +
	"3. Third\n" +
	"\n" +
	"### A table\n" +
	"\n" +
	"| Name | Kind | Size |\n" +
	"| :--- | :---: | ---: |\n" +
	"| Button | component | 12 |\n" +
	"| Table | component | 340 |\n" +
	"| `Markdown` | component | 25 |\n" +
	"\n" +
	"---\n" +
	"\n" +
	"The end.\n"
