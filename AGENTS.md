# AGENTS.md

## Principles

KISS: keep it simple and stupid.
DRY: on behaviors.
WET: on data structure.

## Source of truth

Explore: always audit codebase, documentation, other tech stack we use, respect language idioms, before jumping into a task.
Codebase: codebase's root.
Documentation: `README.md`, `docs/`.
Stack: `~/workspace/stack/`, always explore repos from stack, third-parties here.
Go repo: `~/workspace/stack/golang.go/`; read `doc/go_spec.html`, `doc/go_mem.html`, `doc/godebug.md`, latest `api/go1.*.txt`, then stdlib docs, source, examples, tests under `src/`.
Go wiki repo: `~/workspace/stack/golang.go.wiki/`; read `CodeReviewComments.md` and `TestComments.md`; use them as live review and test checklist.
Go official prose: follow `GO.md` reading map for current docs and blog posts; prefer source, tests, examples, release notes, API diffs over generic web search.
Go caution: never treat `Effective Go` as sufficient or current on its own.
Go practices: consumer-side interfaces unless strong reason; wrap errors with context and `%w`; `panic` only for truly exceptional cases; `context.Context` first parameter, never stored casually; goroutine lifetime and shutdown always explicit; reuse `http.Client` and `http.Transport`; pass context to logging when available.
Go session Architect: read `GO.md`; Go wiki page `CodeReviewComments.md`; docs `/doc/code`, `/doc/modules/layout`, `/doc/modules/managing-dependencies`, `/doc/modules/developing`, `/doc/modules/release-workflow`; latest release notes from `/doc/devel/release`, `/doc/go1.xx`, latest `api/go1.*.txt`, `doc/godebug.md`; then touched stdlib package docs and examples before Specs.
Go session QA: read `go doc -all testing`; Go wiki page `TestComments.md`; `net/http/httptest`, `testing/fstest`, `testing/slogtest` when relevant; docs `/doc/tutorial/add-a-test`, `/doc/tutorial/fuzz`, `/doc/security/fuzz`, `/doc/build-cover`, `/doc/articles/race_detector` before Red.
Go session Dev: read touched stdlib package docs, source, tests, examples; `go doc -all context`, `go doc -all errors`; `doc/go_mem.html` when concurrency exists; docs `/blog/go1.13-errors`, `/blog/errors-are-values`, `/blog/context-and-structs`, `/blog/slog` when relevant; latest `api/go1.*.txt` and `doc/godebug.md` when version or runtime behavior may matter; then Green.
Go session Expert Dev: read Go wiki page `CodeReviewComments.md`; `go doc -all go/doc/comment`; stdlib `doc.go`; docs `/blog/package-names`, `/blog/defer-panic-and-recover`, `/blog/errors-are-values`, `/blog/context-and-structs`, `/blog/slog` when relevant; latest release notes and API diffs before Refactor-code.
Go session Expert QA: read Go wiki page `TestComments.md`; `go doc -all testing`; `testing/fstest`, `net/http/httptest`, `testing/slogtest` when relevant; docs `/doc/tutorial/fuzz`, `/doc/security/fuzz`, `/doc/build-cover`, `/doc/articles/race_detector`; latest release notes and `api/go1.*.txt` before Refactor-tests.

## Files

LOC: prefer ~500 LOC; refactor/split as needed.
Filename: never generic names like `tools` or `utils`; never prepend the module name in the filename; always think what does the file to define its name.

## Symbols

Style: always intention-revealing, one meaning, stable style, readable; never vague, misleading, noisy.
Vars: always from data.
Bools: always from state/question.
Functions: always from action.
Classes/Types: always from thing.
Constants: always from fixed value.
Collections: always from plural.
Event: always from event, effet.

## Docstrings

Style: always short, specific, behavior-driven, about contract, clear on inputs/outputs/side-efects/edge cases, state units and formats.

## Test-Driven Development

Specs: always toward GRASP architecture; input = user + codebase; output = specs + todos; 1 session = 1 agent Architect; audit package boundaries, contracts, existing tests, release-note deltas first; use `rg`, `go doc`, `gopls symbols`, `gopls definition`, `gopls references`; never do Red.
Red: always write failing tests; input = specs + todos; output = red tests; 1 session = 1 agent QA; follow `testing`, `httptest`, `fstest`, `slogtest`, fuzz, coverage, race guidance when relevant; use `go test` modes to prove failure first; never touch Specs, never do Green.
Green: always make it work/correct to satisfy specs; input = red tests + specs; output = code; 1 session = 1 agent Dev; prefer minimal code, stdlib-first, consumer-side interfaces, explicit errors and context propagation; use `go doc`, `gopls definition`, `gopls references`, `gopls implementation`; never touch Red, never do Refactor.
Refactor-code: always toward SOLID design; input = code; output = clean code; 1 session = 1 agent Expert Dev; use `gopls symbols`, `gopls definition`, `gopls references`, `gopls implementation`, `gopls prepare_rename`, `gopls rename`; never rename symbols by raw search/replace when semantic rename applies; never do Refactor-test.
Refactor-tests: always toward SOLID design; input = tests; output = clean tests; 1 session = 1 agent Expert QA; use `gopls symbols`, `gopls definition`, `gopls references`, `gopls prepare_rename`, `gopls rename`; keep fixtures small, failures explicit, helpers intention-revealing; never touch Refactor-code, never do Expand.
Expand: when feature done, always tests for safety, edge cases, regressions; input = user + codebase; output = new TDD cycle; 1 session = 1 agent Architect; use bugs, missed branches, fuzz seeds, race findings, coverage gaps as inputs; the loop goes on.

## Go refactor tools

Symbols: use `gopls symbols <file>`.
Definition: use `gopls definition <file>:<line>:<column>`.
References: use `gopls references <file>:<line>:<column>`.
Implementation: use `gopls implementation <file>:<line>:<column>`.
Rename-check: use `gopls prepare_rename <file>:<line>:<column>` before rename.
Rename: use `gopls rename -w <file>:<line>:<column> <new-name>` for semantic rename.
Rename rule: never use raw search/replace for symbol rename when `gopls rename` applies.
Imports: use `gopls imports <file>` or `goimports -w .`.
Format: use `gopls format <file>` or `gofmt -w .`.
Diagnostics: use `gopls check <file>`, `go vet ./...`, `golangci-lint run`.

## Coverage and Codecov

Coverage focus: prioritize meaningful coverage in `internal/...`; keep `cmd/...` thin and test wiring, flags, config loading, startup, shutdown, exit paths, and error propagation.
Coverage: use `go test -cover ./...` for quick signal.
Coverage profile: use `go test -coverprofile=coverage.out ./...`.
Coverage report: use `go tool cover -func=coverage.out` and `go tool cover -html=coverage.out`.
Codecov: use as diff and trend signal, never as substitute for test design.
Codecov review: always inspect uncovered changed lines, especially in `internal/...`; also inspect critical paths, error paths, concurrency paths, and regressions in `cmd/...` wiring.
Codecov rule: never chase percent only; prefer meaningful coverage on behavior, boundaries, and failure modes.
Codecov architecture rule: never move logic into `cmd/` to avoid tests; if logic grows, extract it into `internal/` and test it there.

## Before a commit

Build: `go build ./...` always pass.
Testing: all tests always pass, include race detector when applicable.
Lint: always lint with `go vet ./...` and `golangci-lint run`; then fix.
Format: always format with `gofmt`; then fix imports with `goimports`.
Scripts:
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
