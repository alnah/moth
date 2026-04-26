# AGENTS.md

## Principles

- KISS: keep it simple and stupid.
- DRY: on behaviors.
- WET: on data structure.

## Source of truth

- Explore: always audit codebase, documentation, and stack before jumping into a task; respect language or tech idioms.
- Codebase: codebase's root.
- Documentation: `README.md`, `docs/`.
- Stack: `~/workspace/stack/` contains local source-of-truth clones named `owner.repo`; use `~/workspace/stack/maps/<TECH>.md` as navigation map when available.
- Go: always start with `~/workspace/stack/maps/GO.md` to choose task-relevant local sources; prefer lookup in stack and maps to orientate yourself.
- Go stack roles:
  - `golang.go` = spec, memory model, stdlib, toolchain, API diffs;
  - `golang.website` = go.dev docs, blog, tutorials, release notes;
  - `golang.go.wiki` = wiki history;
  - `golang.tour` = Tour only;
  - `golang.pkgsite` = pkg.go.dev implementation only.
- Go behavior truth: always project version/deps, source, tests, examples, release notes, API diffs, official prose; prefer web search last.
- Go practice truth: Go wiki review/test/concurrency comments and official blog are good sources; verify exact behavior in source, docs, tests, spec, memory model, and API diffs when relevant.
- Go reading rule: read only task-relevant sources; spec for language questions, memory model for concurrency, release/API diffs for version/runtime behavior, stdlib docs/source/tests for touched packages.
- Go caution: never treat `Effective Go` as sufficient or current on its own or current; always be curious about other sources of truth.
- Go practices: use Go wiki review/test/concurrency comments as good sources; prefer consumer-side interfaces unless strong reason; wrap errors with context and `%w`; use `panic` only for truly exceptional cases; pass `context.Context` as first parameter and do not store it casually; make goroutine and resource lifetimes explicit; reuse documented concurrency-safe clients, pools, transports, and compiled objects; close or stop owned resources; pass context to logging and telemetry when available, etc.

## Files

- LOC: prefer ~500 LOC; refactor/split as needed.
- Filename: never generic names like `tools` or `utils`; never prepend the module name in the filename; always think what does the file to define its name.

## Symbols

- Style: always intention-revealing, one meaning, stable style, readable; never vague, misleading, noisy.
- Vars: always from data.
- Bools: always from state/question.
- Functions: always from action.
- Classes/Types: always from thing.
- Constants: always from fixed value.
- Collections: always from plural.
- Event: always from event, effet.

## Docstrings

- Style: always short, specific, behavior-driven, about contract, clear on inputs/outputs/side-efects/edge cases, state units and formats.

## Test-Driven Development

- Specs: always toward GRASP architecture; input = user + codebase; output = specs + todos; 1 session = 1 agent Architect; audit package boundaries, contracts, existing tests, release-note deltas first; use `rg`, `go doc`, `gopls symbols`, `gopls definition`, `gopls references`; never do Red.
- Red: always write failing tests; input = specs + todos; output = red tests; 1 session = 1 agent QA; follow `testing`, `httptest`, `fstest`, `slogtest`, fuzz, coverage, race guidance when relevant; use `go test` modes to prove failure first; never touch Specs, never do Green.
- Green: always make it work/correct to satisfy specs; input = red tests + specs; output = code; 1 session = 1 agent Dev; prefer minimal code, stdlib-first, consumer-side interfaces, explicit errors and context propagation; use `go doc`, `gopls definition`, `gopls references`, `gopls implementation`; never touch Red, never do Refactor.
- Refactor-code: always toward SOLID design; input = code; output = clean code; 1 session = 1 agent Expert Dev; use `gopls symbols`, `gopls definition`, `gopls references`, `gopls implementation`, `gopls prepare_rename`, `gopls rename`; never rename symbols by raw search/replace when semantic rename applies; never do Refactor-test.
- Refactor-tests: always toward SOLID design; input = tests; output = clean tests; 1 session = 1 agent Expert QA; use `gopls symbols`, `gopls definition`, `gopls references`, `gopls prepare_rename`, `gopls rename`; keep fixtures small, failures explicit, helpers intention-revealing; never touch Refactor-code, never do Expand.
- Expand: when feature done, always tests for safety, edge cases, regressions; input = user + codebase; output = new TDD cycle; 1 session = 1 agent Architect; use bugs, missed branches, fuzz seeds, race findings, coverage gaps as inputs; the loop goes on.

## Go refactor tools

- Symbols: use `gopls symbols <file>`.
- Definition: use `gopls definition <file>:<line>:<column>`.
- References: use `gopls references <file>:<line>:<column>`.
- Implementation: use `gopls implementation <file>:<line>:<column>`.
- Rename-check: use `gopls prepare_rename <file>:<line>:<column>` before rename.
- Rename: use `gopls rename -w <file>:<line>:<column> <new-name>` for semantic rename.
- Rename rule: never use raw search/replace for symbol rename when `gopls rename` applies.
- Imports: use `gopls imports <file>` or `goimports -w .`.
- Format: use `gopls format <file>` or `gofmt -w .`.
- Diagnostics: use `gopls check <file>`, `go vet ./...`, `golangci-lint run`.

## Coverage and Codecov

- Coverage focus: prioritize meaningful coverage in `internal/...`; keep `cmd/...` thin and test wiring, flags, config loading, startup, shutdown, exit paths, and error propagation.
- Coverage: use `go test -cover ./...` for quick signal.
- Coverage profile: use `go test -coverprofile=coverage.out ./...`.
- Coverage report: use `go tool cover -func=coverage.out` and `go tool cover -html=coverage.out`.
- Codecov: use as diff and trend signal, never as substitute for test design.
- Codecov review: always inspect uncovered changed lines, especially in `internal/...`; also inspect critical paths, error paths, concurrency paths, and regressions in `cmd/...` wiring.
- Codecov rule: never chase percent only; prefer meaningful coverage on behavior, boundaries, and failure modes.
- Codecov architecture rule: never move logic into `cmd/` to avoid tests; if logic grows, extract it into `internal/` and test it there.

## Before a commit

- Build: `go build ./...` always pass.
- Testing: all tests always pass, include race detector when applicable.
- Lint: always lint with `go vet ./...` and `golangci-lint run`; then fix.
- Format: always format with `gofmt`; then fix imports with `goimports`.
- Scripts:
  - `gofmt -w .`
  - `goimports -w .`
  - `go test ./...`
  - `go test -race ./...`
  - `go test -cover ./...`
  - `go test -coverprofile=coverage.out ./...`
  - `go tool cover -func=coverage.out`
  - `go vet ./...`
  - `golangci-lint run`
  - `go build ./...`
