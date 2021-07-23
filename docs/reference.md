# Reference


## irgsh-chief
Acts as the master. The others (also applied to irgsh-cli) will talk to the chief. The chief also provides a web user interface for workers and pipelines monitoring.



## irgsh-builder
The builder worker of IRGSH.

### COMMANDS
     init-builder, i  Initialize builder
     init-base, i     Initialize pbuilder base.tgz. This need to be run under sudo or root
     update-base, i   update base.tgz
     help, h          Shows a list of commands or help for one command

### GLOBAL OPTIONS
     --help, -h     show help
     --version, -v  print the version

## irgsh-repo
Serves as repository so it may need huge volume of storage.

### COMMANDS
     init, i  initialize repository
     sync, i  update base.tgz
     help, h  Shows a list of commands or help for one command

### GLOBAL OPTIONS
     --help, -h     show help
     --version, -v  print the version



## irgsh-cli
Client-side tool to maintain packages.

### COMMANDS
     config   Configure irgsh-cli
     submit   Submit new build
     status   Check status of a pipeline
     log      Read the logs of a pipeline
     update   Update the irgsh-cli tool
     help, h  Shows a list of commands or help for one command

### GLOBAL OPTIONS
     --help, -h     show help
     --version, -v  print the version
