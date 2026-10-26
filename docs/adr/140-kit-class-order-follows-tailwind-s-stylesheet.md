# ADR-140: Kit class order follows Tailwind's stylesheet

- **Date:** 2026-10-25
- **Status:** Accepted

## Decision

Kit class order follows Tailwind's stylesheet (phase 10; amends ADR-130's "classes apply in the order Root, variant, size, state, `Class`"). Every class prop of a kit element goes through `cls`/`joinClass`, which joins the recipe parts and the instance classes and then puts every class that has a variant (a colon outside brackets and parentheses: `hover:`, `focus-visible:`, `dark:`, `aria-invalid:`, `group-aria-[sort=…]:`) after every plain one, keeping the order inside each group. The engine is unchanged: Tela still applies classes in order, last wins, variants included (§10, ADR-103)

## Rationale

Tailwind emits variant utilities after the plain ones, so in CSS a matching variant beats a plain class wherever each is written, and shadcn's recipes are written for that: concatenating the button root's `focus-visible:border-ring` before the outline variant's `border-border` lost the focus border under Tela's order (`TestOutlineButtonFocusBorder`). Two groups give a recipe that precedence, an instance `Class` joined last still wins inside its group, and it matches tailwind-merge as shadcn's `cn` uses it (a plain instance class does not remove a recipe's `hover:` class). Every kit golden and Chromium sheet is unchanged (they show static states); the sticky table cell now restates its background on hover (`group-hover:bg-background`), as it would need in CSS
