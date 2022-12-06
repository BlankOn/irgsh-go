# Guide for developer

## Requirements

Ensure that you have:
- Golang
- Docker
- These packages installed: `gpg pbuilder debootstrap devscripts curl reprepro`

## Cloning

`https://github.com/BlankOn/irgsh-go.git && cd irgsh-go`

## Preparation

### GPG Key

You need to have a pair of GPG key in your GPG store.  If you don't have one, please create it with `gpg --generate-key`. Check it by running `gpg --list-key`

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

## Testing

Open the fourth terminal and try to submit dummy package using this command bellow:

- `./bin/irgsh-cli submit --experimental --source https://github.com/BlankOn/bromo-theme.git --package https://github.com/BlankOn-packages/bromo-theme.git`

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
