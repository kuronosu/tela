# Recipe: tela dev, run and debug

Questions: how to see the app in a real window that reloads on save; what state survives a reload; how to debug with Delve; how to attach DevTools or an agent to a running session; `tela run` and `tela build`.

**`tela dev [pkg]`** runs the package's app in a real window that never closes. It rebuilds in the background when a non-test `.go` file of the module, `go.mod`, an embedded file or a `tela.Assets(os.DirFS(...))` file changes, and swaps the running process for the new build under that window, keeping the state of its components (hooks, `aria` and `kit` hooks included) so the screen stays where you left it. Go cannot replace code in a process, so the app runs as a child of `tela dev`, which owns the window; every build is a new process. A build error is an overlay over the last good frame, and the next good build takes over. Dev builds use `-tags teladev` with `CGO_ENABLED=0` and skip what slows a build (VCS stamp, symbol table, DWARF, inlining).

```sh
tela dev ./myapp                      # a package main as it is; any other package runs its Preview / Previews / App.Root as tela snapshot renders it
tela dev --preview "2 Invalid" .      # one named screen of Previews
tela dev --dark --size 1040x800 .     # the app sees dark mode (App.ColorScheme still wins)
tela dev --no-state .                 # fresh state at every reload; R in the terminal does it once, r reloads, q quits
tela dev --debug .                    # builds for a debugger, see below
tela dev --devtools .                 # also open the DevTools window; tela devtools attaches to a session later
tela dev -v .                         # time of every step, and every hook slot a reload could not keep, with the reason
```

- **State.** What lives in hooks (`UseState`, reducers, refs, `UseValue`, the uncontrolled state of `kit` controls), text fields, scroll offsets and focus survives a reload; state outside the component tree (package variables, controllers, a connection's settings) does not unless it is declared in `App.DevState` (ignored outside `tela dev`). A hook slot that cannot be kept (a type that no longer matches) starts afresh, and `-v` lists it with the reason. A renamed or moved component keeps its state when its declaration and place in the tree match.
- **Desktop services across reloads (ADR-310).** An open file dialog or message box, a registered global shortcut, the result of a drop and a window's backdrop carry over to the new build.
- **`--debug` (ADR-310).** Builds keep DWARF and the symbol table and use `-gcflags=all=-N -l`, so a debugger can attach (the first build takes longer; a `GOFLAGS` of your own with those flags wins and `tela dev` warns). Each reload is a new process, so every start prints its process ID and `dlv attach <pid>`; attach again after each reload. On Linux, attaching to a process that is not the debugger's child needs ptrace permission (`kernel.yama.ptrace_scope` 0, or `sudo dlv attach`). While Delve holds the app at a breakpoint the window does not update, and a reload then starts without the state. Without the flag nothing changes.
- **DevTools and agents.** While it runs, `tela dev` serves a tools socket (the address is in `<user cache>/tela/dev/<pid>.json`; `-v` prints it). `tela devtools` opens a window with the element tree and its pick tool, props, hooks, why a component rendered, computed style, the accessibility tree, a frame profiler and the log. `tela mcp --attach` serves the same tools as `tela mcp` over the REAL windows of that session (add the optional `window` argument; `resize`, `open` and `prefer` are not available, `film` and `test` work, `wait` waits real time), so an agent can click the window a person is looking at.
- **On Windows**, Defender scans each new build on its first run and can slow every reload; `tela dev` prints once the directories to exclude and the PowerShell command to run as administrator. It never changes the settings itself.

**Without a window or a watcher.** `tela run ./myapp [args]` builds and runs the program with the terminal attached and the exit status forwarded (on macOS as a development `.app` in the user cache, so the Dock and notifications work). `tela build -o dist ./myapp` builds the program for the environment's `GOOS`/`GOARCH`; `GOOS=darwin` writes `dist/<Name>.app`, `GOOS=windows` a GUI program with icon and version info (`-console` keeps a console). `tela package` makes the packages for every target. All builds are `CGO_ENABLED=0`.

**For an agent**, the loop is not `tela dev` (it needs a display and a person) but `tela mcp`, `tela snapshot` and `tela inspect`, which render headlessly and deterministically, then `tela check` (recipe accessibility) and the tests (recipe testing). Use `tela dev` when a person wants to see and touch the app, or to look at what a headless render cannot: the OS's own windows, dialogs, menus and tray.
