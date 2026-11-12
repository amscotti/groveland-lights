#!/usr/bin/env bash
# worktree-rm.sh T-### — remove the scheduler worktree and its branch.
set -euo pipefail

TASK="${1:?usage: worktree-rm.sh T-###}"
ROOT="$(git rev-parse --show-toplevel)"
WT="$ROOT/../wt/$TASK"

if [ -e "$WT" ]; then
  # node_modules is a symlink into the main repo; drop it before pruning.
  [ -L "$WT/node_modules" ] && rm "$WT/node_modules"
  git -C "$ROOT" worktree remove --force "$WT" 2>/dev/null || rm -rf "$WT"
fi
git -C "$ROOT" branch -D "task/$TASK" 2>/dev/null || true
echo "removed $WT"
