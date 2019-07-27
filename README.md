# irgsh-go

[![Build Status](https://travis-ci.org/BlankOn/irgsh-go.svg?branch=master)](https://travis-ci.org/BlankOn/irgsh-go) [![Go Report Card](https://goreportcard.com/badge/github.com/BlankOn/irgsh-go)](https://goreportcard.com/report/github.com/BlankOn/irgsh-go) [![codecov](https://codecov.io/gh/BlankOn/irgsh-go/branch/master/graph/badge.svg)](https://codecov.io/gh/BlankOn/irgsh-go)

IRGSH (https://groups.google.com/d/msg/blankon-dev/yvceclWjSw8/HZUL_m6-BS4J, pronunciation: *irgis*) is an all-in-one tool to create and maintain Debian-derived GNU/Linux distribution: from packaging to repository, from ISO build to release management. This codebase is a complete rewrite of the old IRGSH components (https://github.com/BlankOn?q=irgsh).

This is still under heavy development, therefore you should not rely on this for production since it still subject to breaking API changes.

Patches, suggestions and comments are welcome.

## Requirements

You need Docker, Redis and these packages,

```
gpg pbuilder debootstrap devscripts python-apt reprepro
```

## Install

```
curl -L -o- https://raw.githubusercontent.com/BlankOn/irgsh-go/master/install.sh | bash -s v0.0.22-alpha
```

The command will install the irgsh binaries, default configuration and daemons. A spesial user named `irgsh` will also be added to your system. Make sure `irgsh` user has access to Docker. To do so, run

```
sudo usermod -aG docker irgsh
```

## Components

A minimal IRGSH ecosystem contains three instances and a CLI tool.

- `irgsh-chief` acts as the master. The others (also applied to`irgsh-cli`) will talk to the chief. The chief also provides a web user interface for workers and pipelines monitoring.
- `irgsh-builder` is the builder worker of IRGSH.
- `irgsh-repo` will serves as repository so it may need huge volume of storage.
- `irgsh-iso` works as ISO builder and serves the ISO image files immediately. [WIP]
- `irgsh-cli

### Architecture

<img src="utils/assets/irgsh-distributed-architecture.png">

<img src="utils/assets/irgsh-flow.png">

GPG signature is used as authentication bearer on any submission attemp. Hence, you will need to register maintainer's public key to `irgsh`'s GPG keystore (read Initial setup).

### Initial setup

Please refer to `/etc/irgsh/config.yml` for available preferences.

#### Builder

Initialize and prepare the `base.tgz` (this one need root user or sudo),

```
sudo irgsh-builder init-base
```

Then, on behalf of `irgsh` user, prepare the containerized pbuilder,

```
irgsh-builder init-builder
```

#### Repo

On behalf of `irgsh` user, initialize the `irgsh-repo` to create and prepare reprepro repository,

```
irgsh-repo init

```

#### Chief

On behalf of `irgsh` user, add the package maintainer GPG public key(s),

```
gpg --import /path/to/maintainer-pubkey.asc
```

## CLI

`irgsh-cli` need to be configured first to define the `irgsh-chief` instance address and your GPG key as package mantainer,

```
irgsh-cli config --chief http://irgsh.blankonlinux.or.id:8080 --key B113D905C417D9C31DAD9F0E509A356412B6E77F
```

#### Run

You can start them with,

```
/etc/init.d/irgsh-chief start
/etc/init.d/irgsh-builder start
/etc/init.d/irgsh-repo start
```
Their logs is available in `/var/log/irgsh/`. After these three instances are up and running, you may continue to work with `irgsh-cli` from anywhere.

You can submit a package using `irgsh-cli`

```
irgsh-cli submit --source https://github.com/BlankOn/bromo-theme.git --package https://github.com/BlankOn-packages/bromo-theme.git
```

And checking the status of a pipeline,

```
irgsh-cli status 2019-04-01-174135_1ddbb9fe-0517-4cb0-9096-640f17532cf9
```


## Todos

### CLI

- Submit :heavy_check_mark:
- GPG signing :heavy_check_mark:
- Logging :heavy_check_mark:

### Chief

- Auth (GPG or mutual auth)
- WebSocket
- Builder registration
- Repo registration

### Builder

- Init:
  - base.tgz :heavy_check_mark:
- Clone :heavy_check_mark:
- Signing :heavy_check_mark:
- Build :heavy_check_mark:
- Upload :heavy_check_mark:
- Dockerized pbuilder :heavy_check_mark:
- Multiarch support
- RPM support

### Repo

- Init :heavy_check_mark:
- Sync :heavy_check_mark:
- Download :heavy_check_mark:
- Inject :heavy_check_mark:
- Rebuild repo :heavy_check_mark:
- Multiarch support
- RPM support

### PabrikCD

- Build
- Upload

### Release management

- Release cycle (RC, alpha, beta, final)
- Patches/Updates after release

### Others

- No sudo needed :heavy_check_mark:
- Daemonized instances :heavy_check_mark:
- Dockerized instances (docker-compose)
