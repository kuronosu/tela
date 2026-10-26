// Package llmsdoc holds the tests that keep docs/llms.txt true to the code
// (ADR-177). It has no code of its own: llms_test.go reads the file and
//
//   - type-checks every ```go block against the current module, with the
//     rules of `tela lint` (internal/lint): unknown classes, roles, aria
//     tokens, keys and the rules of hooks fail the test like type errors;
//   - checks that the settings screen it shows is examples/llms/settings's
//     main.go, verbatim but for the import of the icons;
//   - checks that every struct component of package kit is named, and that
//     the fields listed in the kit catalog exist.
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
// The import "github.com/kuronosu/tela/icons/lucide" (a separate module) is
// replaced by the example's subset, examples/llms/settings/lucide, so a
// block may use the icons of that subset only. Code that is wrong on
// purpose (a pitfall) goes in a ```text block, which is not checked.
//
// The blocks are written as packages under testdata/snippets-*/ while the
// test runs (`go list` finds packages under testdata when named; the
// directory is removed at the end).
package llmsdoc
