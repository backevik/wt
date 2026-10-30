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

exec "$HOME/go/bin/wt" open "$1"
