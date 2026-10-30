#!/bin/bash

# Required parameters:
# @raycast.schemaVersion 1
# @raycast.title New Worktree
# @raycast.mode compact
# @raycast.packageName wt

# Optional parameters:
# @raycast.icon 🌳
# @raycast.argument1 { "type": "text", "placeholder": "branch (random if empty)", "optional": true }
# @raycast.description Create a git worktree with its own Rex session.

exec "$HOME/go/bin/wt" new ${1:+"$1"}
