// Package semantics holds Tela's semantics tree (ARCHITECTURE sections 4,
// 8 and 16): a parallel tree of roles, names, states and actions derived
// from the Element and Box trees, used by accessibility backends, tests
// (telatest's ByRole and ByText) and agents (the aria snapshot of `tela
// inspect --aria` and `tela mcp`).
//
// The package defines the node type and its vocabulary (WAI-ARIA 1.2 roles
// and the role groups of the accessible name computation and of the
// states), and serializes a tree as a Playwright aria snapshot (Snapshot).
// It does not import core: package core builds the tree (core's
// semantics.go) and runs the actions.
//
// # Nodes
//
// A tree mirrors the element tree: the root (KindRoot) is the window's
// content, every primitive element with a computed style is a KindElement
// node, and every non-empty text is a KindText node. Components are
// transparent. Containers without a role (Div, Span) are nodes with the
// implicit role "generic" (ExplicitRole false), so that their states
// (focusable, scrollable), actions and bounds are available; the aria
// snapshot flattens them, as Playwright does. Hidden subtrees (display:
// none, AriaHidden, visibility: hidden) are kept with StateHidden, for
// queries that include hidden elements; the aria snapshot leaves them out.
//
// # Aria snapshots
//
// Snapshot follows Playwright's ariaSnapshot (packages/injected/src/
// ariaSnapshot.ts of Playwright 1.61, default mode): text runs are merged
// across flattened containers with block boundaries as spaces and white
// space collapsed, a single text child equal to the node's name is
// dropped, links carry their destination as a /url property, and keys and
// values are quoted with Playwright's YAML rules. Its output is byte for
// byte what Playwright prints for the equivalent HTML, with these
// deliberate differences:
//
//   - a textbox shows its Value (its text content until Tela has text
//     inputs) where Playwright shows the value only for input and textarea
//     elements;
//   - there are no iframes, slots, shadow roots, pseudo-element content,
//     placeholders, aria-owns or title attributes, and aria-invalid is
//     only true or false (no "grammar" or "spelling"), so [invalid] is
//     the only form printed;
//   - inert subtrees (core's Inert prop, an exiting Presence child, the
//     outside of a modal overlay) are hidden, as in the browser's
//     accessibility tree, where Playwright 1.61, which ignores the inert
//     attribute, would print them; the outside of a modal matches
//     Playwright for aria-hidden="true", which React Aria sets there.
package semantics
