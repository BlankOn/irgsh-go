# Development Guide

Use isolated local state and test-only credentials. Development instructions do
not authorize production operations.

## Requirements

- Go 1.25 or later
- `build-essential` for compilation and race tests
- For local services: Redis, `gpg devscripts curl reprepro`
- For native package builds: `sbuild >= 0.87.0`, `mmdebstrap uidmap dpkg-dev ca-certificates`,
  unprivileged user namespaces, and a dedicated builder account with valid subordinate IDs
- Docker only for the containerized Redis helper and E2E chief/repo services

Use the [rootless builder guide](docs/rootless-builder.md) for host provisioning
and evidence. The Gitpod image supports compilation and routine tests; it does
not provision native namespace builds.

Clone the repository, then work from its root:

```bash
git clone git@github.com:BlankOn/irgsh-go.git
cd irgsh-go
```

## Routine checks

These commands do not initialize a repository or install a service:

```bash
make test
make build
```

`make test` writes `coverage.txt`. Run `make coverage` only when you want to open
the HTML coverage report.

## Local credentials and configuration

Use a dedicated test GPG key. Never use a production signing key or copy private
key material into the repository, logs, screenshots, or pull request evidence.

The local Make targets read `GPG_KEY` from the ignored `.env` file. Targets that
depend on `load-gpg` update the tracked `utils/config.yaml` with the key identity,
so review that file before committing changes.

```text
GPG_KEY=<test-key-fingerprint>
```

Initialize the CLI against the local chief with:

```bash
make client
```

## Local services

`make chief`, `make builder`, and `make repo` build and run one component with
`DEV=1`. Development work directories are redirected from `/var/lib/irgsh` into
`./tmp`. Each component still requires Redis and its own valid configuration.
Run the builder only under its provisioned dedicated account with a separate
workdir and no runtime socket access. For that account's component config, use
the direct commands in the [builder guide](docs/rootless-builder.md#base-initialization-and-update).

```bash
make redis
make chief
make repo
```

`make redis` starts a Docker container on the host network. Use it only when that
network exposure and container lifetime are acceptable in the development
environment.

Submit test packages only to this isolated stack. Use test maintainers, keys,
repositories, and artifacts. Do not point development commands at a production
chief, Redis, repository, or worker.

## Native builder and E2E

`make builder-init` builds the worker and runs `init-base` with `DEV=1` as the
current account. Provision a dedicated account and mappings first, and ensure
its workdir and parent directories allow namespace access. Initialization and
`update-base` create an unprivileged mmdebstrap tarball and replace the selected
base atomically. Jobs preserve immutable source inputs and a pinned base;
retries use fresh output and temporary directories. Cancellation signals the
process group directly without sudo.

The manual/nightly E2E workflow provisions a disposable `irgsh-builder-e2e`
account with its own subordinate IDs and no Docker group. The runner orchestrates
chief/repo/Redis containers; the dedicated account initializes and runs the
native builder. `./e2e/run.sh` requires that provisioned environment. Passing
routine tests does not establish native E2E or target-host evidence; record each
result using the [builder evidence procedure](docs/rootless-builder.md#target-host-evidence-record).

## Privileged and destructive commands

Do not run these as routine setup:

- `make repo-init` confirms interactively, then removes and recreates the
  configured normal and experimental distributions. The standard target uses
  `DEV=1`, but a directly invoked production binary uses its configured workdir.
- `make iso` writes the ISO build script under `/usr/share/irgsh` with `sudo` and
  runs live-build from a persistent work tree.
- `make build-install` invokes the installer and changes systemd services.
- `make deb` removes the local Debian build directory with `sudo` before building.

Before running one, resolve and inspect every target path and configuration. Use
a disposable host or purpose-built isolated environment, test-only credentials,
and no production mounts. Record unavailable checks as not run, not passed.

## Production operations

Installing or updating services, initializing a production repository, changing
credentials or permissions, publishing packages, deploying software, and
resetting data require explicit operator authorization and a separate reviewed
procedure. Do not infer that authority from development access or a merge.

See [DESIGN.md](DESIGN.md) for privilege boundaries and persistent state.
