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

# Raycast runs with a minimal PATH, so fall back to where go install puts wt.
wt=$(command -v wt || echo "${GOBIN:-${GOPATH:-$HOME/go}/bin}/wt")
exec "$wt" new ${1:+"$1"}
