# Agent Instructions

`wt` is a Go CLI that manages git worktrees, each tied to a Rex session.

## Commands

```sh
go build ./...
go vet ./...
go test -race ./...
gofmt -l .        # must print nothing
go install .      # installs to $(go env GOPATH)/bin/wt
```

## Layout

| Path | Purpose |
| --- | --- |
| `main.go` | Flag parsing and command dispatch |
| `internal/app` | Commands, stale-state checks (`reconcile`), and `clean`'s rules (`Decide`) |
| `internal/config` | `~/.config/wt/config.yaml`: repos and per-repo layouts |
| `internal/state` | `~/.local/state/wt/state.json`, under a file lock |
| `internal/git`, `internal/github`, `internal/rex` | Thin wrappers around the `git`, `gh` and `rex` CLIs |
| `extras/` | Example config, Rex actions (`rex/wt.lua`), Raycast scripts |

## Rules

- **The state file is the source of truth for which worktrees wt owns.**
  wt only touches worktrees it created. It never scans or adopts others.
- **Every command runs `reconcile` first.** It clears session IDs Rex no
  longer has and records switched branches. When Rex is unreachable
  (`rex.ErrUnreachable`, exit status 4) it must not clear session IDs.
- **`Decide` holds `clean`'s rules** and is a pure function. Change the rules
  there and in `decide_test.go` together.
- **Kill a Rex session last.** wt may run inside the session it removes.
- **Don't hold pointers into `state.Worktrees` across `State.Remove`.**
  `slices.DeleteFunc` zeroes the removed tail.
- **Talk to Rex only through the `rex` CLI** (`rex api call`, `rex ls --json`).
  Check method shapes with `rex api describe METHOD --json`.
- Integration tests in `internal/app/app_test.go` use real git repos and fake
  `rex` and `gh` scripts. Add a case there for any behaviour change.
