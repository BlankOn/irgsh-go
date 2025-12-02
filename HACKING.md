# Guide for developer

## Requirements

Ensure that you have:
- Golang
- Docker
- These packages installed: `build-essential gpg pbuilder debootstrap devscripts curl reprepro`

## Cloning

`git clone git@github.com:BlankOn/irgsh-go.git && cd irgsh-go`

## Preparation

### Containerized Development (Docker Compose)

Alternatively, you can use the provided `docker-compose.dev.yml` for an isolated, reproducible environment (Debian bookworm by default). This mounts the source tree and the host Docker socket (for nested Docker usage required by the builder pbuilder workflow) and exposes service ports.

Build and start the dev container:

```bash
PROJECT_PATH=$(pwd) docker-compose -f docker-compose.dev.yml up -d --build
```

Enter the container shell and run services:

```bash
docker-compose -f docker-compose.dev.yml exec -it dev bash
```

you may need to run `vm.overcommit_memory = 1` in host machine

### GPG Key

You need to have a pair of GPG key in your GPG store.  If you don't have one, please create it with `gpg --generate-key`. When generating GPG key for irgsh infrastructure, please do not set any passphrase. Check it by running `gpg --list-key`

```
$ gpg --list-key
/home/herpiko/.gnupg/pubring.kbx
--------------------------------
pub   rsa4096 2020-10-17 [SC] [expires: 2021-10-17]
      41B4FC0A57E7F7F8DD94E0AA2D21BB5FAA32AF3F
uid           [ultimate] Herpiko Dwi Aguno <herpiko@gmail.com>
sub   rsa4096 2020-10-17 [E] [expires: 2021-10-17]
```
Copy the key identity (in my case, it's the `41B4FC0A57E7F7F8DD94E0AA2D21BB5FAA32AF3F` string) then paste it to replace `GPG_SIGN_KEY` in `utils/config.yml`

In dev environment, this single key will acts as both repository signing key and package maintainer signing key. On prod, they will be different keys.

### Initialization

#### Client

You need to build then initialize the CLI client to point out to the chief and your signing key (see `GPG Key` section).

- `make client`
- `./bin/irgsh-cli config --chief http://localhost:8080 --key 41B4FC0A57E7F7F8DD94E0AA2D21BB5FAA32AF3F`

#### Builder

`make builder-init`

This command will:
- Create pbuilder base.tgz that follow our configuration. This step need root privilege, you'll be asked for root password.
- Create docker image that will be used to build packages.

This one may take longer as it need to build an entire chroot environment.

#### Repository

You need to set the `repo.dist_signing_key` in `./utils/config.yaml` to your GPG key identity. For local development, it's okay to use the same key. In production, the repo signing key should be something else than maintainer's keys. Then,

`make repo-init`

This command will remove existing repositories if any and reinit the new one. You may be asked for your GPG key passphrase. You can tweak repository configuration in `repo` section of `utils/config.yml`

### Redis

`make redis`

## Starting up

Open three different terminal and run these command for each:
- `make chief` occupying port 8080
- `make builder`, occupying port 8081
- `make repo`, occupying port 8082

### Containerized Development (Docker Compose)

Alternatively, you can use the provided `docker-compose.dev.yml` for an isolated, reproducible environment (Debian bookworm by default). This mounts the source tree and the host Docker socket (for nested Docker usage required by the builder pbuilder workflow) and exposes service ports.

Build and start the dev container:

```bash
docker compose -f docker-compose.dev.yml build dev
docker compose -f docker-compose.dev.yml up -d
```

Enter the container shell and run services:

```bash
docker compose -f docker-compose.dev.yml exec dev bash
make chief &
make builder &
make repo &
```

(You can also run them one per terminal using `docker compose exec dev make chief` etc.)

Initialization steps (if first run):

```bash
docker compose -f docker-compose.dev.yml exec dev make builder-init
docker compose -f docker-compose.dev.yml exec dev make repo-init
```

Submitting a test package from inside the container:

```bash
docker compose -f docker-compose.dev.yml exec dev bash
./bin/irgsh-cli config --chief http://localhost:8080 --key YOUR_GPG_KEY_ID
./bin/irgsh-cli submit --experimental --source https://github.com/BlankOn/bromo-theme.git --package https://github.com/BlankOn-packages/bromo-theme.git --ignore-checks
```

Access services from the host:
- Chief: `http://localhost:18080` (mapped from container's 8080)
- Builder: `http://localhost:18081` (mapped from container's 8081)
- Repo: `http://localhost:18082` (mapped from container's 8082)

To stop and clean up:

```bash
docker compose -f docker-compose.dev.yml down
```

Override Debian suite (for testing `trixie`):

```bash
docker compose -f docker-compose.dev.yml build --build-arg DEBIAN_SUITE=trixie dev
```

**Notes:**
- The builder requires Docker access for creating build images; the socket mount plus `privileged: true` handles this (docker-in-docker via host daemon, not a nested daemon).
- If pbuilder base creation (`make builder-init`) fails due to permissions, ensure the container runs in privileged mode (already set) and the host user has Docker rights.
- GPG key provisioning still happens on the host; import or generate keys inside the container if isolating completely.
- Port mappings default to 18080-18082 on the host to avoid conflicts. Adjust in `docker-compose.dev.yml` as needed.

## Testing

Open the fourth terminal and try to submit dummy package using this command bellow:

- `./bin/irgsh-cli submit --experimental --source https://github.com/BlankOn/bromo-theme.git --package https://github.com/BlankOn-packages/bromo-theme.git --ignore-checks`

You may be asked for your GPG key passphrase. You'll see the package preprared in this terminal, then in the chief terminal (job coordination), then in builder terminal (package building), then in repo terminal (package submission into the repository).

If all is well, you can see the result by opening `http://localhost:8082/experimental/` on your web browser. At this point, you have explored the full cycle of the basic usage. You may want to start to hack.

Check the status of a pipeline

```
curl http://localhost:8080/api/v1/status?uuid=uuidstring
```

## Test & Coverage

```
make test
```

It will test the code and open the coverage result on your browser.
