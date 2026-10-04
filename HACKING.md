# Development Guide

Use isolated local state and test-only credentials. Development instructions do
not authorize production operations.

## Requirements

- Go 1.25 or later
- Docker
- `build-essential gpg pbuilder debootstrap devscripts curl reprepro`

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

```bash
make redis
make chief
make builder
make repo
```

`make redis` starts a Docker container on the host network. Use it only when that
network exposure and container lifetime are acceptable in the development
environment.

Submit test packages only to this isolated stack. Use test maintainers, keys,
repositories, and artifacts. Do not point development commands at a production
chief, Redis, repository, or worker.

## Privileged and destructive commands

Do not run these as routine setup:

- `make builder-init` installs host packages as root, removes matching
  `/var/cache/pbuilder/base*` files, writes `/root/.pbuilderrc`, and builds the
  `pbocker` image.
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
