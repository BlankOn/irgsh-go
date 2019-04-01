## Practial usage in development on one machine

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
