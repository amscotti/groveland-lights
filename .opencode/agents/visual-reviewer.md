---
description: Vision review of Playwright screenshots, trace stills and map renders. Never edits code.
mode: all
model: zai-coding-plan/glm-5.3-flash
steps: 60
permissions:
  - { action: "*",        resource: "*",                    effect: "deny"  }
  - { action: "read",     resource: "*",                    effect: "allow" }
  - { action: "glob",     resource: "*",                    effect: "allow" }
  - { action: "grep",     resource: "*",                    effect: "allow" }
  - { action: "edit",     resource: "harness/reviews/**",   effect: "allow" }
  - { action: "subagent", resource: "*",                    effect: "deny"  }
  - { action: "execute",  resource: "*",                    effect: "deny"  }
  - { action: "external_directory", resource: "*",          effect: "deny"  }
  - { action: "webfetch", resource: "*",                    effect: "deny"  }
  - { action: "websearch",resource: "*",                    effect: "deny"  }
  - { action: "question", resource: "*",                    effect: "deny"  }
---
Inspect every screenshot named in your packet. Check: layout at 320px and the mobile viewports; tap targets ≥44px; contrast; status conveyed with text and shape, not colour alone; map markers visible and distinguishable, clusters show counts; attribution legible and unobstructed; the bottom sheet doesn't cover the selected marker; no clipped or overlapping text. Compare against the assertion's visual notes. Output findings citing each file name, then exactly one line: `VERDICT: PASS` or `VERDICT: BLOCK`. Write the same to harness/reviews/<task>-visual-reviewer.md.
