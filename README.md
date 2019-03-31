# irgsh-go

## CLI

- Auth
- GPG signing

## Chief

- Auth
- WebSocket

## Builder

- Init:
  - Identity
  - base.tgz :heavy_check_mark:
- Clone :heavy_check_mark:
- Signing :heavy_check_mark:
- Build :heavy_check_mark:
- Upload :heavy_check_mark:

## Repo

- Init :heavy_check_mark:
- Sync :heavy_check_mark:
- Download :heavy_check_mark:
- Inject :heavy_check_mark:
- Rebuild repo :heavy_check_mark:

## PabrikCD

- Build
- Upload

## Env

See `env.local` for entire env vars reference.

## Practial Usage (in one machine)

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

The `chief` will live on port 8080.

- `/api/v1/submit` - POST
- `/api/v1/status` - GET


Submit new build pipeline,

```
curl --header "Content-Type: application/json" --request POST --data '{"sourceUrl":"git@github.com:BlankOn/bromo-theme.git","packageUrl":"git@github.com:blankon-packages/bromo-theme.git"}' http://localhost:8080/api/v1/submit
```

Check the status of a pipeline

```
curl http://localhost:8080/api/v1/status?uuid=uuidstring
```
