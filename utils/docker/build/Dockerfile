FROM golang:1.12.1
RUN mkdir -p /tmp/src
COPY . /tmp/src
WORKDIR /tmp/src
RUN (cd chief && go install)
RUN (cp chief/utils.go builder/utils.go && cd builder && go install)
RUN (cp chief/utils.go iso/utils.go && cd iso && go install)
RUN (cp chief/utils.go repo/utils.go && cd repo && go install)
RUN (cp chief/utils.go cli/utils.go && cd cli && go install)
