# Post-Merge Follow-Up Specifications

Follow-up work identified during PR #193 review. These are improvements to the
codebase introduced by the refactoring, not regressions.

## Phase 1: HTTP Server WriteTimeout

### Problem

The chief HTTP server (`cmd/chief/main.go:166-171`) sets `ReadHeaderTimeout`,
`ReadTimeout`, and `IdleTimeout` but omits `WriteTimeout`. A slow or malicious
client can hold a connection open indefinitely during response writing, causing
goroutine and file descriptor exhaustion.

### Current code

```go
// cmd/chief/main.go:166-171
return &http.Server{
    Handler:           mux,
    ReadHeaderTimeout: 10 * time.Second,
    ReadTimeout:       15 * time.Second,
    IdleTimeout:       90 * time.Second,
}
```

### Complication

A blanket `WriteTimeout` breaks long-running responses:
- `/artifacts/` and `/logs/` serve large files via `http.FileServer`
- Dashboard rendering queries multiple backends (Redis, SQLite, machinery)

A single `WriteTimeout` value must be large enough for artifact downloads but
still protect against slow-client attacks.

### Solution

Add `WriteTimeout: 60 * time.Second` to the server. This is long enough for
typical artifact downloads and dashboard renders, but prevents indefinite
connection holding.

If artifact downloads need more than 60 seconds in the future, the file-serving
routes should be split to a separate server or use `http.TimeoutHandler` per
route.

### Files to change

- `cmd/chief/main.go:166-171` -- add `WriteTimeout: 60 * time.Second`

### Verification

- `go build ./cmd/chief/`
- `go vet ./cmd/chief/`
- Manual test: start chief, verify dashboard loads, verify artifact download
  still works

---

## Phase 2: Drain Response Bodies in HTTP Client

### Problem

`checkResponse()` in `internal/cli/repository/chief_client.go:46-52` reads up
to 4KB of the error response body. If the error body exceeds 4KB, the remainder
stays in the TCP buffer. When `resp.Body.Close()` is called (by the caller's
`defer`), the connection cannot be reused by `http.Transport`'s connection pool.

Additionally, on the success path, several functions decode JSON from the body
but don't drain any trailing bytes after the JSON object ends. This is less
critical because `json.NewDecoder` typically consumes the full body, but it's
not guaranteed.

### Current code

```go
// internal/cli/repository/chief_client.go:46-52
func checkResponse(resp *http.Response) error {
    if resp.StatusCode >= 200 && resp.StatusCode < 300 {
        return nil
    }
    body, _ := io.ReadAll(io.LimitReader(resp.Body, 4096))
    return httputil.HTTPStatusError{StatusCode: resp.StatusCode, Body: string(body)}
}
```

When `checkResponse` returns an error, the caller does:
```go
defer resp.Body.Close()
if err := checkResponse(resp); err != nil {
    return ..., err
}
```

The body is not fully drained between `checkResponse` and `Close`.

### Solution

After reading the error body in `checkResponse`, drain any remaining bytes:

```go
func checkResponse(resp *http.Response) error {
    if resp.StatusCode >= 200 && resp.StatusCode < 300 {
        return nil
    }
    body, _ := io.ReadAll(io.LimitReader(resp.Body, 4096))
    io.Copy(io.Discard, resp.Body) // drain remainder for connection reuse
    return httputil.HTTPStatusError{StatusCode: resp.StatusCode, Body: string(body)}
}
```

### Files to change

- `internal/cli/repository/chief_client.go:50` -- add drain after LimitReader

### Verification

- `go build ./...`
- `go vet ./...`
- `go test -race ./internal/cli/...`

---

## Phase 3: Fix Broken Error Chains (fmt.Errorf with %w and %v)

### Problem

10 call sites use `fmt.Errorf` with both `%w` and `%v` in ways that break the
error chain or produce redundant output.

**Pattern A -- `fmt.Errorf("%w: %v", sentinel, err)` (8 sites):**
This wraps the sentinel error but formats the underlying error as a string. The
result:
- `errors.Is(result, ErrConfigMissing)` works (good)
- `errors.Is(result, err)` does NOT work (bad -- the underlying error is lost)
- The error message contains the underlying error text, but it's not unwrappable

**Pattern B -- `fmt.Errorf("...%v...%w", errA, errB)` (2 sites):**
In `repo_sync.go`, this wraps one error but loses the other. Both errors are
potentially useful for `errors.Is`/`errors.As` matching.

### Affected locations

| File | Line | Current code |
|------|------|-------------|
| `internal/cli/usecase/package.go` | 36 | `fmt.Errorf("%w: %v", ErrConfigMissing, err)` |
| `internal/cli/usecase/package.go` | 272 | `fmt.Errorf("dpkg-genbuildinfo failed (debuild fallback also failed: %v): %w", shellErr, err)` |
| `internal/cli/usecase/package.go` | 394 | `fmt.Errorf("%w: %v", ErrConfigMissing, err)` |
| `internal/cli/usecase/package.go` | 411 | `fmt.Errorf("%w: %v", ErrConfigMissing, loadErr)` |
| `internal/cli/usecase/iso.go` | 13 | `fmt.Errorf("%w: %v", ErrConfigMissing, err)` |
| `internal/cli/usecase/iso.go` | 52 | `fmt.Errorf("%w: %v", ErrConfigMissing, err)` |
| `internal/cli/usecase/iso.go` | 69 | `fmt.Errorf("%w: %v", ErrConfigMissing, err)` |
| `internal/cli/usecase/retry.go` | 14 | `fmt.Errorf("%w: %v", ErrConfigMissing, err)` |
| `internal/cli/repository/repo_sync.go` | 126 | `fmt.Errorf("failed to acquire lock: %w (close error: %v)", err, closeErr)` |
| `internal/cli/repository/repo_sync.go` | 138 | `fmt.Errorf("failed to unlock: %w (close error: %v)", unlockErr, closeErr)` |

### Solution

**Pattern A fix:** Use `errors.Join` (Go 1.20+) to wrap both errors, or use
`fmt.Errorf` with two `%w` verbs (Go 1.20+):

```go
// Before
fmt.Errorf("%w: %v", ErrConfigMissing, err)

// After (Go 1.20+ supports multiple %w)
fmt.Errorf("%w: %w", ErrConfigMissing, err)
```

This preserves both error chains. `errors.Is(result, ErrConfigMissing)` and
`errors.Is(result, err)` both work.

**Pattern B fix (repo_sync.go):** Same approach:

```go
// Before
fmt.Errorf("failed to acquire lock: %w (close error: %v)", err, closeErr)

// After
fmt.Errorf("failed to acquire lock: %w (close error: %w)", err, closeErr)
```

### Files to change

- `internal/cli/usecase/package.go` -- lines 36, 272, 394, 411
- `internal/cli/usecase/iso.go` -- lines 13, 52, 69
- `internal/cli/usecase/retry.go` -- line 14
- `internal/cli/repository/repo_sync.go` -- lines 126, 138

### Verification

- `go build ./...`
- `go vet ./...`
- `go test -race ./internal/cli/...`
- Verify `errors.Is` works on both wrapped errors in existing tests

---

## Phase 4: Chief Unit Tests

### Problem

The entire `internal/chief/` subtree (15 production files, 1,564 lines) has
zero test files. The clean architecture refactoring made these files testable
(all dependencies are behind interfaces in `ports.go`), but no tests were
written.

### Untested files

**Domain (5 files, 134 lines):**
- `internal/chief/domain/errors.go` -- SafeIDPattern regex, error types
- `internal/chief/domain/job.go` -- job tracking types
- `internal/chief/domain/maintainer.go` -- Maintainer struct
- `internal/chief/domain/status.go` -- DeriveBuildPipelineState, DeriveISOPipelineState, DeriveCurrentStage
- `internal/chief/domain/submission.go` -- Submission, ISOSubmission, SubmitPayloadResponse

**Usecase (7 files, 1,250 lines):**
- `internal/chief/usecase/chief.go` -- ChiefUsecase facade, constructor
- `internal/chief/usecase/dashboard.go` -- DashboardService, view models, template rendering
- `internal/chief/usecase/maintainer.go` -- MaintainerService, GPG key parsing
- `internal/chief/usecase/ports.go` -- interfaces (no logic to test)
- `internal/chief/usecase/status.go` -- StatusService, BuildStatus, ISOStatus
- `internal/chief/usecase/submission.go` -- SubmissionService, SubmitPackage, RetryPipeline, BuildISO
- `internal/chief/usecase/upload.go` -- UploadService, UploadArtifact, UploadLog, UploadSubmission

**Repository (3 files, 180 lines):**
- `internal/chief/repository/gpg.go` -- GPG adapter (hard to unit test, needs gpg binary)
- `internal/chief/repository/machinery.go` -- MachineryTaskQueue adapter (hard to unit test, needs Redis)
- `internal/chief/repository/storage.go` -- Storage adapter (needs filesystem)

### Priority order

Tests should be added in order of value:

1. **domain/status.go** -- pure functions, easy to test, highest correctness
   impact (state derivation drives dashboard display)
2. **domain/errors.go** -- SafeIDPattern regex validation (security-critical)
3. **usecase/submission.go** -- core business logic, needs mock interfaces
4. **usecase/upload.go** -- upload validation, size limits, path safety
5. **usecase/maintainer.go** -- GPG output parsing
6. **usecase/status.go** -- status query delegation
7. **usecase/dashboard.go** -- view model construction, template rendering
8. **usecase/chief.go** -- facade constructor and delegation

### Approach

Create mock implementations of the 6 port interfaces in a `mocks_test.go` file
(following the same pattern as `internal/cli/usecase/mocks_test.go`):

```go
// internal/chief/usecase/mocks_test.go
type mockTaskQueue struct { ... }
type mockGPGVerifier struct { ... }
type mockFileStorage struct { ... }
type mockJobStore struct { ... }
type mockISOJobStore struct { ... }
type mockInstanceRegistry struct { ... }
```

### Files to create

- `internal/chief/domain/status_test.go`
- `internal/chief/domain/errors_test.go`
- `internal/chief/usecase/mocks_test.go`
- `internal/chief/usecase/submission_test.go`
- `internal/chief/usecase/upload_test.go`
- `internal/chief/usecase/maintainer_test.go`
- `internal/chief/usecase/status_test.go`
- `internal/chief/usecase/dashboard_test.go`

### Verification

- `go test -race -v ./internal/chief/...`
- `go test -race -coverprofile=coverage.txt ./internal/chief/...`
- Target: >60% coverage for `internal/chief/` subtree

---

## Phase 5: CLI Test Coverage Gaps

### Problem

The CLI usecase layer has good test coverage for config, package status/log,
ISO, and retry. But several files have no tests:

| File | Lines | Why it matters |
|------|-------|---------------|
| `internal/cli/usecase/update.go` | 50 | Self-update logic, downloads binaries |
| `internal/cli/usecase/cli.go` | 44 | Constructor, but trivial |
| `internal/cli/usecase/errors.go` | 18 | Sentinel errors, trivial |

The `SubmitPackage` workflow in `package.go` (449 lines) has tests for
status/log but the actual submit flow (lines 25-390) is only partially tested.

### Priority order

1. **package.go submit flow** -- the core workflow (GPG signing, tarball
   creation, upload, submission) has the most complex logic and highest risk
2. **update.go** -- downloads and replaces the binary, needs mock tests

### Files to create or extend

- `internal/cli/usecase/package_test.go` -- add tests for SubmitPackage flow
- `internal/cli/usecase/update_test.go` -- new file

### Verification

- `go test -race -v ./internal/cli/usecase/...`
- `go test -race -coverprofile=coverage.txt ./internal/cli/usecase/...`
