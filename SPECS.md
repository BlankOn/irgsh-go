# SPECS.md -- CLI & Chief Clean Architecture Refactoring

Reference specification for PR #193 (`refactor/cli` branch).
Goal: fully refactor **CLI** and **chief** following
[bxcodec/go-clean-arch v4](https://github.com/bxcodec/go-clean-arch) patterns.

---

## Table of Contents

1. [Reference Architecture](#1-reference-architecture)
2. [Current State](#2-current-state)
3. [Target Architecture](#3-target-architecture)
4. [CLI Refactoring Spec](#4-cli-refactoring-spec)
5. [Chief Refactoring Spec](#5-chief-refactoring-spec)
6. [Shared Packages](#6-shared-packages)
7. [Outstanding Issues](#7-outstanding-issues)
8. [Applied Fixes](#8-applied-fixes)
9. [CI & Build](#9-ci--build)
10. [Verification Checklist](#10-verification-checklist)

---

## 1. Reference Architecture

Based on [bxcodec/go-clean-arch v4](https://github.com/bxcodec/go-clean-arch)
(Uncle Bob's Clean Architecture adapted for Go).

### Core Rules

1. **Dependency rule**: dependencies point inward only.
   `delivery -> usecase -> domain`. Never the reverse.
2. **Interfaces at the consumer**: the package that *calls* a behavior
   defines the interface, not the package that *implements* it.
   Go structural typing satisfies interfaces implicitly.
3. **Domain knows nothing**: domain/entity types import only stdlib.
4. **Composition root**: `main.go` is the only place that knows all
   concrete types. It wires everything together (manual DI).
5. **Unexported fields**: usecase structs hold unexported dependencies
   injected via constructor. Callers cannot bypass the usecase.

### Layer Mapping (go-clean-arch v4 -> irgsh-go)

| go-clean-arch | irgsh-go equivalent | Responsibility |
|---------------|---------------------|----------------|
| `domain/` | `internal/{component}/domain/` | Entity structs, sentinel errors, no methods with side effects |
| `article/service.go` | `internal/{component}/usecase/` | Business logic; defines repository interfaces it consumes |
| `internal/repository/mysql/` | `internal/{component}/repository/` | Infrastructure adapters (HTTP, filesystem, shell, DB) |
| `internal/rest/` | `cmd/{component}/` (handlers) | Delivery layer; defines service interface it consumes |
| `app/main.go` | `cmd/{component}/main.go` | Composition root, DI wiring |

### Interface Ownership Pattern

```
delivery layer                usecase layer               repository layer
  defines:                      defines:                    implements:
  ServiceInterface  -------->   (is the Service)
                                RepoInterface  ---------->  (is the Repository)
```

- Delivery defines an interface for the usecase methods it calls
- Usecase defines interfaces for the repository methods it calls
- Repository implements those interfaces without importing usecase
- `main.go` wires concrete types that satisfy all interfaces

### Directory Convention

```
internal/{component}/
  domain/          # entities + sentinel errors (no external imports)
  usecase/         # business logic + port interfaces
    ports.go       # all interfaces consumed by usecase
    {feature}.go   # one file per feature/workflow
  repository/      # infrastructure adapters
    {adapter}.go   # one file per external system
cmd/{component}/
  main.go          # composition root
  handler.go       # delivery layer (HTTP / CLI adapter)
```

---

## 2. Current State

### What Changed (origin/master -> refactor/cli)

81 files changed, 6626 insertions, 3224 deletions.

| Metric | origin/master | refactor/cli | Change |
|--------|---------------|--------------|--------|
| `cmd/cli/main.go` | 1,081 lines | 47 lines | -95.6% |
| `cmd/chief/handler.go` | 775 lines | 284 lines | -63.4% |
| Total `cmd/` layer | 2,557 lines | 771 lines | -70% |
| Global state variables | 11 | 0 | eliminated |
| Testable interfaces | 0 | 11 | +11 |
| Architectural layers | 2 | 4 | entity/usecase/repo/delivery |

### Build & Test Status (verified)

```
go build ./...     # OK
go test -race ./...  # all pass
go vet ./...       # clean
```

### Current File Tree

```
cmd/cli/
  main.go              # 47 lines, composition root
  handlers.go          # 235 lines, thin CLI adapter

internal/cli/
  entity/
    config.go          # Config struct
    errors.go          # ErrRepoOrBranchNotFound
    iso.go             # ISOSubmission
    status.go          # VersionResponse, UploadResponse, SubmitResponse, PackageStatus, ISOStatus
    submission.go      # Submission, SubmitParams
    update.go          # GitHubRelease, GitHubReleaseAsset
  usecase/
    cli.go             # CLIUsecase struct + constructor
    ports.go           # 11 interfaces (ConfigStore, PipelineStore, ChiefAPI, etc.)
    config.go          # SaveConfig logic
    package.go         # SubmitPackage, PackageStatus, PackageLog
    iso.go             # SubmitISO, ISOStatus, ISOLog
    retry.go           # RetryPipeline
    update.go          # UpdateCLI
    errors.go          # ErrConfigMissing, ErrPipelineIDMissing, isHTTPNotFound
    config_test.go
    package_test.go
    iso_test.go
    retry_test.go
    mocks_test.go      # manual mock implementations
  repository/
    chief_client.go    # HTTPChiefClient -> ChiefAPI
    config_store.go    # FileConfigStore -> ConfigStore
    pipeline_store.go  # FilePipelineStore -> PipelineStore
    repo_sync.go       # GitRepoSync -> RepoSync
    shell_runner.go    # ShellRunner -> ShellRunner
    debian_packager.go # ShellDebianPackager -> DebianPackager
    gpg_signer.go      # ShellGPGSigner -> GPGSigner
    release_fetcher.go # GitHubReleaseFetcher -> ReleaseFetcher
    update_applier.go  # GoUpdateApplier -> UpdateApplier
    prompter.go        # TerminalPrompter -> Prompter
    config_store_test.go
    pipeline_store_test.go

cmd/chief/
  main.go              # 205 lines, composition root + server setup
  handler.go           # 284 lines, HTTP handlers

internal/chief/
  usecase/
    chief.go           # 1294 lines (GOD OBJECT -- needs splitting)
    types.go           # Submission, ISOSubmission, BuildStatusResponse, Maintainer, etc.
  repository/
    gpg.go             # GPG operations via exec.Command
    storage.go         # Filesystem + tar + sudo operations
```

---

## 3. Target Architecture

### CLI Target (largely achieved, needs refinement)

```
cmd/cli/
  main.go              # composition root (DI wiring only)
  handler.go           # delivery layer: CLI command -> usecase calls

internal/cli/
  domain/              # RENAME from entity/ to match go-clean-arch convention
    config.go
    errors.go
    iso.go
    status.go
    submission.go
    update.go
  usecase/
    ports.go           # interfaces consumed by usecase
    cli.go             # CLIUsecase struct (unexported fields)
    config.go
    package.go
    iso.go
    retry.go
    update.go
  repository/
    chief_client.go
    config_store.go
    pipeline_store.go
    repo_sync.go
    shell_runner.go
    debian_packager.go
    gpg_signer.go
    release_fetcher.go
    update_applier.go
    prompter.go
```

### Chief Target (major restructuring needed)

```
cmd/chief/
  main.go              # composition root (DI wiring)
  handler.go           # delivery layer: HTTP routes -> usecase calls
                       # defines ChiefService interface it consumes

internal/chief/
  domain/              # NEW -- shared types for chief
    submission.go      # Submission, ISOSubmission types
    maintainer.go      # Maintainer type
    job.go             # BuildStatusResponse, job-related types
    errors.go          # sentinel errors
  usecase/
    ports.go           # NEW -- all interfaces consumed by usecase
    submission.go      # SubmitPackage, RetryPipeline, BuildISO
    status.go          # BuildStatus, ISOStatus
    upload.go          # UploadArtifact, UploadLog, UploadSubmission
    maintainer.go      # GetMaintainers, ListMaintainersRaw
    dashboard.go       # RenderIndexHTML (extracted from chief.go)
    version.go         # GetVersion
  repository/
    gpg.go             # GPG operations (implements usecase.GPGVerifier)
    storage.go         # Filesystem operations (implements usecase.FileStorage)
    machinery.go       # NEW -- task queue adapter (implements usecase.TaskQueue)
    dashboard_data.go  # NEW -- data aggregation for dashboard
```

---

## 4. CLI Refactoring Spec

### 4.1 Completed Work

The CLI refactoring is largely done. The monolithic 1081-line `main.go` has
been split into clean architecture layers with proper DI.

### 4.2 Remaining Fixes

#### 4.2.1 Rename `entity/` to `domain/`

go-clean-arch v4 uses `domain/` for the innermost layer. Rename
`internal/cli/entity/` to `internal/cli/domain/`. Update all imports.

```
internal/cli/entity/ -> internal/cli/domain/
```

**Files affected**: every file in `internal/cli/usecase/`, `internal/cli/repository/`,
`cmd/cli/handlers.go` that imports entity.

#### 4.2.2 Unexport CLIUsecase Fields

Current (violates encapsulation):
```go
type CLIUsecase struct {
    Config    ConfigStore      // exported
    Pipelines PipelineStore    // exported
    Chief     ChiefAPI         // exported
    ...
}
```

Target:
```go
type CLIUsecase struct {
    config    ConfigStore      // unexported
    pipelines PipelineStore    // unexported
    chief     ChiefAPI         // unexported
    ...
}
```

Update all internal references from `u.Config` to `u.config`, etc.

#### 4.2.3 Add Delivery-Layer Interface

In `cmd/cli/handlers.go`, define the interface that the delivery layer
consumes (matching go-clean-arch pattern where delivery defines its own
interface for the service):

```go
// CLIService defines the operations available to CLI commands.
type CLIService interface {
    SaveConfig(cfg domain.Config) error
    SubmitPackage(ctx context.Context, params domain.SubmitParams) (domain.SubmitResponse, error)
    PackageStatus(ctx context.Context, pipelineID string) (domain.PackageStatus, error)
    PackageLog(ctx context.Context, pipelineID string) (string, string, error)
    SubmitISO(ctx context.Context, repoURL, branch string) (domain.SubmitResponse, error)
    ISOStatus(ctx context.Context, pipelineID string) (domain.ISOStatus, error)
    ISOLog(ctx context.Context, pipelineID string) (string, error)
    RetryPipeline(ctx context.Context, pipelineID string) (domain.RetryResponse, error)
    UpdateCLI(ctx context.Context) error
}
```

Handlers accept `CLIService` instead of `*usecase.CLIUsecase`.

#### 4.2.4 Fix `http.DefaultClient` Without Timeout

`internal/cli/usecase/package.go:117` -- tarball download uses
`http.DefaultClient` with no timeout. Inject an HTTP client with timeout
via the usecase or use the existing `ChiefAPI` pattern.

#### 4.2.5 Fix `checkResponse` Body Discard

`internal/cli/repository/chief_client.go:46-51` -- read the response body
before returning `HTTPStatusError` so error messages from chief are preserved.

#### 4.2.6 URL-Encode `FetchLog` Path

`internal/cli/repository/chief_client.go:327` -- `logPath` is not
URL-encoded before concatenation into the request URL.

#### 4.2.7 Fix Shell Command Fragility

- `package.go:273` -- `genchangesCmd` uses `ls | grep dsc | tr` pipeline.
  Replace with `filepath.Glob("../*.dsc")` in Go.
- `package.go:291` -- `|| true` swallows `mkdir` failures. Separate the
  optional `.xz` move from required commands.
- `package.go:226-236` -- shell-quote `packageName`/`packageVersion` even
  though validated by regex (defense in depth).

#### 4.2.8 Wire Signal Context

`cmd/cli/handlers.go` uses `context.Background()` everywhere. Wire a
signal-based context (`signal.NotifyContext`) so SIGINT cancels in-flight
operations.

---

## 5. Chief Refactoring Spec

### 5.1 Problem Statement

`internal/chief/usecase/chief.go` is a 1294-line god object that mixes:
- HTML rendering (587 lines in `RenderIndexHTML`)
- GPG key parsing
- Package submission orchestration
- Build/ISO status querying
- File upload handling
- Retry logic
- State aggregation from Redis + SQLite

No interfaces are defined for chief's dependencies. Handler layer has no
service interface. Testing chief logic requires full infrastructure.

### 5.2 Target Decomposition

#### 5.2.1 Create `internal/chief/domain/`

Extract shared types from `internal/chief/usecase/types.go`:

```
internal/chief/domain/
  submission.go    # Submission, ISOSubmission (request types)
  job.go           # BuildStatusResponse, SubmitPayloadResponse
  maintainer.go    # Maintainer struct
  errors.go        # sentinel errors (ErrJobNotFound, ErrInvalidID, etc.)
```

These types must have:
- No imports from internal packages (only stdlib)
- JSON tags for wire format
- Validation constants (safeIDPattern) if needed

#### 5.2.2 Create `internal/chief/usecase/ports.go`

Define all interfaces the usecase layer consumes:

```go
package usecase

// GPGVerifier handles GPG key operations.
type GPGVerifier interface {
    ListKeysWithColons() (string, error)
    ListKeys() (string, error)
    VerifySignedSubmission(submissionPath string) error
    VerifyFile(filePath string) error
}

// FileStorage handles filesystem operations for submissions/artifacts.
type FileStorage interface {
    ArtifactsDir() string
    LogsDir() string
    SubmissionsDir() string
    SubmissionTarballPath(taskUUID string) string
    SubmissionDirPath(taskUUID string) string
    SubmissionSignaturePath(taskUUID string) string
    EnsureDir(path string) error
    ExtractSubmission(taskUUID string) error
    CopyFileWithSudo(src, dst string) error
    CopyDirWithSudo(src, dst string) error
    ChownWithSudo(path string) error
    ChownRecursiveWithSudo(path string) error
}

// TaskQueue abstracts the distributed task queue (machinery).
type TaskQueue interface {
    QueueBuildAndRepo(taskUUID string, payload []byte) error
    QueueISO(taskUUID string, payload []byte) error
    GetBuildState(taskUUID string) (buildState, repoState string, err error)
    GetISOState(taskUUID string) (string, error)
}

// JobStore persists job metadata.
type JobStore interface {
    RecordJob(job domain.JobInfo) error
    GetJob(taskUUID string) (*domain.JobInfo, error)
    GetRecentJobs(limit int) ([]*domain.JobInfo, error)
    UpdateJobState(taskUUID, state string) error
    UpdateJobStages(taskUUID, buildState, repoState, currentStage string) error
}

// ISOJobStore persists ISO job metadata.
type ISOJobStore interface {
    RecordISOJob(job domain.ISOJobInfo) error
    GetISOJob(taskUUID string) (*domain.ISOJobInfo, error)
    GetRecentISOJobs(limit int) ([]*domain.ISOJobInfo, error)
    UpdateISOJobState(taskUUID, state string) error
}

// InstanceRegistry tracks worker health.
type InstanceRegistry interface {
    ListInstances(instanceType, status string) ([]*domain.InstanceInfo, error)
    GetSummary() (domain.InstanceSummary, error)
}

// Notifier sends webhook notifications.
type Notifier interface {
    SendJobNotification(jobType, taskUUID, status string, info domain.JobNotificationInfo)
}
```

#### 5.2.3 Split ChiefUsecase Into Focused Services

**SubmissionService** (`internal/chief/usecase/submission.go`):
```go
type SubmissionService struct {
    gpg       GPGVerifier
    storage   FileStorage
    taskQueue TaskQueue
    jobStore  JobStore
    config    config.IrgshConfig
}

func (s *SubmissionService) SubmitPackage(submission domain.Submission) (domain.SubmitPayloadResponse, error)
func (s *SubmissionService) RetryPipeline(oldTaskUUID string) (domain.SubmitPayloadResponse, error)
func (s *SubmissionService) BuildISO(submission domain.ISOSubmission) (domain.SubmitPayloadResponse, error)
```

**UploadService** (`internal/chief/usecase/upload.go`):
```go
type UploadService struct {
    gpg     GPGVerifier
    storage FileStorage
}

func (s *UploadService) UploadArtifact(id string, file io.Reader) error
func (s *UploadService) UploadLog(id, logType string, file io.Reader) error
func (s *UploadService) UploadSubmission(tokenData []byte, blob io.Reader) (string, error)
```

**StatusService** (`internal/chief/usecase/status.go`):
```go
type StatusService struct {
    taskQueue TaskQueue
    jobStore  JobStore
    isoStore  ISOJobStore
}

func (s *StatusService) BuildStatus(uuid string) (domain.BuildStatusResponse, error)
func (s *StatusService) ISOStatus(uuid string) (string, string, error)
```

**MaintainerService** (`internal/chief/usecase/maintainer.go`):
```go
type MaintainerService struct {
    gpg GPGVerifier
}

func (s *MaintainerService) GetMaintainers() ([]domain.Maintainer, error)
func (s *MaintainerService) ListMaintainersRaw() (string, error)
```

**DashboardService** (`internal/chief/usecase/dashboard.go`):
```go
type DashboardService struct {
    jobStore  JobStore
    isoStore  ISOJobStore
    taskQueue TaskQueue
    registry  InstanceRegistry
    gpg       GPGVerifier
    version   string
}

func (s *DashboardService) RenderIndexHTML() (string, error)
```

**VersionService**: simple -- just returns version string. Can stay as a
method on any service or be a standalone function.

#### 5.2.4 Create TaskQueue Adapter

New file: `internal/chief/repository/machinery.go`

Wraps `*machinery.Server` behind the `TaskQueue` interface. This removes
the direct machinery dependency from the usecase layer.

```go
type MachineryTaskQueue struct {
    server *machinery.Server
}

func NewMachineryTaskQueue(server *machinery.Server) *MachineryTaskQueue

func (q *MachineryTaskQueue) QueueBuildAndRepo(taskUUID string, payload []byte) error
func (q *MachineryTaskQueue) QueueISO(taskUUID string, payload []byte) error
func (q *MachineryTaskQueue) GetBuildState(taskUUID string) (string, string, error)
func (q *MachineryTaskQueue) GetISOState(taskUUID string) (string, error)
```

#### 5.2.5 Add Handler-Layer Service Interface

In `cmd/chief/handler.go`, define the interface the delivery layer consumes:

```go
// ChiefService defines the operations available to HTTP handlers.
type ChiefService interface {
    // Submission
    SubmitPackage(submission domain.Submission) (domain.SubmitPayloadResponse, error)
    RetryPipeline(oldTaskUUID string) (domain.SubmitPayloadResponse, error)
    BuildISO(submission domain.ISOSubmission) (domain.SubmitPayloadResponse, error)

    // Upload
    UploadArtifact(id string, file io.Reader) error
    UploadLog(id, logType string, file io.Reader) error
    UploadSubmission(tokenData []byte, blob io.Reader) (string, error)

    // Status
    BuildStatus(uuid string) (domain.BuildStatusResponse, error)
    ISOStatus(uuid string) (string, string, error)

    // Dashboard & Info
    RenderIndexHTML() (string, error)
    GetMaintainers() ([]domain.Maintainer, error)
    ListMaintainersRaw() (string, error)
    GetVersion() string
}
```

Or use a facade that composes the split services if preferred.

#### 5.2.6 Extract HTML to Template

`RenderIndexHTML` (587 lines of string-concatenated HTML/CSS/JS) should use
`html/template`. This:
- Automatically escapes all interpolated values (XSS prevention)
- Separates presentation from data
- Makes the template testable independently

Possible approach:
- Embed template via `//go:embed dashboard.html`
- Dashboard service prepares a `DashboardData` struct
- Template renders the struct

#### 5.2.7 Centralize Status Mapping

Status terminology (`SUCCESS`/`FAILURE` from machinery vs `DONE`/`FAILED`
for pipeline) is scattered across 4+ files. Create a single mapping
function in `internal/chief/domain/`:

```go
func MapMachineryState(state string) string {
    switch state {
    case "SUCCESS": return "DONE"
    case "FAILURE": return "FAILED"
    case "PENDING", "RECEIVED", "STARTED": return "BUILDING"
    default: return "UNKNOWN"
    }
}
```

#### 5.2.8 Duplicate Submission Types

`internal/cli/domain/submission.go` and `internal/chief/domain/submission.go`
share the same wire format. Options:
- **(a)** Keep them separate but document the coupling contract in both files
  with a comment referencing the other.
- **(b)** Create a shared `pkg/irgsh/` package with the wire-format types
  that both CLI and chief import.

Recommendation: **(a)** for now -- the types are not identical (chief has
`TaskUUID`, `Timestamp`), and a shared package adds coupling between
independently deployable components.

---

## 6. Shared Packages

### 6.1 Existing (no changes needed for CLI/chief refactor)

| Package | Purpose | Used By |
|---------|---------|---------|
| `pkg/httputil` | JSON responses, `PostJSONWithRetry`, `HTTPError`, `HTTPStatusError` | chief, cli, notification |
| `pkg/systemutil` | `CmdExec`, `CopyDir`, `MoveFile`, `ReadFileTrimmed`, `WriteFile` | all components |
| `internal/config` | `IrgshConfig` loading and validation | all components |
| `internal/storage` | SQLite `DB`, `JobStore`, `ISOJobStore` | chief (via monitoring) |
| `internal/monitoring` | Worker `Registry`, heartbeat, metrics | chief, builder, repo, iso |
| `internal/notification` | Webhook notifications | builder, repo (not yet chief) |

### 6.2 Outstanding Issues in Shared Packages

| # | Package | Issue | Severity |
|---|---------|-------|----------|
| 1 | `pkg/systemutil` | `CmdExec` logPath unquoted (shell injection) | High |
| 2 | `pkg/systemutil` | Error string capitalized with period | Medium |
| 3 | `internal/monitoring/metrics.go` | Unused `lastTime` variable | High |
| 4 | `internal/monitoring/jobs.go` | Dummy Args in `GetJobStagesFromMachinery` | High |
| 5 | `internal/monitoring/registry.go` | `context.Background()` stored permanently | Medium |
| 6 | `internal/notification` | Hardcoded `LogBaseURL` | Medium |
| 7 | `internal/notification` | `context.Background()` in `SendWebhook` | Low |
| 8 | `internal/config` | `os.Getwd()` error ignored | Low |

---

## 7. Outstanding Issues

### 7.1 In-Scope (CLI + Chief)

| # | File | Issue | Severity | Fix |
|---|------|-------|----------|-----|
| 1 | `internal/cli/usecase/package.go:117` | `http.DefaultClient` no timeout | High | Inject client or add timeout |
| 2 | `internal/cli/repository/chief_client.go:46` | `checkResponse` discards body | High | Read body before error |
| 3 | `internal/cli/repository/chief_client.go:327` | `FetchLog` path not URL-encoded | Low | `url.PathEscape(logPath)` |
| 4 | `internal/cli/usecase/package.go:273` | `genchangesCmd` fragile shell pipeline | Medium | Use `filepath.Glob` in Go |
| 5 | `internal/cli/usecase/package.go:291` | `\|\| true` swallows mkdir errors | Medium | Separate optional xz move |
| 6 | `internal/cli/usecase/package.go:226` | Unquoted packageName in shell | Medium | Add `sq()` wrapping |
| 7 | `cmd/cli/handlers.go` | `context.Background()` everywhere | Low | Wire signal context |
| 8 | `internal/cli/usecase/cli.go` | Exported struct fields | Medium | Make fields unexported |
| 9 | `internal/chief/usecase/chief.go` | 1294-line god object | Critical | Split per section 5.2 |
| 10 | `internal/chief/usecase/chief.go:273` | Version unescaped in HTML | High | `html.EscapeString()` |
| 11 | `cmd/chief/handler.go:26` | `writeUsecaseError` no Content-Type | Medium | Set `application/json` |
| 12 | `cmd/chief/main.go:41` | Config loaded before CLI app | Medium | Move into `app.Action` |
| 13 | `cmd/chief/main.go:159` | No HTTP server timeouts | Medium | Use `http.Server{}` with timeouts |
| 14 | `internal/cli/repository/chief_client.go:116` | Entire blob in memory | Low | Use `io.Pipe()` for streaming |
| 15 | `internal/chief/usecase/chief.go:786` | Build+repo share UUID | Low | Document or separate UUIDs |

### 7.2 Out-of-Scope (builder/repo/iso -- future PRs)

| # | File | Issue | Severity |
|---|------|-------|----------|
| 1 | `cmd/iso/iso.go` | Shell injection via CmdExec | Critical |
| 2 | `cmd/repo/repo.go` | Shell injection via CmdExec | Critical |
| 3 | `cmd/repo/repo.go` | Unchecked json.Unmarshal + panicking type assertions | Critical |
| 4 | `cmd/builder/init.go` | Shell injection + deprecated ioutil | Critical+High |
| 5 | `cmd/builder/main.go:22` | Unused `configPath` variable | High |
| 6 | `cmd/builder/main.go:51` | Duplicate command aliases `"i"` | High |
| 7 | `cmd/repo/main.go:74` | Duplicate command aliases `"i"` | High |
| 8 | `utils/scripts/iso-build.sh` | Shell injection via unquoted vars | High |
| 9 | Builder/repo/iso `main.go` | No HTTP server timeouts | Medium |
| 10 | Builder/repo/iso `main.go` | No graceful shutdown (no SIGTERM handler) | Medium |

### 7.3 Dependency Concerns (informational)

| Package | Issue |
|---------|-------|
| `gopkg.in/src-d/go-git.v4` | Archived. Active fork: `github.com/go-git/go-git/v5` |
| `github.com/inconshreveable/go-update` | Unmaintained (last commit 2016). No signature verification |
| `github.com/hpcloud/tail` | Archived. Maintained fork: `github.com/nxadm/tail` |

---

## 8. Applied Fixes

Commits already on `refactor/cli` addressing previous review findings.

### Security

| Commit | Fix |
|--------|-----|
| `214b28d` | `exec.Command` args in chief gpg.go, storage.go, CLI gpg_signer.go, debian_packager.go |
| `3d7d19f` | Path traversal validation, UploadLog 10MB limit, nil server fatalf, token cleanup |
| `d9085be` | `io.WriteString` over `fmt.Fprintf(w, userString)`, `json.Marshal` over templates |
| `6413a34` | Direct file I/O for WriteLog, preserve MoveFile permissions |
| `fc297e3` | `--no-absolute-names` on tar extraction |
| `34aa7ac` | GPG fingerprint validation before debsign |
| `f79c5ac` | Restrict URL schemes to http/https |
| `42ba1d2` | Shell-quote paths in CLI shell commands |

### Bug Fixes

| Commit | Fix |
|--------|-----|
| `56488e9` | Chief types, BuildISO, BuildStatus, XSS escaping, error handling |
| `18cc8c3` | checkResponse helper, URL encoding, LimitReader, remove unused atomic |
| `3b8a0d2` | tmpDir cleanup, tarball error propagation, token 0600, context-aware HTTP |
| `58f1cb5` | `atomic.Int32` for activeTasks in builder/repo/iso |
| `35d664e` | ErrRepoOrBranchNotFound to entity, errors.Is, promptui.ErrAbort handling |
| `3a31ec8` | Typed HTTPStatusError checks, safeDebianName regex |
| `63a4b7b` | Double WriteHeader fix in ResponseJSON, remove unused DecodeJSON |
| `f72d19a` | Content-Type application/json on chief API responses |
| `27ad4d9` | Orphaned file cleanup on upload write errors |
| `80a9ec1` | Preserve underlying error in config load failures |
| `7e7ad69` | Error return when dpkg-genbuildinfo and debuild both fail |
| `1871730` | Remove dummy Args from BuildStatus/ISOStatus signatures |

### Refactoring

| Commit | Fix |
|--------|-----|
| `8615f18` | Local interfaces in repo layer, remove unused GenChanges/Lintian |

---

## 9. CI & Build

### Current Issues

| # | Issue | Fix |
|---|-------|-----|
| 1 | `Makefile` test target skips `internal/` | Change to `go test -race ./...` |
| 2 | Coverage overwritten per command | Single `go test` invocation with `-coverprofile` |
| 3 | Integration tests gated behind build tag, never run in CI | Add CI job with `-tags integration` or restructure tests |
| 4 | `cp -rf -R` redundant flags | Remove `-R` |
| 5 | CLAUDE.md references removed files (`cache.go`, `fs.go`) | Update directory structure |

### Test Coverage Map

| Package | Has Tests | Coverage |
|---------|-----------|----------|
| `internal/cli/usecase` | config, package, iso, retry | Core flows covered |
| `internal/cli/repository` | config_store, pipeline_store | Storage covered |
| `internal/chief/usecase` | **none** | Needs tests after split |
| `internal/chief/repository` | **none** | Needs tests |
| `cmd/chief` | **none** | Needs handler tests |
| `internal/storage` | jobs, iso_jobs | SQLite covered |
| `pkg/httputil` | response | Helpers covered |

---

## 10. Verification Checklist

Run after every change to ensure nothing breaks.

```bash
# 1. Compile
go build ./...

# 2. Vet
go vet ./...

# 3. Unit tests with race detector
go test -race -count=1 ./...

# 4. Integration tests (requires infrastructure)
go test -tags integration -race ./cmd/builder/ ./cmd/repo/

# 5. Verify binaries are produced
make build
ls -la bin/
```

### Architecture Compliance Checks

- [ ] `internal/cli/domain/` imports only stdlib
- [ ] `internal/chief/domain/` imports only stdlib
- [ ] `internal/cli/usecase/` imports only domain + stdlib + pkg/
- [ ] `internal/chief/usecase/` imports only domain + stdlib + pkg/
- [ ] `internal/cli/repository/` does NOT import usecase (uses local interfaces)
- [ ] `internal/chief/repository/` does NOT import usecase (uses local interfaces)
- [ ] `cmd/cli/handlers.go` defines its own service interface
- [ ] `cmd/chief/handler.go` defines its own service interface
- [ ] All usecase struct fields are unexported
- [ ] No global mutable state in `cmd/` packages (except config loaded in main)
- [ ] All interfaces defined at the consumer side

### Import Direction Verification

```bash
# Should return nothing (no reverse dependencies):
go list -f '{{.ImportPath}}: {{.Imports}}' ./internal/cli/domain/... | grep -E 'usecase|repository'
go list -f '{{.ImportPath}}: {{.Imports}}' ./internal/chief/domain/... | grep -E 'usecase|repository'
go list -f '{{.ImportPath}}: {{.Imports}}' ./internal/cli/usecase/... | grep 'repository'
go list -f '{{.ImportPath}}: {{.Imports}}' ./internal/chief/usecase/... | grep 'repository'
```
