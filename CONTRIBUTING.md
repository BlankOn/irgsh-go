# Contributing to IRGSH-GO

IRGSH uses standard Debian and Unix tools because their output is useful to
package maintainers, operators, and contributors. Keep that observability without
treating shell text as a safe execution interface.

Read [AGENTS.md](AGENTS.md) for contribution scope and merge gates,
[DESIGN.md](DESIGN.md) for architecture, and [HACKING.md](HACKING.md) before
running local services or initialization commands.

## Command execution

Prefer direct process arguments for dynamic values:

```go
cmd := exec.CommandContext(
	ctx,
	"reprepro",
	"-V",
	"--component", component,
	"includedeb", suite, debPath,
)
```

Validate enumerated values such as suites, components, branches, architectures,
and job identifiers before any filesystem or process operation. Use option
separators where the command supports them. A value from an HTTP request, task
payload, repository, archive, configuration file, or environment remains
untrusted until the affected boundary validates it.

Use a shell only when its syntax is required for a pipeline, redirection, glob,
or compound operation. Quote every dynamic value with the established helper,
and never concatenate unvalidated input into shell text.

`systemutil.CmdExec` and `systemutil.CmdExecContext` execute a complete string
through Bash and log that string when a log path is supplied. Use them only when
shell semantics are necessary and all dynamic values are validated and quoted.
They are not mandatory for every external command.

Keep useful command-level logs:

- Describe the operation in IRGSH terms.
- Record the executable and safe arguments needed to reproduce it.
- Preserve exit status and relevant standard output and error output.
- Never log credentials, authorization headers, signing material, bot tokens, or
  secret-bearing URLs. Pass secrets through a protected file, standard input, or
  environment as supported by the tool, while logging a redacted invocation.

Prefer native Go when it gives a smaller or safer implementation, including
streaming large data, atomic file replacement, structured parsing, bounded
queries, or error handling that shell composition would obscure.

## Error handling and state

- Return errors with the failed operation and enough sanitized context to act on
  them.
- Preserve the original error for inspection when practical.
- Do not report success after a required command, upload, publication, or state
  transition fails.
- Do not publish partial or stale artifacts.
- Keep cancellation and cleanup within the job's owned workspace. Never interrupt
  an active reprepro transaction as generic cleanup.
- Send completion notifications only after the authoritative result is known.

## Tests

- Add the smallest focused test that proves changed non-trivial behavior.
- Test meaningful outcomes, relevant invalid input, boundaries, and failure paths.
- Use temporary directories, isolated services, and test-only keys.
- Do not add unconditional skips or weaken assertions.
- Keep privileged and destructive integration checks opt-in and document their
  environment and exact result.
- Run `go vet ./...`, `go test -race ./...`, and `make build` before requesting a
  routine merge. Record the tested commit and actual results in the pull request.

## Pull requests

Use `.github/pull_request_template.md`. Keep `## Summary` current, include a
standalone fully qualified closing reference only for work the pull request
finishes, and put actual verification under `## Test plan`. Follow every gate in
[AGENTS.md](AGENTS.md); a mergeable pull request is not necessarily ready.
