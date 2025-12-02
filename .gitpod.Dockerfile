ARG DEBIAN_SUITE=bookworm
FROM debian:${DEBIAN_SUITE}-slim
ARG GO_VERSION=1.13.15
RUN apt update && apt install -y --no-install-recommends \
    gpg pbuilder debootstrap devscripts python3-apt reprepro make curl ca-certificates git && \
    rm -rf /var/lib/apt/lists/* && \
    curl -O https://storage.googleapis.com/golang/go${GO_VERSION}.linux-amd64.tar.gz && \
    tar -xf go${GO_VERSION}.linux-amd64.tar.gz && \
    mv go /usr/local && \
    rm -rf go${GO_VERSION}.linux-amd64.tar.gz
ENV PATH="${PATH}:/usr/local/go/bin"
