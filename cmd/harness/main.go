// Command harness is the deterministic orchestration layer for the Groveland Lights
// repository: scheduler, guard, merge gate, status, agent verification and self-test.
//
// It is protected path (policy.json "protected": "cmd/harness/**"): only bootstrap and
// the plan's fix tasks may change it. It never asks an LLM to make a policy decision.
package main

import (
	"fmt"
	"os"
)

const usage = `usage: harness <subcommand> [args]

subcommands:
  sched [--dry-run]        run the task scheduler loop (reads harness/STOP to halt)
  guard T-###              enforce post-run checks on a task worktree (exit 0 = clean)
  gate T-###               serial merge gate: rebase, guard, check, holdout, ff-merge, push
  status                   print task states and recent progress events
  verify-agents            verify .opencode/agents against harness/policy.json
  selftest [--live]        deterministic harness self-test (fake workers, no LLM);
                           --live additionally runs T-000 through real agents
  protect                  regenerate harness/protected.sha256
`

func main() {
	if len(os.Args) < 2 {
		fmt.Fprint(os.Stderr, usage)
		os.Exit(2)
	}
	var err error
	switch os.Args[1] {
	case "sched":
		err = cmdSched(os.Args[2:])
	case "guard":
		err = cmdGuard(os.Args[2:])
	case "gate":
		err = cmdGate(os.Args[2:])
	case "status":
		err = cmdStatus(os.Args[2:])
	case "verify-agents":
		err = cmdVerifyAgents(os.Args[2:])
	case "selftest":
		err = cmdSelftest(os.Args[2:])
	case "protect":
		err = cmdProtect(os.Args[2:])
	default:
		fmt.Fprint(os.Stderr, usage)
		os.Exit(2)
	}
	if err != nil {
		fmt.Fprintf(os.Stderr, "harness: %v\n", err)
		os.Exit(1)
	}
}
