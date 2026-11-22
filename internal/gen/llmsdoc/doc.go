// Package llmsdoc holds the tests that keep docs/llms.txt, the short core
// guide for LLMs, and the recipes of docs/llms/ true to the code (ADR-177,
// ADR-313). It has no code of its own: llms_test.go reads the files and
//
//   - type-checks every ```go block of the core and of every recipe against
//     the current module, with the rules of `tela lint` (internal/lint):
//     unknown classes, roles, aria tokens, keys and the rules of hooks fail
//     the test like type errors;
//   - mounts the compact settings screen of llms/settings-screen.md (a whole
//     package main) headlessly in dev mode, as `tela inspect` does, and
//     fails on any dev message;
//   - checks that every struct component of package kit is named in the
//     core and in llms/kit.md, and that the fields listed in the kit
//     catalog exist;
//   - keeps the core within coreMaxBytes and each recipe within its row of
//     recipeMaxBytes, and the core's "## Recipes" index in step with the
//     files of docs/llms/ (one line each).
//
// A block is checked as one of three shapes, chosen by its text:
//
//   - a whole file, when it starts with a package clause; it is compiled
//     as is;
//   - declarations (types, funcs, vars) without a package clause: they are
//     put in a package with the usual imports (tela, a dot import of h, kit,
//     aria, telatest, i18n, lucide and a few of the standard library), each
//     of which the wrapper uses once so none is unused;
//   - otherwise, statements: the body of a component, wrapped in
//     func snippetBody() tela.Node { ... }, with a final return nil when
//     the block does not end with a return.
//
// Whole files with the same package name other than main are one package
// (across the recipes too), so a test block can use what an earlier block
// declares. The import "github.com/kuronosu/tela/icons/lucide" (a separate
// module) is replaced by a subset, examples/settings/lucide (the icons of
// the blocks and of that example), so a block may use the icons of that
// subset only. Code that is wrong on purpose goes in a ```text block, which
// is not checked, or in a ```go mistake block of a mistakes section.
//
// The blocks are written as packages under testdata/snippets-*/ while the
// test runs (`go list` finds packages under testdata when named; the
// directory is removed at the end).
package llmsdoc
