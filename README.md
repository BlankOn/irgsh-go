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

See `env.local` for entire env vars reference.

## Practial usage in development on one machine

Please prepare your GPG key for signing purpose and set it on env var.

For builder, you need initialize the base.tgz first with `make irgsh-builder-init`

For repo, you need initialize the reprepro repository first with `make irgsh-repo-init`

Spin up redis as backend

```
$ make redis
```

Run the nodes in different terminal

```
$ make irgsh-builder
```
```
$ make irgsh-repo
```
```
$ make irgsh-chief
```

Testing

```
$ make submit
```


## Endpoints

The `chief` will be live on port 8080.

- `/api/v1/submit` - POST
- `/api/v1/status` - GET


Submit new build pipeline,

```
curl --header "Content-Type: application/json" --request POST --data '{"sourceUrl":"https://github.com/BlankOn/bromo-theme.git","packageUrl":"https://github.com/blankon-packages/bromo-theme.git"}' http://localhost:8080/api/v1/submit
```

Check the status of a pipeline

```
curl http://localhost:8080/api/v1/status?uuid=uuidstring
```

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

