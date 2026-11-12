#!/usr/bin/env bash
# worktree-new.sh T-### — create the scheduler worktree for a task.
# Idempotent, shellcheck-clean. Exits non-zero with a message on failure.
set -euo pipefail

TASK="${1:?usage: worktree-new.sh T-###}"
ROOT="$(git rev-parse --show-toplevel)"
WT="$ROOT/../wt/$TASK"

if [ -e "$WT" ]; then
  echo "worktree $WT already exists" >&2
  exit 0
fi

git -C "$ROOT" worktree add "$WT" -b "task/$TASK" main

# Test env, if present.
if [ -f "$ROOT/.env.test" ]; then
  cp "$ROOT/.env.test" "$WT/.env.test"
fi

# Pinned node_modules so npx/Playwright never fetch registry latest.
if [ -d "$ROOT/node_modules" ] && [ ! -e "$WT/node_modules" ]; then
  ln -s "$ROOT/node_modules" "$WT/node_modules"
fi

echo "created $WT (branch task/$TASK)"
