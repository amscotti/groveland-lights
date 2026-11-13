---
description: UI-focused worker for templates, CSS, Alpine and map code. Looks at its own screenshots. Scheduler only.
mode: all
model: zai-coding-plan/glm-5.3-flash
steps: 200
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
  - { action: "shell",    resource: "pwd",                   effect: "allow" }
  - { action: "shell",    resource: "ls*",                   effect: "allow" }
  - { action: "shell",    resource: "echo*",                 effect: "allow" }
  - { action: "shell",    resource: "mkdir*",                effect: "allow" }
  - { action: "shell",    resource: "make *",                effect: "allow" }
  - { action: "shell",    resource: "make run*",             effect: "deny"  }
  - { action: "shell",    resource: "make plan*",            effect: "deny"  }
  - { action: "shell",    resource: "make stop*",            effect: "deny"  }
  - { action: "shell",    resource: "make status*",          effect: "deny"  }
  - { action: "shell",    resource: "make selftest*",        effect: "deny"  }
  - { action: "shell",    resource: "go *",                  effect: "allow" }
  - { action: "shell",    resource: "go run ./cmd/harness*", effect: "deny"  }
  - { action: "shell",    resource: "gofmt *",               effect: "allow" }
  - { action: "shell",    resource: "npx playwright *",      effect: "allow" }
  - { action: "shell",    resource: "npx tsc *",             effect: "allow" }
  - { action: "shell",    resource: "npx html-validate *",   effect: "allow" }
  - { action: "shell",    resource: "git status*",          effect: "allow" }
  - { action: "shell",    resource: "git diff*",            effect: "allow" }
  - { action: "shell",    resource: "git log*",             effect: "allow" }
  - { action: "shell",    resource: "git add *",            effect: "allow" }
  - { action: "shell",    resource: "git commit -m *",      effect: "allow" }
  - { action: "shell",    resource: "*--no-verify*",        effect: "deny"  }
  - { action: "edit",     resource: "acceptance/**",        effect: "deny"  }
  - { action: "edit",     resource: "**/*.golden*",         effect: "deny"  }
  - { action: "subagent", resource: "*",                    effect: "deny"  }
  - { action: "execute",  resource: "*",                    effect: "deny"  }
  - { action: "external_directory", resource: "*",          effect: "deny"  }
  - { action: "webfetch", resource: "*",                    effect: "deny"  }
  - { action: "websearch",resource: "*",                    effect: "deny"  }
  - { action: "question", resource: "*",                    effect: "deny"  }
---
Read AGENTS.md and the attached packet. Run the claimed acceptance specs first and confirm they fail. Implement only within `owns`, with unit tests. Run `make check-fast` and `make e2e-task T=<id>` until green. Then open your Playwright screenshots under `test-results/` and compare them against the packet's visual notes: layout at 320px, tap targets ≥44px, contrast, status text labels (never colour-only), attribution visible, nothing clipped. Fix and re-run until they match. Write the handoff at `harness/handoffs/T-###.md`. If blocked or the spec seems wrong, write `BLOCKED: <reason>` and stop — never edit the spec.
