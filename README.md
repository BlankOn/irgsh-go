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

To install or update irgsh-go, you can use the install script using cURL:

```
curl -o- https://raw.githubusercontent.com/BlankOn/irgsh-go/master/utils/scripts/install.sh | bash
```

The command will install the irgsh binaries, default configuration and daemons. A spesial user named `irgsh` will also be added to your system.

## Components

Although these can be run in one machine, a minimal IRGSH ecosystem contains four instances and hey depend on Redis as backend (queue, pubsub).

- `irgsh-chief` acts as the master. The others (also applied to`irgsh-cli`) will talk to the chief. The chief also provides a web user interface for worker and pipeline monitoring.
- `irgsh-builder` is the builder worker of IRGSH.
- `irgsh-repo` will serves as repository so it may need huge volume of storage.
- `irgsh-iso` works as ISO builder and serves the ISO image files immediately. [WIP]

### Architecture

<img src="utils/assets/irgsh-distributed-architecture.png">

### Workflow

<img src="utils/assets/irgsh-flow.png">

### Initial setup

Please refer to `/etc/irgsh/config.yml` for available preferences.

Running the chief is quite simple as starting the service with `/etc/init.d/irgsh-chief start`, as well for `irgsh-builder` and `irgsh-repo`. For `irgsh-builder` and `irgsh-repo`, we need to initialize them first on behalf of `irgsh` user.

#### Builder

Initialize and prepare the pbuilder (this one need root user or sudo),

```
sudo irgsh-builder init-base
```

Prepare the containerized pbuilder,

```
irgsh-builder init-builder
```

Since the builder is using Docker, you need to make sure `irgsh` user has access to Docker. To do so, run

```
sudo usermod -aG docker irgsh
```

#### Repo

Initialize the repo to create and prepare reprepro repository,

```
irgsh-repo init

```

#### Chief

Add the package maintainer GPG public key(s),

```
gpg --import /path/to/pubkey.asc
```

#### Run

You can start them from `service`,

```
service irgsh-chief start
service irgsh-builder start
service irgsh-repo start
```

After these three instances are up and running, you may continue to work with `irgsh-cli` from anywhere.

## CLI

`irgsh-cli` need to be initialized first to define the `irgsh-chief` instance address and your GPG key as package mantainer,

```
irgsh-cli init --chief http://irgsh.blankonlinux.or.id:8080 --key B113D905C417D9C31DAD9F0E509A356412B6E77F
```

Then you can submit a package,

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
