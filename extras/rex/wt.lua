-- Rex actions for wt. Load from ~/.config/rex/init.lua with:
--   dofile(os.getenv("HOME") .. "/projects/hobby/wt/extras/rex/wt.lua")
-- Each action runs wt in a floating pane, so pickers and errors are visible.

local wt = os.getenv("HOME") .. "/go/bin/wt"

local function popup(args, opts)
  local cmd = "'" .. wt .. "' " .. args
  local script
  if opts.keep then
    script = cmd .. "; echo; printf 'Press Enter to close'; read _"
  else
    script = cmd .. " || { echo; printf 'Failed. Press Enter to close'; read _; }"
  end
  local _, err = rex.session.new_layer{
    bounds = { 0.15, 0.1, 0.7, 0.6 },
    focus = true,
    layout = rex.layout.block{
      flavor = "com.superlogical.terminal.shell",
      label = "wt",
      options = { command = { "sh", "-c", script } },
    },
  }
  if err ~= nil then error(err, 0) end
end

local actions = {
  { name = "wt_new", title = "New Worktree", args = "new", key = "ctrl+alt+cmd+n" },
  { name = "wt_open", title = "Open Worktree…", args = "open", key = "ctrl+alt+cmd+o" },
  { name = "wt_rm", title = "Remove Worktree…", args = "rm", key = "ctrl+alt+cmd+q" },
  { name = "wt_ls", title = "List Worktrees", args = "ls", keep = true, key = "ctrl+alt+cmd+l" },
  { name = "wt_clean", title = "Clean Up Worktrees", args = "clean", keep = true },
}

for _, a in ipairs(actions) do
  rex.action{
    name = a.name,
    title = a.title,
    category = "Worktrees",
    keywords = "wt worktree git",
    run = function() popup(a.args, a) end,
  }
  if a.key then rex.bind(a.key, a.name) end
end
