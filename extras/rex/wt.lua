-- Rex actions for wt. Copy or symlink this file to ~/.config/rex/wt.lua, then
-- add this line to ~/.config/rex/init.lua and run `rex config reload`:
--   dofile(os.getenv("HOME") .. "/.config/rex/wt.lua")
-- Each action runs wt in a floating pane, so pickers and errors are visible.

-- Shortcuts. Change them, or set one to nil to leave the action unbound.
-- Every action is also in the command palette under "Worktrees".
local keys = {
  wt_new = "ctrl+alt+cmd+n",
  wt_open = "ctrl+alt+cmd+o",
  wt_rm = "ctrl+alt+cmd+q",
  wt_ls = "ctrl+alt+cmd+l",
  wt_clean = nil,
}

-- The pane's shell reads only login startup files, so wt may not be on its
-- PATH. Fall back to where go install puts it. Set WT_BIN for the server to
-- use another binary.
local find_wt = os.getenv("WT_BIN") and "wt='" .. os.getenv("WT_BIN"):gsub("'", "'\\''") .. "'"
  or 'wt=$(command -v wt || echo "${GOBIN:-${GOPATH:-$HOME/go}/bin}/wt")'

local function popup(args, keep)
  local cmd = find_wt .. '; "$wt" ' .. args
  local script
  if keep then
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
  { name = "wt_new", title = "New Worktree", args = "new" },
  { name = "wt_open", title = "Open Worktree…", args = "open" },
  { name = "wt_rm", title = "Remove Worktree…", args = "rm" },
  { name = "wt_ls", title = "List Worktrees", args = "ls", keep = true },
  { name = "wt_clean", title = "Clean Up Worktrees", args = "clean", keep = true },
}

for _, a in ipairs(actions) do
  rex.action{
    name = a.name,
    title = a.title,
    category = "Worktrees",
    keywords = "wt worktree git",
    run = function() popup(a.args, a.keep) end,
  }
  if keys[a.name] then rex.bind(keys[a.name], a.name) end
end
