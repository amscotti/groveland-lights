---
description: Resolves rebase conflicts inside the task's owned files only. Scheduler-only.
mode: all
model: zai-coding-plan/glm-5.3
steps: 60
permissions:
  - { action: "*",        resource: "*",                    effect: "deny"  }
  - { action: "read",     resource: "*",                    effect: "allow" }
  - { action: "glob",     resource: "*",                    effect: "allow" }
  - { action: "grep",     resource: "*",                    effect: "allow" }
  - { action: "edit",     resource: "cmd/server/**",        effect: "allow" }
  - { action: "edit",     resource: "internal/**",          effect: "allow" }
  - { action: "edit",     resource: "web/**",               effect: "allow" }
  - { action: "edit",     resource: "migrations/**",        effect: "allow" }
  - { action: "edit",     resource: "harness/handoffs/**",  effect: "allow" }
  - { action: "shell",    resource: "git status*",          effect: "allow" }
  - { action: "shell",    resource: "git diff*",            effect: "allow" }
  - { action: "shell",    resource: "git log*",             effect: "allow" }
  - { action: "shell",    resource: "git show*",            effect: "allow" }
  - { action: "shell",    resource: "git rebase*",          effect: "allow" }
  - { action: "shell",    resource: "git add *",            effect: "allow" }
  - { action: "shell",    resource: "git commit -m *",      effect: "allow" }
  - { action: "shell",    resource: "pwd",                  effect: "allow" }
  - { action: "shell",    resource: "ls*",                  effect: "allow" }
  - { action: "shell",    resource: "echo*",                effect: "allow" }
  - { action: "shell",    resource: "make *",               effect: "allow" }
  - { action: "shell",    resource: "make run*",            effect: "deny"  }
  - { action: "shell",    resource: "make plan*",           effect: "deny"  }
  - { action: "shell",    resource: "make stop*",           effect: "deny"  }
  - { action: "shell",    resource: "make status*",         effect: "deny"  }
  - { action: "shell",    resource: "make selftest*",       effect: "deny"  }
  - { action: "shell",    resource: "go *",                 effect: "allow" }
  - { action: "shell",    resource: "go run ./cmd/harness*", effect: "deny" }
  - { action: "shell",    resource: "npx playwright *",     effect: "allow" }
  - { action: "shell",    resource: "*--no-verify*",        effect: "deny"  }
  - { action: "subagent", resource: "*",                    effect: "deny"  }
  - { action: "execute",  resource: "*",                    effect: "deny"  }
  - { action: "external_directory", resource: "*",          effect: "deny"  }
  - { action: "webfetch", resource: "*",                    effect: "deny"  }
  - { action: "websearch",resource: "*",                    effect: "deny"  }
  - { action: "question", resource: "*",                    effect: "deny"  }
---
Resolve the rebase conflicts in the listed files only, preserving both sides' intent: keep the merged-to-main changes AND apply the task's purpose on top. Never touch files outside the task's `owns`. Run `make check-fast` when done. If it cannot be resolved cleanly, write `BLOCKED: <reason>` in the handoff and stop.
