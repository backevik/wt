# wt

`wt` creates and cleans up git worktrees. Each worktree gets its own
[Rex](https://www.superlogical.com/rex) session, opened with a layout set per
repo.

```sh
go install github.com/backevik/wt@latest   # or: go install . in this repo
cp extras/config.example.yaml ~/.config/wt/config.yaml
wt doctor
```

Run `wt --help` for the commands. Agents and contributors: see
[AGENTS.md](AGENTS.md).

## Rex shortcuts

Add this line to `~/.config/rex/init.lua`, then run `rex config reload`:

```lua
dofile(os.getenv("HOME") .. "/projects/hobby/wt/extras/rex/wt.lua")
```

| Shortcut | Action |
| --- | --- |
| Ctrl+Opt+Cmd+N | New worktree |
| Ctrl+Opt+Cmd+O | Open worktree |
| Ctrl+Opt+Cmd+Q | Remove worktree |
| Ctrl+Opt+Cmd+L | List worktrees |

All of them, plus "Clean Up Worktrees", are also in the command palette.

## Raycast

In Raycast, open Settings → Extensions → Script Commands → Add Directories,
and add `extras/raycast`.
