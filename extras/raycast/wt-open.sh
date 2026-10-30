#!/bin/bash

# Required parameters:
# @raycast.schemaVersion 1
# @raycast.title Open Worktree
# @raycast.mode compact
# @raycast.packageName wt

# Optional parameters:
# @raycast.icon 🌳
# @raycast.argument1 { "type": "text", "placeholder": "worktree name" }
# @raycast.description Switch to a worktree's Rex session, recreating it if it was closed.

# Raycast runs with a minimal PATH, so fall back to where go install puts wt.
wt=$(command -v wt || echo "${GOBIN:-${GOPATH:-$HOME/go}/bin}/wt")
exec "$wt" open "$1"
