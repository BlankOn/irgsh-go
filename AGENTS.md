# AI Contribution Contract

This file is the canonical contract for AI-assisted work in IRGSH. `CLAUDE.md`
imports it; do not duplicate these rules there. Read [DESIGN.md](DESIGN.md) for
architecture, [CONTRIBUTING.md](CONTRIBUTING.md) for engineering guidance, and
[HACKING.md](HACKING.md) before running development or initialization commands.

## Descriptions and scope

- Treat the active issue or pull request as the sole task scope. Read its complete
  description, discussion, linked work, and affected review threads before editing.
- Keep issue and pull request descriptions current. They are the single source
  of truth for accepted scope, decisions, status, implementation summary, and
  verification, not append-only journals.
- Re-read a description immediately before updating it. Preserve concurrent
  human edits and relevant attribution. Never rewrite or delete human discussion.
- Update descriptions only. Never post issue comments, pull request conversation
  comments, inline review comments, administrative replies, progress notes, test
  receipts, completion reports, or merge receipts.
- Read the full affected flow before editing: callers, inputs, validation, state
  changes, persistence, outputs, errors, cancellation, and tests.
- Do not implement unrelated findings or silently enlarge scope. Agree on a
  revised active issue before materially expanding the change.
- Use fully qualified references such as `BlankOn/irgsh-go#230`. Use existing
  labels only. Do not reassign work, create labels, or rewrite shared history.

## Engineering

- Prefer deletion, existing code, the standard library, native platform
  features, and existing dependencies, in that order.
- Do not add speculative abstractions, an ORM, a generic storage interface, a
  queue, a frontend, deployment machinery, compatibility layers, or configuration
  without a concrete requirement in the active issue.
- Do not edit generated files directly.
- Add no code comments by default. Preserve required legal text, shebangs,
  build/embed/generate and linter directives, generated script content, runtime
  logs, and test fixtures. Justify any exceptional explanatory comment in the
  pull request description.
- Validate trust boundaries. Preserve authoritative sources and revisions, Git
  ancestry, exact identifiers, versions and checksums, atomicity at the affected
  storage boundary, error handling, and bounded filtering, ordering, limits, and
  pagination. Apply only the terms relevant to the affected IRGSH flow.
- Keep repository prose concise and in normal English. Maintain one architecture
  document at the two-to-four-page convention stated in `DESIGN.md`; do not create
  an architecture document for each task.
- Ship focused runnable tests with non-trivial behavior. Cover meaningful
  outcomes and relevant negative or boundary cases. Do not weaken assertions,
  remove useful regressions, add unconditional skips to make a check green, or
  defer component coverage to an umbrella integration issue.
- Report failed, skipped, unavailable, and unrun checks honestly. None counts as
  a pass.
- Never put credentials, tokens, private keys, decrypted secrets, or production
  data in prompts, logs, commits, screenshots, fixtures, or evidence. Use
  isolated state and test-only credentials. Record sanitized commands, results,
  and the tested revision.

## Git and pull requests

- Start an issue branch from the default branch and name it `<issue>-<slug>`.
- Keep each commit and pull request self-contained. Use a short imperative commit
  subject. Do not impose an arbitrary line-count target at the cost of correctness.
- A pull request description must contain `## Summary`, a standalone fully
  qualified `Closes`, `Fixes`, or `Resolves` reference, and `## Test plan`.
- Use a closing reference only when the pull request completes that issue. Use a
  non-closing `Related:` reference for partial tracking work, and agree on its
  properly scoped active issue before implementation. Repeat the closing keyword
  for every issue genuinely completed.
- Record actual results, the tested commit, and necessary exceptions in those
  sections. Never present intended commands as completed evidence.
- Put closing references in the pull request body, not only in commit messages.
- Do not force-push the default branch. Do not use rebase merge. Do not merge a
  whole stack or adopt automatic assignment behavior without authorization.

This contract supersedes any external workflow that requires comments,
administrative replies, automatic assignment, arbitrary pull request sizing, or
whole-stack merging. Those behaviors are not part of this repository's process.

## Routine merge gates

A routine merge is authorized only when every applicable gate passes. An unknown
required result blocks the merge. Explain why a check is inapplicable; never
invent a pass.

1. **Scope:** The affected flow is understood, the diff is limited to the active
   issue, and every claimed acceptance criterion is satisfied.
2. **Correctness and security:** Relevant validation, exact-value preservation,
   ancestry or provenance, atomicity, error paths, query bounds, and credential
   handling are verified. No speculative infrastructure or unjustified comment
   exception is present.
3. **Tests:** Changed non-trivial behavior has focused tests and runnable
   evidence. Required integration checks pass. A skipped test is not evidence.
4. **CI and review:** Required checks pass for the revision being merged,
   repository review requirements are met, and no blocking feedback remains.
   Read the latest review verdict against current HEAD. Fix P1 and P2 findings in
   code, summarize review outcomes in the pull request description without
   replies or fabricated approval, and re-request review after substantive
   changes invalidate an earlier verdict.
5. **Description:** The pull request has a current summary, complete closing
   reference, actual test results, tested commit, and justified exceptions. The
   issue description reflects the latest accepted scope and status.
6. **Merge and verification:** Recheck HEAD, base, and merge conditions immediately
   before merging. Use an expected-head guard when available, an approved merge
   method, and no administrative bypass.

After merge, verify the landed revision and issue state. Update descriptions
without comments. Remove only branches owned by the completed work and no longer
needed.

Merge authorization does not authorize deployment, publication, credential or
permission changes, data resets, unrelated pull requests, or weaker gates. A
policy-changing pull request needs maintainer approval under the policy already
in force; an AI must not use its proposed rules to approve itself.

## Current enforcement map

Recheck mutable GitHub settings before each merge. At the 2026-09-23 audit:

| Gate | Existing evidence or enforcement | Gap requiring explicit verification |
| --- | --- | --- |
| Scope | Pull request summary, closing reference, and human review | No automated scope check |
| Correctness and security | Focused tests and review evidence | No general automated trust-boundary check |
| Tests | Pull request workflow runs `go vet ./...` and `go test -race ./...`; `build-devel` builds after `test` | Required component or integration checks must be recorded separately |
| CI and review | GitHub Actions reports checks and review state | `main guard` does not require checks, approvals, or resolved threads |
| Description | Pull request template and manual review | GitHub does not enforce body completeness |
| Merge and verification | `main guard` requires a pull request and blocks deletion and force-push | Current HEAD, allowed merge method, landed revision, and issue closure remain manual |

Do not change rulesets or other repository settings unless that operation is
separately authorized. Broad CI/CD modernization belongs to
`BlankOn/irgsh-go#229`.

## Verification commands

Use the smallest relevant set, then run the repository gates before completion:

```bash
go vet ./...
go test -race ./...
make build
```

Run destructive, privileged, integration, or production checks only in the
isolated environment documented in [HACKING.md](HACKING.md). Never treat an
unavailable environment as a passing result.
