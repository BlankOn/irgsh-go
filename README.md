# irgsh-go

## Requirements

```
sudo apt-get install -y pbuilder debootstrap devscripts python-apt reprepro
```

## Install

Before install, make sure you stopped all the running irgsh-\* instances. To install or update irgsh-go, you can use the install script using cURL:

```
curl -o- https://raw.githubusercontent.com/BlankOn/irgsh-go/master/scripts/install.sh | bash
```

## Env

## IRGSH instances

Minimal IRGSH ecosystem contains three instances that supposed to be live on different machines. They depends on Redis as backend (queue, pubsub).

- `irgsh-chief` acts as the master. The the others (also applied to`irgsh-cli`) will talks to the chief.
- `irgsh-builder` is the builder worker of IRGSH. Machine with speed CPU and high RAM is favored.
- `irgsh-repo` will serve as repository so it may need huge volume of storage.

We may need more than one `irgsh`builder`, depends on our available resources.

Please refer to `env.local` and `config.yml` for available preference variables

Running the chief is quite simple as `irgsh-chief -c config.yml`, as well for `irgsh-builder` and `irgsh-repo`. For `irgsh-builder` and `irgsh-repo`, we need to initialize them first.

Initialize the builder to create and prepare pbuilder,

```
irgsh-pbuilder init
```

Initialize the repo to create and prepare reprepro repository,

```
irgsh-repo init

```

After these three instances are up and running, you may continue to work with `irgsh-cli`.



## CLI

`irgsh-cli` need to be initialized first to define the `irgsh-chief` instance address,

```
irgsh-cli --chief http://irgsh.blankonlinux.or.id:8080 init
```

Then you can submit a package,

```
irgsh-cli --source https://github.com/BlankOn/bromo-theme.git --package https://github.com/BlankOn-packages/bromo-theme.git submit
```

And checking the status of a pipeline,

```
irgsh-cli --pipeline 2019-04-01-174135_1ddbb9fe-0517-4cb0-9096-640f17532cf9 status
```


## Todos

### CLI

- Submit :heavy_check_mark:
- GPG signing
- Auth

### Chief

- Auth
- WebSocket

### Builder

- Init:
  - Identity
  - base.tgz :heavy_check_mark:
- Clone :heavy_check_mark:
- Signing :heavy_check_mark:
- Build :heavy_check_mark:
- Upload :heavy_check_mark:

### Repo

- Init :heavy_check_mark:
- Sync :heavy_check_mark:
- Download :heavy_check_mark:
- Inject :heavy_check_mark:
- Rebuild repo :heavy_check_mark:

### PabrikCD

- Build
- Upload

