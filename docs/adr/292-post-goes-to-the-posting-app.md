# ADR-292: A post goes to the app that makes it

- **Date:** 2026-11-17
- **Status:** Accepted. Amends ADR-051 (`Post`), ADR-059 (telatest receives posts) and ADR-180 (calls routed like `Post`).

## Decision

While several Trees receive posts (tests that mount apps in parallel), `tela.Post` and the calls routed like it (`SetColorScheme`, `SetTheme`, `Quit`, the default language, and package desktop's lookup of the running app) go to the newest receiver whose UI thread makes the call (the goroutine that called `ReceivePosts`: `Run`'s, or the test's that mounted the app, which runs its handlers, effects and timers), else to the app of the `UseGo` goroutine that makes it, else to the newest receiver, as before. With one receiver nothing changes and a post reads no goroutine id. A host syncs its backend's strings through its own queue.

## Why

The one receiver kept every test whose app switched the scheme, dropped a card, ran a toast callback or opened a native menu or a message box sequential. Made parallel, `examples/dashboard`'s `-short` run alone goes from 16.0 to 10.4 s (12 threads, same CPU time). A goroutine id (a stack header, 15 to 60 µs) is cheap at the rate of posts, `ReceivePosts` and `UseGo` starts.

## What it changes

`internal/core`, the docs of `tela.Post`, `tela.UseGo` and `telatest.Mount`, §7, and the tests made parallel. Posts from other goroutines (a plain `go`, `time.AfterFunc`) still go to the newest receiver: such tests stay sequential.
