# Native component deployment

This procedure deploys `chief`, `builder`, `repo`, or `iso` from an attested
GitHub release. It preserves `/etc/irgsh`, `/var/lib/irgsh`, and the active set
of systemd units. The CLI artifact is not installed by this procedure.

Automatic deployment is supported only when wire formats, configuration, and
persisted schemas remain backward-compatible. Use a declared maintenance
procedure for an incompatible migration: quiesce submissions, back up state,
stop all affected components, run the coordinated migration, validate the
whole cluster, and keep a migration-specific rollback plan.

## Repository prerequisite

Before publishing a production release, [enable immutable releases](https://docs.github.com/en/code-security/how-tos/secure-your-supply-chain/establish-provenance-and-integrity/prevent-release-changes)
under **Settings > General > Releases > Enable release immutability** in
`BlankOn/irgsh-go`. Published immutable assets and tags cannot be replaced.

## One-time host provisioning

Run provisioning in a maintenance window with no active jobs. The examples
use Debian package names and a host that runs repo instances for `verbeek` and
`rani`; adapt unit names, config paths, and loopback ports to the units already
defined on that host.

Builder hosts must complete the [rootless builder prerequisites](rootless-builder.md)
before selecting the new binary. Each builder instance needs its own account,
subordinate IDs, workdir, initialized base, and component-specific config.

Install the host tools:

```sh
sudo apt-get update
sudo apt-get install -y gh curl tar coreutils util-linux openssh-server systemd
gh attestation verify --help >/dev/null
```

Create a locked trigger account. It needs `/bin/sh` because `sshd` invokes the
forced command through the account shell, but its only authorized key is
restricted later. Do not add it to the `irgsh` group.

```sh
sudo useradd --system --home-dir /var/empty/irgsh-deploy --shell /bin/sh irgsh-deploy
sudo passwd --lock irgsh-deploy
sudo install -d -o root -g root -m 0755 /var/empty/irgsh-deploy
sudo install -d -o root -g root -m 0755 /var/empty/irgsh-deploy/.ssh
```

Install the two programs from a reviewed checkout:

```sh
sudo install -d -o root -g root -m 0755 /usr/local/sbin /usr/local/libexec
sudo install -o root -g root -m 0755 utils/deploy/irgsh-deploy /usr/local/sbin/irgsh-deploy
sudo install -o root -g root -m 0755 utils/deploy/irgsh-deploy-request /usr/local/libexec/irgsh-deploy-request
```

Create a bootstrap release for each component present on the host before
changing its unit. This repo example keeps the binary's current version and
arguments while moving only the executable path:

```sh
sudo install -d -o root -g root -m 0755 /opt/irgsh/releases/repo/bootstrap/bin
sudo install -o root -g root -m 0755 /usr/bin/irgsh-repo /opt/irgsh/releases/repo/bootstrap/bin/irgsh-repo
sudo install -d -o root -g root -m 0755 /opt/irgsh/current
sudo ln -s /opt/irgsh/releases/repo/bootstrap /opt/irgsh/current/repo
```

Repeat that layout for every local component. ISO also needs its helper:

```sh
sudo install -d -o root -g root -m 0755 /opt/irgsh/releases/iso/bootstrap/bin
sudo install -d -o root -g root -m 0755 /opt/irgsh/releases/iso/bootstrap/share
sudo install -d -o root -g root -m 0755 /opt/irgsh/current
sudo install -o root -g root -m 0755 /usr/bin/irgsh-iso /opt/irgsh/releases/iso/bootstrap/bin/irgsh-iso
sudo install -o root -g root -m 0755 /usr/share/irgsh/iso-build.sh /opt/irgsh/releases/iso/bootstrap/share/iso-build.sh
sudo ln -s /opt/irgsh/releases/iso/bootstrap /opt/irgsh/current/iso
```

If a `current` path already exists, inspect it instead of replacing it. There
must be one absolute symlink per component and its target must be below
`/opt/irgsh/releases/<component>/`.

Create a drop-in for each concrete unit. Clear `ExecStart` before setting the
release path, and retain every existing argument. A repo instance looks like:

```sh
sudo install -d -o root -g root -m 0755 /etc/systemd/system/irgsh-repo@verbeek.service.d
printf '%s\n' '[Service]' 'ExecStart=' 'ExecStart=/opt/irgsh/current/repo/bin/irgsh-repo -c /etc/irgsh/repo-verbeek.yaml' 'TimeoutStopSec=infinity' | sudo tee /etc/systemd/system/irgsh-repo@verbeek.service.d/deploy.conf >/dev/null
sudo chown root:root /etc/systemd/system/irgsh-repo@verbeek.service.d/deploy.conf
sudo chmod 0644 /etc/systemd/system/irgsh-repo@verbeek.service.d/deploy.conf
```

Use these executable paths and stop deadlines in the other drop-ins:

| Component | `ExecStart` executable | `TimeoutStopSec` |
| --- | --- | --- |
| chief | `/opt/irgsh/current/chief/bin/irgsh-chief` | `45s` |
| builder | `/opt/irgsh/current/builder/bin/irgsh-builder` | `infinity` |
| repo | `/opt/irgsh/current/repo/bin/irgsh-repo` | `infinity` |
| iso | `/opt/irgsh/current/iso/bin/irgsh-iso` | `infinity` |

For `irgsh-builder@verbeek.service`, preserve the selected config and dedicated
identity in its drop-in:

```ini
[Service]
User=irgsh-builder-verbeek
Group=irgsh-builder-verbeek
WorkingDirectory=/var/lib/irgsh/builder-verbeek
ExecStart=
ExecStart=/opt/irgsh/current/builder/bin/irgsh-builder -c /etc/irgsh/builder-verbeek.yaml
TimeoutStopSec=infinity
```

The ISO drop-in must also select the release helper:

```ini
[Service]
Environment="IRGSH_ISO_SCRIPT=/opt/irgsh/current/iso/share/iso-build.sh"
ExecStart=
ExecStart=/opt/irgsh/current/iso/bin/irgsh-iso
TimeoutStopSec=infinity
```

Create one root-owned target map per local component. Each row is a
host-defined unit and its own loopback version endpoint. CI can select the
component but cannot add a unit or URL.

```sh
sudo install -d -o root -g root -m 0755 /etc/irgsh/deploy-targets
printf '%s\n' 'irgsh-repo@verbeek.service http://127.0.0.1:8182/api/v1/version' 'irgsh-repo@rani.service http://127.0.0.1:8282/api/v1/version' | sudo tee /etc/irgsh/deploy-targets/repo >/dev/null
sudo chown root:root /etc/irgsh/deploy-targets/repo
sudo chmod 0600 /etc/irgsh/deploy-targets/repo
```

Only `irgsh-<component>.service` and
`irgsh-<component>@<instance>.service` names are accepted. URLs must be
`http://127.0.0.1:<port>/api/v1/version` or
`http://[::1]:<port>/api/v1/version`. List every local instance, including
instances that are normally inactive; inactive units remain inactive during a
deployment.

Allow the trigger account to run exactly the no-argument root helper. The
empty quoted sudoers argument is required; omitting it would allow arbitrary
arguments.

```sh
printf '%s\n' 'irgsh-deploy ALL=(root) NOPASSWD: /usr/local/sbin/irgsh-deploy ""' | sudo tee /etc/sudoers.d/irgsh-deploy >/dev/null
sudo chown root:root /etc/sudoers.d/irgsh-deploy
sudo chmod 0440 /etc/sudoers.d/irgsh-deploy
sudo visudo -cf /etc/sudoers.d/irgsh-deploy
```

Install a separate public key for each GitHub Environment. Keep the file and
directory root-owned so the trigger user cannot replace the forced command.

```text
restrict,command="/usr/local/libexec/irgsh-deploy-request" ssh-ed25519 AAAAC3NzaC1lZDI1NTE5AAAA... irgsh-github-staging
```

```sh
sudo install -o root -g root -m 0644 authorized_keys /var/empty/irgsh-deploy/.ssh/authorized_keys
```

Reload systemd, restart each provisioned unit one at a time, and check its
local endpoint before continuing:

```sh
sudo systemctl daemon-reload
sudo systemctl restart irgsh-repo@verbeek.service
sudo systemctl is-active irgsh-repo@verbeek.service
curl --fail --silent http://127.0.0.1:8182/api/v1/version
```

Finish with permission checks. The second command must print nothing:

```sh
sudo stat -c '%U %G %a %n' /usr/local/sbin/irgsh-deploy /usr/local/libexec/irgsh-deploy-request /etc/irgsh/deploy-targets/repo
sudo -u irgsh-deploy find /etc/irgsh /var/lib/irgsh /opt/irgsh -writable -print
```

## Deploy and roll back

The accepted remote command is exactly:

```text
deploy <chief|builder|repo|iso> v<major>.<minor>.<patch>
```

Invoke it with the environment's dedicated key and pinned host key:

```sh
ssh -i irgsh-staging -o IdentitiesOnly=yes -o StrictHostKeyChecking=yes -o UserKnownHostsFile=known_hosts irgsh-deploy@staging.example.org 'deploy repo v2.3.2'
```

For local maintenance, send the same two validated fields to the root helper;
do not pass command-line arguments:

```sh
printf 'repo\nv2.3.2\n' | sudo /usr/local/sbin/irgsh-deploy
```

Rollback is an ordinary deployment of an older attested version:

```sh
ssh -i irgsh-staging -o IdentitiesOnly=yes -o StrictHostKeyChecking=yes -o UserKnownHostsFile=known_hosts irgsh-deploy@staging.example.org 'deploy repo v2.3.1'
```

Do not repoint `current` manually. The helper verifies both attestations, the
checksum, archive members, binary version, unit health, and rollback health.

## Drain timeout

A drain timeout returns failure before changing `current`. It deliberately
does not kill the worker, and systemd may still be waiting for its active job.
Inspect the unit and deployment log without restarting or sending a signal:

```sh
sudo systemctl status irgsh-repo@verbeek.service
sudo journalctl -t irgsh-deploy --since '30 minutes ago'
sudo journalctl -u irgsh-repo@verbeek.service --since '30 minutes ago'
```

Resolve the job-side cause and wait for the unit to become inactive. Because
the old symlink is still selected, start the previously active unit and verify
the old endpoint before retrying the deployment:

```sh
sudo systemctl start irgsh-repo@verbeek.service
sudo systemctl is-active irgsh-repo@verbeek.service
curl --fail --silent http://127.0.0.1:8182/api/v1/version
```

Never use `systemctl kill`, `kill -9`, or another forced stop to clear a repo,
builder, or ISO deployment timeout.

## Configuration and state evidence

For a staging drill, take content inventories while the system is idle, deploy
or roll back, then compare them. An empty diff confirms that the deployment did
not modify configuration or service state:

```sh
sudo find /etc/irgsh /var/lib/irgsh -xdev -type f -exec sha256sum {} + | LC_ALL=C sort > /tmp/irgsh-state.before
```

```sh
sudo find /etc/irgsh /var/lib/irgsh -xdev -type f -exec sha256sum {} + | LC_ALL=C sort > /tmp/irgsh-state.after
diff -u /tmp/irgsh-state.before /tmp/irgsh-state.after
```

Normal live jobs can update `/var/lib/irgsh`, so collect this evidence only in
the controlled idle window used for the deployment drill.

## GitHub Environments

Create `staging` and `production` Environments in the repository. Store these
secrets in each Environment, never as repository-level secrets:

| Secret | Value |
| --- | --- |
| `DEPLOY_HOST` | One trusted DNS name or IP address |
| `DEPLOY_USER` | `irgsh-deploy` |
| `DEPLOY_SSH_KEY` | The Environment's private key |
| `DEPLOY_KNOWN_HOSTS` | Host-key lines obtained and verified out of band |

Use different SSH key pairs for staging and production and install each public
key only on its corresponding host. Pin the complete host-key material in
`DEPLOY_KNOWN_HOSTS`; do not discover it with `ssh-keyscan` during a workflow.

Protect the production Environment with required reviewers, prevent a person
who started the run from approving it, and restrict deployment refs to the
protected `main` branch and stable `v*` release tags. Apply the same ref
restriction to staging; staging reviewers may be less restrictive. Keep all
four deployment secrets at Environment scope so pull request workflows cannot
read them.

The workflow has no repository permissions, checkout, or general remote shell.
It sends only the validated component and stable version to the host's forced
command. Start a deployment from the Actions UI or with:

```sh
gh workflow run deploy.yaml --ref main -f environment=staging -f component=repo -f version=v2.3.2
```

Use the same workflow for production after staging evidence is approved:

```sh
gh workflow run deploy.yaml --ref main -f environment=production -f component=repo -f version=v2.3.2
```

## Staging deployment record

Copy this template into the change record for every candidate release. Link
logs instead of pasting private keys, tokens, hostnames, or other secrets.

```text
Release version:
Source commit:
Workflow run URL and ID:
Component:
Old endpoint versions:
New endpoint versions:
/etc/irgsh checksum before:
/etc/irgsh checksum after:
/var/lib/irgsh checksum before:
/var/lib/irgsh checksum after:
Corrupt artifact rejection:
Concurrent request rejection:
Drain-timeout result:
Health-failure injection:
Automatic rollback result:
Explicit previous-version deployment result:
Approver:
Approval time:
```

Do not approve production until the record shows a successful upgrade, all
requested failure cases, healthy automatic rollback, a successful explicit
deployment of the previous version, and unchanged configuration and state.
