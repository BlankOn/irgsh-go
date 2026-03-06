# STATE.md -- Work Tracking

Branch: `refactor/cli`
Last verified: `go build && go vet && go test -race` all pass.

---

## Commit Guidelines

- Describe the change itself. Do not reference SPECS.md sections, item
  numbers, milestone names, or phase names in commit messages.
- Good: `refactor: unexport CLIUsecase struct fields`
- Bad: `refactor: address SPECS 4.2.2, milestone 2 item`

---

## Phases

### Phase 1: CLI Domain Layer

Rename `internal/cli/entity/` to `internal/cli/domain/` and update all
imports. This aligns with the go-clean-arch v4 convention.

| Task | Status |
|------|--------|
| Rename `entity/` directory to `domain/` | done |
| Update imports in `internal/cli/usecase/*.go` | done |
| Update imports in `internal/cli/repository/*.go` | done |
| Update imports in `cmd/cli/handlers.go` | done |
| Update imports in test files | done |
| Verify: `go build && go test -race ./...` | done |

### Phase 2: CLI Clean Architecture Fixes

| Task | Status |
|------|--------|
| Unexport `CLIUsecase` struct fields | done |
| Define `CLIService` interface in `cmd/cli/handlers.go` | done |
| Fix `http.DefaultClient` without timeout in `package.go` | done |
| Fix `checkResponse` body discard in `chief_client.go` | done |
| URL-encode `FetchLog` path in `chief_client.go` | done |
| Replace fragile `genchangesCmd` shell pipeline with `filepath.Glob` | done |
| Separate optional `.xz` move from required commands (`\|\| true` fix) | done |
| Shell-quote `packageName`/`packageVersion` in orig tarball creation | done |
| Wire signal context in `cmd/cli/handlers.go` | done |
| Verify: `go build && go test -race ./...` | done |

### Phase 3: Chief Domain & Ports

Create the chief domain layer and interface definitions.

| Task | Status |
|------|--------|
| Create `internal/chief/domain/submission.go` | todo |
| Create `internal/chief/domain/job.go` | todo |
| Create `internal/chief/domain/maintainer.go` | todo |
| Create `internal/chief/domain/errors.go` | todo |
| Create `internal/chief/domain/status.go` (centralized status mapping) | todo |
| Create `internal/chief/usecase/ports.go` (all interfaces) | todo |
| Verify: `go build && go test -race ./...` | todo |

### Phase 4: Chief Service Split

Split the 1294-line `ChiefUsecase` god object into focused services.

| Task | Status |
|------|--------|
| Create `MachineryTaskQueue` adapter in `repository/machinery.go` | todo |
| Extract `SubmissionService` (SubmitPackage, RetryPipeline, BuildISO) | todo |
| Extract `UploadService` (UploadArtifact, UploadLog, UploadSubmission) | todo |
| Extract `StatusService` (BuildStatus, ISOStatus) | todo |
| Extract `MaintainerService` (GetMaintainers, ListMaintainersRaw) | todo |
| Extract `DashboardService` (RenderIndexHTML) | todo |
| Define `ChiefService` interface in `cmd/chief/handler.go` | todo |
| Update handler wiring in `cmd/chief/main.go` | todo |
| Verify: `go build && go test -race ./...` | todo |

### Phase 5: Chief Handler & Server Fixes

| Task | Status |
|------|--------|
| Fix `writeUsecaseError` Content-Type | todo |
| Move config loading into `app.Action` | todo |
| Add HTTP server read/write timeouts | todo |
| Escape `s.version` in HTML with `html.EscapeString` | todo |
| Verify: `go build && go test -race ./...` | todo |

### Phase 6: Dashboard Template Extraction

| Task | Status |
|------|--------|
| Create `DashboardData` struct for template rendering | todo |
| Convert `RenderIndexHTML` to `html/template` | todo |
| Embed template via `//go:embed` | todo |
| Verify: `go build && go test -race ./...` | todo |

### Phase 7: Documentation & CI

| Task | Status |
|------|--------|
| Update CLAUDE.md directory structure | todo |
| Update CLAUDE.md test file locations | todo |
| Document submission type coupling (CLI vs chief) | todo |
| Fix Makefile test target to `go test ./...` | todo |
| Fix coverage reporting (single invocation) | todo |
| Remove redundant `cp -rf -R` flags | todo |
| Verify: `go build && go test -race ./...` | todo |

---

## Notes

- CLAUDE.md must be updated whenever directory structure or architecture
  changes (e.g., after Phase 1 rename, after Phase 3 domain creation,
  after Phase 4 service split).
- Builder, repo, and iso refactoring is out of scope for this branch.
  Those have critical shell injection issues tracked in SPECS.md section
  7.2 for future PRs.
- All phases should be individually committable. Each phase ends with a
  passing build+test verification.
