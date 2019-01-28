# irgsh-go

## Chief

## Builder

- Clone
- Extract
- Build
- Uploading

## Repo

- Downloading
- Inject
- Rebuild repo

## PabrikCD

- Build
- Uploading

## Practial Usage (in one machine)

Prepare redis as backend

```
$ make redis
```

Run the nodes in different terminal

```
$ make builder
```
```
$ make repo
```
```
$ make chief
```


## Endpoints

The `chief` will live on port 8080.

- `/api/v1/submit` - POST
- `/api/v1/status` - GET


Submit new build pipeline,

```
curl --header "Content-Type: application/json" --request POST --data '{"sourceUrl":"git@github.com:BlankOn/manokwari.git","packageUrl":"git@github.com:blankon-packages/manokwari.git"}' http://localhost:8080/api/v1/submit
```

Check the status of a pipeline

```
curl http://localhost:8080/api/v1/status?uuid=uuidstring
```
