# Contributing to IRGSH-GO

Thank you for your interest in contributing to IRGSH-GO. This document provides guidelines and explains key design decisions to help you understand the project's philosophy.

## Design Philosophy: Shell Execution Over Native Implementation

One of the most notable design choices in IRGSH-GO is the extensive use of shell command execution (`exec.Command`) rather than implementing functionality natively in Go. This is an intentional architectural decision aligned with BlankOn's core mission.

### Why Shell Execution?

**Educational Transparency**

BlankOn Linux's primary mission is to leverage and develop people's technical capabilities. IRGSH-GO serves not only as a build system but also as an educational platform. By executing standard Unix/Linux commands explicitly, we create comprehensive logs that serve as learning materials for:

- New contributors learning the Debian packaging process
- System administrators understanding the build pipeline
- Developers troubleshooting build failures
- Anyone curious about what happens behind the scenes

**Readable and Reproducible Logs**

Every shell command executed by IRGSH workers is logged with an accompanying explanation prefixed with `###`. For example:

```
### Fetching the submission tarball from chief
curl -v -o /var/lib/irgsh/builder/artifacts/job-id/debuild.tar.gz https://chief/submissions/job-id.tar.gz

### Building the package
docker run -v /var/lib/irgsh/builder/artifacts/job-id:/tmp/build --privileged=true pbocker bash -c /build.sh

### Injecting the deb files from artifact to the repository
reprepro -v -v -v --nothingiserror --component main includedeb peyem /var/lib/irgsh/repo/artifacts/job-id/*.deb
```

This approach allows users to:

1. **Learn by observation**: Understand each step of the packaging process
2. **Reproduce manually**: Copy and run commands locally for debugging
3. **Diagnose failures**: Identify exactly which command failed and why
4. **Build expertise**: Gain practical knowledge of tools like `reprepro`, `pbuilder`, `dpkg`, and `gpg`

**Standard Tooling**

The commands used (`curl`, `tar`, `gpg`, `reprepro`, `docker`) are industry-standard tools that packagers will encounter throughout their careers. Exposing these commands directly helps contributors develop transferable skills.

### When Native Go Implementation is Preferred

There are specific cases where native Go implementation is more appropriate:

**Progress Indicators for Long-Running Operations**

For operations where user feedback is critical, such as uploading large tarballs in `irgsh-cli`, we implement functionality natively in Go. This allows us to provide real-time progress indicators that keep packagers informed about upload state, which would not be possible with a simple `curl` shell execution.

**Performance-Critical Operations**

When processing large files in memory-constrained environments (e.g., streaming file uploads to prevent OOM), native Go implementations with proper streaming support are preferred.

**Complex Error Handling**

Operations requiring sophisticated error handling, retries, or state management may benefit from native implementation.

## Code Style Guidelines

### Shell Commands

When adding new shell command executions:

1. Always provide a descriptive explanation for logging
2. Use `systemutil.CmdExec()` which handles logging automatically
3. Keep commands readable and avoid overly complex one-liners
4. Quote paths properly to handle spaces

```go
cmdStr := fmt.Sprintf("reprepro -v include %s %s/*.changes",
    distCodename,
    artifactPath,
)
_, err = systemutil.CmdExec(
    cmdStr,
    "Injecting the changes file from artifact to the repository",
    logPath,
)
```

### Error Handling

- Always check and handle errors appropriately
- Log errors with sufficient context for debugging
- Send notifications on job completion (success or failure)

### Configuration

- Add new configuration fields to `internal/config/config.go`
- Update `utils/config.yaml` with examples and comments
- Document required vs optional fields

## Getting Started

1. Fork the repository
2. Create a feature branch
3. Make your changes following the guidelines above
4. Run tests with `make test`
5. Submit a pull request

## Questions?

If you have questions about contributing, please open an issue or reach out to the BlankOn developer community at blankon-dev@googlegroups.com.
