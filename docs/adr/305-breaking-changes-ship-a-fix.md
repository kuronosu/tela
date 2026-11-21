# ADR-305: Every breaking change ships a `tela lint -fix` rule

- **Date:** 2026-11-20
- **Status:** Accepted. Amends ADR-173, ADR-208.

## Decision

A change that breaks the public API (what ADR-208 lists under **Breaking**) ships, in the same change, a rewrite that `tela lint -fix` applies to the old spelling. The rewrites are the entries of `Migrations` (`internal/lint/migrations.go`), each with an `ID`, the release it broke (`Since`), what it does and an `Example` of old code and the new code it becomes. The kinds are table-driven: `Member` (a renamed method or field of a Tela type), `Ident` (a renamed function, type or constant of a Tela package, dot imports too), `Import` (a moved package, with the new name of its uses) and `Custom` (a function over the AST, such as `desktop.Launch.Forwarded` becoming `Source == desktop.LaunchForwarded`). They are reported by the rule `migrate` (`//tela:ignore migrate`), always with a fix; a rewrite may add the import it needs.

Old code does not type-check, so a package that fails to type-check is checked for `migrate` alone, from what go/types resolved (the type of a selector's operand, the package an identifier names); a migration reports nothing for a name the new API still has. After `-fix` rewrites such a package, the run goes again, so the other rules, and their fixes, see it compile.

**Enforcement.** Each `Breaking` entry of `CHANGELOG.md`'s Unreleased section ends with `migrate: <ID>` (several: comma separated), or `migrate: none -- <why>` when nothing mechanical exists (a changed default, a behavior). `internal/lint`'s tests fail on an entry without one, on an unknown ID, and on `none` without a reason. Every entry of the table is tested: its example is rewritten by `-fix` into the new code, which compiles and lints clean. An agent adds the entry in the commit that breaks the API; a break whose rewrite cannot be written says why in the changelog.

## Why

Breaks are released as patches while Tela has no outside users (ADR-208), and the people who do use it, mostly through agents, are left to find each old name by compile errors. A rewrite costs less than the explanation, and the changelog marker keeps the convention from depending on memory.

## What it changes

`internal/lint` (rule, table, `Options.Migrations`, edits that replace text and add imports), `tela lint`'s usage, a comment in `CHANGELOG.md`'s Unreleased section, and one line in the Rules of `AGENTS.md`. Back-filled since v0.14.0: `desktop-launch-forwarded` and `telatest-app-move`; the other breaks (a gradient layering, `ByClass` counts, `i18n` rounding) are behavior.
