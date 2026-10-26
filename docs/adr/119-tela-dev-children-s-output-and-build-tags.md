# ADR-119: `tela dev` children's output and build tags

- **Date:** 2026-10-22
- **Status:** Accepted

## Decision

`tela dev` children's output and build tags (phase-9 review; amends ADR-115 and ADR-116). **Output.** A child's standard output and error are pipes that the supervisor copies to its own (the terminal), each stream in order (standard error through the tail that finds runtime crashes, as before); a child never gets the terminal's descriptors. Children stay in their own process group (Ctrl+C reaches only `tela dev`) with the null device as standard input, so a background group never touches the terminal. `exec.Cmd.WaitDelay` (1 s) bounds the copy when a process the child left running holds the pipes; that delay alone is not an error. **Tags.** The `-tags` of the dev builds and of their `go list` is `teladev` followed by the tags of the user's GOFLAGS (`-tags=a,b`, `-tags a,b`, or the legacy space-separated list), which a `-tags` on the command line would otherwise replace; GOFLAGS is split as the go command splits it (an entry may be quoted whole) and read once, when the target is resolved

## Rationale

A process in a background group that writes to its controlling terminal is stopped by SIGTTOU when `stty tostop` is set, so a child writing to the terminal could stop before its first frame (a timed-out swap) or freeze; going through the supervisor costs one copy of the app's log output. Unlike `-ldflags` or `-gcflags`, `-tags` cannot be left to GOFLAGS, since the dev runtime needs `teladev`
