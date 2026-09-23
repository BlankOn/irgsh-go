# Native rootless builder operations

## Supported host

Use a native Linux host with unprivileged user namespaces and `sbuild >= 0.87.0`
supporting `--chroot-mode=unshare`, plus `mmdebstrap` supporting `--mode=unshare`. Builds use
the host's native architecture; cross builds and foreign-architecture emulation
are not supported. Native results do not establish rootless-container support.

The generated configuration disables automatic base creation with
`$unshare_mmdebstrap_auto_create`, introduced in
[sbuild 0.87.0](https://lists.debian.org/debian-backports-changes/2024/12/msg00093.html).
On Ubuntu 24.04, follow the
[official backports setup](https://ubuntu.com/project/docs/contributors/setup/set-up-for-ubuntu-development/)
to install a supported version. The builder preflight and manual installer
reject unsupported versions.

Package installation, account creation, subordinate-ID allocation, and resource
limits are administrator operations. Base creation and recurring builds run as
the dedicated builder account. It must have no `root`, `sudo`, or `docker` group
membership, signing keys, deployment credentials, or runtime socket access.
`newuidmap` and `newgidmap` are the only expected set-ID helpers. Do not set
`NoNewPrivileges=true` or strip these helpers' set-ID bits.

Run first validation on an isolated host using test-only credentials and state.
Record the host evidence below before admitting production jobs. Host security
policy must allow the namespace operations; an installed package is not evidence
that its unshare backend works on that host.

## Default account provisioning

Install the host dependencies:

```sh
sudo apt-get install sbuild mmdebstrap uidmap dpkg-dev devscripts ca-certificates
```

On Ubuntu 24.04, use the enabled backports pocket:

```sh
sudo apt-get install -t noble-backports sbuild mmdebstrap uidmap dpkg-dev devscripts ca-certificates
```

The Debian package and installer create `irgsh-builder` with primary group
`irgsh-builder`, home `/var/lib/irgsh/builder`, a disabled password, and
`/usr/sbin/nologin`. Its only added supplementary group is `irgsh`, for config
read access. Component-release hosts without those accounts can provision them:

```sh
getent group irgsh >/dev/null || sudo addgroup --system irgsh
getent group irgsh-builder >/dev/null || sudo addgroup --system irgsh-builder
getent passwd irgsh-builder >/dev/null || sudo adduser --system --home /var/lib/irgsh/builder --no-create-home --ingroup irgsh-builder --disabled-password --shell /usr/sbin/nologin --gecos 'IRGSH Builder' irgsh-builder
sudo adduser irgsh-builder irgsh
sudo install -d -o irgsh-builder -g irgsh-builder -m 0755 /var/lib/irgsh/builder
id irgsh-builder
```

Inspect existing accounts before using them; provisioning does not remove their
groups. Parent directories must be traversable by the subordinate namespace
users. Keep the builder workdir at mode `0755`; do not put it beneath a private
operator home directory.

Create `/etc/irgsh/builder-verbeek.yaml` with the environment's Redis URL,
`chief.address`, and `builder` settings from [the config template](../utils/config.yaml).
Set `builder.workdir` to `/var/lib/irgsh/builder` and `builder.dist_codename` to
`verbeek`. Include no repo signing configuration. Preserve existing config;
review and install a component-specific copy, owned by `root:irgsh`, mode `0640`,
inside `/etc/irgsh` owned by `root:irgsh`, mode `0750`.

## Subordinate-ID allocation

Inventory every existing mapping before choosing ranges:

```sh
sudo cat /etc/subuid /etc/subgid
```

Replace `START-END` below with an administrator-selected decimal range.
`END - START + 1` must be at least 65536. Neither range may overlap an existing
entry in its respective `/etc/subuid` or `/etc/subgid` file, host account IDs,
or another builder's allocation. Each file needs an exact account-name row;
numeric UID aliases alone do not satisfy the builder's preflight.

```sh
sudo usermod --add-subuids START-END --add-subgids START-END irgsh-builder
```

The package and installer never allocate these ranges automatically. Preserve
existing valid allocations across upgrades and while any namespace is active.

## Base initialization and update

Use the same account, binary, config, and workdir as the service:

```sh
sudo -u irgsh-builder irgsh-builder -c /etc/irgsh/builder-verbeek.yaml init-base
sudo -u irgsh-builder irgsh-builder -c /etc/irgsh/builder-verbeek.yaml update-base
```

For a deployment-managed service, invoke
`/opt/irgsh/current/builder/bin/irgsh-builder` explicitly instead of relying on
`PATH`. Neither operation needs root. Both rebuild into a temporary tarball and
atomically replace the selected base only after success. A concurrent rebuild
for the same base fails immediately; wait for the existing rebuild and retry.
Active jobs keep their pinned base.

Bases live under `builder.workdir/bases/` with identity
`<target>-<upstream-suite>-<native-architecture>-<archive-hash>`. The archive hash
is the first twelve SHA-256 hexadecimal characters of the exact upstream URL.
Changing the configured suite or URL selects another base and requires its own
initialization. No global apt cache or legacy base is reused.

The legacy interactive `/usr/share/irgsh/init.sh` can also initialize the default
base as `irgsh-builder`, using `/etc/irgsh/config.yaml`. It always announces the
base rebuild before confirmation and stops on failure. It also offers destructive
shared-state and repository initialization; use the direct builder command for
routine base maintenance.

## Service migration

Quiesce new submissions, let active jobs drain, and back up configuration and
state before changing the service identity. Record shared-account privileges:

```sh
id irgsh
sudo systemctl cat irgsh-builder.service
sudo systemctl stop irgsh-builder.service
```

Inventory every chief, repo, ISO, and other local service using `irgsh` before
removing any legacy group. Do not remove `docker` from `irgsh` when another local
service still needs it. Move only builder state to the dedicated account:

```sh
sudo chown -R irgsh-builder:irgsh-builder /var/lib/irgsh/builder
sudo chmod 0755 /var/lib/irgsh/builder
sudo systemctl edit irgsh-builder.service
```

For a package-installed binary, use this drop-in:

```ini
[Service]
User=irgsh-builder
Group=irgsh-builder
WorkingDirectory=/var/lib/irgsh/builder
ExecStart=
ExecStart=/usr/bin/irgsh-builder -c /etc/irgsh/builder-verbeek.yaml
TimeoutStopSec=infinity
```

For component-release deployment, retain the selected release path and config as
shown in [deployment provisioning](deployment.md#one-time-host-provisioning).
Inspect inherited drop-ins and remove builder-only `GNUPGHOME`, capability grants,
or namespace restrictions that conflict with this host contract. Do not weaken
another component's unit. Set suitable `MemoryMax`, `CPUQuota`, and `TasksMax`
for this host in the builder drop-in, then record their effective values.

After account and mapping checks and successful base initialization:

```sh
sudo systemctl daemon-reload
sudo systemctl restart irgsh-builder.service
sudo systemctl is-active irgsh-builder.service
sudo systemctl show irgsh-builder.service -p User -p Group -p SupplementaryGroups -p WorkingDirectory -p ExecStart -p MainPID
```

## One account and workdir per additional builder instance

Use a separate account, config, subordinate ranges, workdir, and health port for
every instance. For a second distribution named `rani`:

```sh
sudo addgroup --system irgsh-builder-rani
sudo adduser --system --home /var/lib/irgsh/builder-rani --no-create-home --ingroup irgsh-builder-rani --disabled-password --shell /usr/sbin/nologin --gecos 'IRGSH Rani Builder' irgsh-builder-rani
sudo adduser irgsh-builder-rani irgsh
sudo install -d -o irgsh-builder-rani -g irgsh-builder-rani -m 0755 /var/lib/irgsh/builder-rani
sudo usermod --add-subuids START-END --add-subgids START-END irgsh-builder-rani
```

Choose new, non-overlapping `START-END` ranges after inspecting both mapping
files. Create `/etc/irgsh/builder-rani.yaml` with `builder.dist_codename: rani`
and `builder.workdir: /var/lib/irgsh/builder-rani`. Preserve the selected
environment's chief and Redis addresses; use config ownership and modes above.

If no builder template exists, install the reviewed source unit as its template.
Inspect any existing template instead of overwriting it:

```sh
sudo install -o root -g root -m 0644 utils/systemctl/irgsh-builder.service /etc/systemd/system/irgsh-builder@.service
sudo systemctl edit irgsh-builder@rani.service
```

Use this instance drop-in for a package-installed binary:

```ini
[Service]
User=irgsh-builder-rani
Group=irgsh-builder-rani
WorkingDirectory=/var/lib/irgsh/builder-rani
Environment=PORT=8281
ExecStart=
ExecStart=/usr/bin/irgsh-builder -c /etc/irgsh/builder-rani.yaml
TimeoutStopSec=infinity
```

Initialize and start this instance:

```sh
sudo -u irgsh-builder-rani irgsh-builder -c /etc/irgsh/builder-rani.yaml init-base
sudo systemctl daemon-reload
sudo systemctl start irgsh-builder@rani.service
```

For component-release hosts, retain the `/opt/irgsh/current/builder/bin/irgsh-builder`
path in both commands and register `irgsh-builder@rani.service` with its health
endpoint in the [host-defined unit map](deployment.md#one-time-host-provisioning).
CI still selects only a component; it cannot supply arbitrary unit names.

## Rollback without restoring pbocker on a shared host

Stop new submissions and drain the affected builder. A previous native rootless
release can use the existing [verified deployment rollback](deployment.md#deploy-and-roll-back)
after checking its config and state compatibility. Keep its account and workdir.

If rollback requires the legacy builder, use a separately authorized isolated
legacy build host. Never restore pbocker, Docker socket access, privileged
containers, or shared-account builder execution on the shared production host.
Do not repoint release symlinks manually or reuse the native workdir as legacy
state. Chief, repo, ISO, signing keys, and archive ownership stay unchanged.

## Target-host evidence record

Record the tested IRGSH commit and release, date, operator, sanitized commands,
exit status, complete non-secret outputs, and log links. Repeat for every target
host and instance. Mark unavailable, failed, skipped, and unrun checks explicitly;
none is a pass. Use test-only signed inputs and no production keys or data.

Capture the following commands before exercising jobs:

```sh
git rev-parse HEAD
irgsh-builder --version
cat /etc/os-release
uname -srvmo
dpkg --print-architecture
sbuild --version
mmdebstrap --version
sysctl kernel.unprivileged_userns_clone user.max_user_namespaces
sysctl kernel.apparmor_restrict_unprivileged_userns
sudo -u irgsh-builder unshare --user --map-root-user true
sudo cat /etc/subuid /etc/subgid
stat -c '%U %G %a %n' /usr/bin/newuidmap /usr/bin/newgidmap
namei -l /var/lib/irgsh/builder
stat -c '%U %G %a %n' /var/lib/irgsh/builder
stat -fc %T /sys/fs/cgroup
sudo systemctl show irgsh-builder.service -p User -p Group -p SupplementaryGroups -p MainPID -p ControlGroup -p MemoryMax -p CPUQuotaPerSecUSec -p TasksMax -p NoNewPrivileges -p RestrictNamespaces
builder_pid=$(systemctl show irgsh-builder.service -p MainPID --value)
sudo sed -n '/^Uid:/p; /^Gid:/p; /^Groups:/p' "/proc/$builder_pid/status"
id irgsh-builder
sudo -u irgsh-builder find /var/lib/irgsh/builder/bases -name base.tar -exec sha256sum {} +
sudo -u irgsh-builder find /var/lib/irgsh/builder/bases -type d -exec stat -c '%U %G %a %n' {} +
sudo -u irgsh-builder sh -c 'test ! -w /var/run/docker.sock && printf "No Docker socket write access\n"'
```

Use the selected release binary for its version and record its source commit
from the release when the host has no checkout. Record absent sysctl keys as
absent; policy differs by host. The single-ID namespace probe does not replace
real mmdebstrap/sbuild execution. Record any inherited cgroup limits in addition
to the unit settings.

For each case below, retain the exact submission or fault-injection command,
pipeline ID, result, artifact checksums, and relevant sanitized logs:

- Signed `3.0 (native)` and `3.0 (quilt)` sources on the native architecture;
  preserve the original `.dsc` and every referenced source file through repo
  ingestion. Here non-native means source format, not foreign architecture.
- A clean base and a reused base; an update during an active job must not change
  that job's pinned base. Record base paths and checksums before and after.
- DNS and TLS failures, then recovery; retries must not exceed `build_attempts`
  and must leave no stale successful artifacts.
- Cancellation during an actual namespace build and during retry backoff; all
  descendants must exit, the final log must remain available, and no artifact
  may be handed to repo.
- Concurrent instances with distinct accounts, ranges, and workdirs; no shared
  output, base, or temporary state.
- Failed artifact upload; the build must fail and must not queue repo work.
- A complete chief-to-builder-to-repo pipeline, including binary metadata and
  successful retrieval of the published package from the test repository.

Target-host and rootless-container results are separate records. Do not close
container acceptance using native-host evidence.
