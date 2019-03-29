# irgsh-go

## Chief

## Builder

- Init:
  - Identity
  - base.tgz :heavy_check_mark:
- Clone :heavy_check_mark:
- Signing :heavy_check_mark:
- Build :heavy_check_mark:
- Upload

## Repo

- Download
- Inject
- Rebuild repo

## PabrikCD

- Build
- Upload

## Env

- `IRGSH_BUILDER_SIGNING_KEY` (required)
- `IRGSH_BUILDER_WORKDIR`

## Practial Usage (in one machine)

Please prepare your GPG key for signing purpose and set it on env var.

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
