# wt

`wt` creates and cleans up git worktrees. Each worktree gets its own
[Rex](https://www.superlogical.com/rex) session, opened with a layout set per
repo.

Needs `git`, Rex, and optionally `gh` (PR status) and `fzf` (picking a
worktree).

```sh
go install github.com/backevik/wt@latest
mkdir -p ~/.config/wt
curl -fsSL https://raw.githubusercontent.com/backevik/wt/main/extras/config.example.yaml > ~/.config/wt/config.yaml
wt doctor
```

`go install` puts `wt` in `$(go env GOPATH)/bin`, usually `~/go/bin`. Make
sure that's on your `PATH`.

Run `wt --help` for the commands. Agents and contributors: see
[AGENTS.md](AGENTS.md).

## Rex shortcuts

Save [`extras/rex/wt.lua`](extras/rex/wt.lua) as `~/.config/rex/wt.lua`, add
this line to `~/.config/rex/init.lua`, then run `rex config reload`:

```lua
dofile(os.getenv("HOME") .. "/.config/rex/wt.lua")
```

| Shortcut | Action |
| --- | --- |
| Ctrl+Opt+Cmd+N | New worktree |
| Ctrl+Opt+Cmd+O | Open worktree |
| Ctrl+Opt+Cmd+Q | Remove worktree |
| Ctrl+Opt+Cmd+L | List worktrees |

Change them in the `keys` table at the top of the file. All the actions,
plus "Clean Up Worktrees", are also in the command palette.

## Raycast

In Raycast, open Settings → Extensions → Script Commands → Add Directories,
and add a folder with the scripts from [`extras/raycast`](extras/raycast).
