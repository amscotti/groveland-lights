# Groveland Lights — single entry point for humans, agents and CI (plan §6.1.8).
# CI runs the same targets; never a parallel definition.

SHELL := /bin/bash
.SHELLFLAGS := -eu -o pipefail -c

MILESTONE ?=
TASK ?=
BASE_URL ?=
TS := $(shell date -u +%Y%m%dT%H%M%SZ)

.DEFAULT_GOAL := help

.PHONY: help
help: ## List targets
	@grep -hE '^[a-zA-Z_-]+:.*?## ' $(MAKEFILE_LIST) | awk 'BEGIN{FS=":.*?## "}{printf "  %-22s %s\n", $$1, $$2}'

# ---------- toolchain ----------

.PHONY: doctor
doctor: ## Verify every pinned tool version
	@tools_ok=1; \
	check() { local name=$$1 want=$$2 got=$$3; if [ "$$got" != "$$want" ]; then echo "  $$name: WANT $$want GOT $$got"; tools_ok=0; else echo "  $$name: OK ($$got)"; fi }; \
	check go 1.27.4 "$$(go version 2>/dev/null | awk '{print $$3}' | sed s/go//)"; \
	check node 26 "$$(node --version 2>/dev/null | sed s/v// | cut -d. -f1)"; \
	check golangci-lint 2.15.0 "$$(golangci-lint version 2>/dev/null | awk '{print $$4}' | sed s/v//)"; \
	check sqlc 1.31.1 "$$(sqlc version 2>/dev/null | sed s/v//)"; \
	check lefthook 2.2.2 "$$(lefthook version 2>/dev/null | sed s/v//)"; \
	check gitleaks 8.30.1 "$$(gitleaks version 2>/dev/null | sed s/v//)"; \
	check flyctl 0.5.8 "$$(flyctl version 2>/dev/null | awk '{print $$NF}' | sed s/v//)"; \
	check shellcheck 0.11.0 "$$(shellcheck --version 2>/dev/null | awk '/^version:/{print $$2}')"; \
	npm ls --depth=0 --parseable 2>/dev/null | grep -q '@playwright/test' && echo "  @playwright/test: OK" || { echo "  @playwright/test: MISSING"; tools_ok=0; }; \
	[ $$tools_ok -eq 1 ] && echo "doctor: all OK" || { echo "doctor: FAILURES"; exit 1; }

.PHONY: browsers
browsers: ## Install Playwright browsers for local runs
	npx playwright install chromium webkit

# ---------- build & fast checks ----------

.PHONY: build
build: ## Build the server binary into bin/server
	go build -o bin/server ./cmd/server

.PHONY: fmt
fmt: ## gofmt and tidy
	gofmt -w $$(git ls-files '*.go')
	go mod tidy

.PHONY: fmt-check
fmt-check: ## Fail if formatting or module files drift
	@bad=$$(gofmt -l . 2>/dev/null | grep -v '^$$' || true); \
	if [ -n "$$bad" ]; then echo "gofmt needed:"; echo "$$bad"; exit 1; fi
	go mod tidy -diff >/dev/null 2>&1 || { go mod tidy; git diff --quiet go.mod go.sum || { echo "go.mod/go.sum drifted"; git checkout go.mod go.sum; exit 1; }; }

.PHONY: vet
vet:
	go vet ./...

.PHONY: lint
lint:
	golangci-lint run ./...

.PHONY: test-unit
test-unit:
	go test -race -count=1 -shuffle=on ./...

.PHONY: check-fast
check-fast: fmt-check vet lint test-unit ## Fast gate for worker loops

# ---------- deeper checks ----------

.PHONY: generate
generate: ## sqlc codegen (no-op until migrations exist)
	@if ls migrations/*.sql >/dev/null 2>&1; then sqlc generate; else echo "generate: no migrations yet, skip"; fi

.PHONY: generate-check
generate-check:
	@if ls migrations/*.sql >/dev/null 2>&1; then \
		sqlc generate && git diff --exit-code -- internal/store/gen || { echo "sqlc output not committed"; exit 1; }; \
	else echo "generate-check: no migrations yet, skip"; fi

.PHONY: cover
cover: ## Coverage floor on the policy's coverage packages
	@floor=$$(jq -r '.coverage_floor' harness/policy.json); \
	pkgs=$$(jq -r '.coverage_packages | join(" ")' harness/policy.json); \
	if [ -z "$$pkgs" ]; then echo "cover: no coverage packages pinned yet, skip"; exit 0; fi; \
	go test -cover $$pkgs | awk -v floor=$$floor '/^ok/ { cov=$$5; gsub(/%/,"",cov); print; if (cov+0 < floor+0) { print "coverage floor miss: " $$2 " " cov "% < " floor "%"; bad=1 } } END { exit bad?1:0 }'

.PHONY: vuln
vuln:
	go tool govulncheck ./...

.PHONY: templates-check
templates-check: ## Render every template with fixtures, validate the HTML
	GL_RENDER_SNAPSHOTS=1 go test ./internal/web/... -run TestRenderAll -count=1
	npx html-validate testdata/rendered

.PHONY: js-check
js-check:
	npx tsc --noEmit -p tsconfig.json

.PHONY: test-acceptance-go
test-acceptance-go: build ## Black-box Go acceptance tests against a running server
	bash scripts/acceptance-go.sh

.PHONY: e2e
e2e:
	npx playwright test

.PHONY: e2e-task
e2e-task: ## Only the specs for TASK's claims
	@if [ -z "$(TASK)" ]; then echo "usage: make e2e-task TASK=T-###"; exit 2; fi
	@ids=$$(jq -r --arg id "$(TASK)" '.tasks[] | select(.id == $$id) | .claims[]' harness/tasks/graph.json | sed 's/^/@/' | paste -sd'|' -); \
	if [ -z "$$ids" ]; then echo "no claims for $(TASK)"; exit 0; fi; \
	echo "e2e-task: grep $$ids"; npx playwright test --grep "$$ids" --pass-with-no-tests

.PHONY: a11y
a11y:
	npx playwright test --grep @a11y --pass-with-no-tests

.PHONY: visual
visual:
	npx playwright test --grep @visual --pass-with-no-tests

.PHONY: secrets
secrets:
	gitleaks detect --source . --no-banner --redact

.PHONY: phase-grep
phase-grep:
	@if grep -rnE '\bPhase [0-9]' cmd/server internal web migrations 2>/dev/null; then \
		echo "forbidden 'Phase N' reference in app code"; exit 1; fi

.PHONY: check
check: check-fast generate-check cover vuln templates-check js-check test-acceptance-go e2e a11y visual secrets phase-grep ## Everything; the gate definition

.PHONY: ci
ci: check build docker-build ## What CI runs

.PHONY: docker-build
docker-build:
	docker build -t groveland-lights:dev .

# ---------- harness ----------

.PHONY: protect
protect: ## Regenerate the protected manifest + ratchet baseline
	go run ./cmd/harness protect

.PHONY: harness-verify
harness-verify: ## verify-agents + harness unit tests + deterministic selftest
	go run ./cmd/harness verify-agents
	go test ./cmd/harness/...
	go run ./cmd/harness selftest

.PHONY: selftest-live
selftest-live:
	go run ./cmd/harness selftest --live

.PHONY: run
run: ## Start the scheduler
	go run ./cmd/harness sched

.PHONY: status
status:
	go run ./cmd/harness status

.PHONY: stop
stop:
	touch harness/STOP

.PHONY: plan
plan: ## Ask the planner agent to expand PHASE=n into packets
	@if [ -z "$(PHASE)" ]; then echo "usage: make plan PHASE=n"; exit 2; fi
	opencode run --standalone --auto --agent planner --format json --archived \
		--title "plan-$(PHASE)" --file docs/PLAN.md \
		"Plan the next phase per docs/PLAN.md and harness/state.md. Produce the contract delta and task packets for phase $(PHASE). Follow your instructions."

.PHONY: holdout
holdout: ## Run hidden holdout suites (absolute HOLDOUT_DIR from policy)
	@dir=$${HOLDOUT_DIR:-$$(jq -r '.holdout_dir' harness/policy.json)}; \
	if [ ! -d "$$dir" ]; then echo "holdout: no holdout dir, skip"; exit 0; fi; \
	HOLDOUT_DIR="$$dir" bash scripts/holdout-run.sh

.PHONY: validate
validate: ## Run all @A-<n>xx specs for MILESTONE=Mn with artifacts kept
	@if [ -z "$(MILESTONE)" ]; then echo "usage: make validate MILESTONE=M2"; exit 2; fi
	n=$${MILESTONE#M}; mkdir -p harness/runs/$(TS)/validate; \
	npx playwright test --grep "@A-$$n[0-9]" --pass-with-no-tests || exit 1; \
	cp -r test-results/* harness/runs/$(TS)/validate/ 2>/dev/null || true; \
	echo "validate: artifacts in harness/runs/$(TS)/validate"

.PHONY: smoke
smoke: ## @smoke specs against BASE_URL (deployed)
	@if [ -z "$(BASE_URL)" ]; then echo "usage: make smoke BASE_URL=https://..."; exit 2; fi
	BASE_URL=$(BASE_URL) npx playwright test --grep @smoke --pass-with-no-tests

.PHONY: lighthouse
lighthouse:
	@echo "lighthouse: n/a until Phase 3 (D9)"

.PHONY: drill-local
drill-local:
	@echo "drill-local: n/a until Phase 5"
