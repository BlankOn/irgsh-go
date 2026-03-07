FROM golang:1.23-bookworm AS build

WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY . .
RUN make build

FROM debian:bookworm

ENV DEBIAN_FRONTEND=noninteractive

# Pre-seed pbuilder debconf to avoid interactive mirror prompt.
RUN echo 'pbuilder pbuilder/mirrorsite string http://deb.debian.org/debian' | debconf-set-selections

# All deps needed by chief, builder, repo, and CLI.
# Keeping apt lists so builder init-base can run apt-get install without
# re-downloading package metadata.
RUN apt-get update && apt-get install -y --no-install-recommends \
    build-essential \
    pbuilder debootstrap devscripts dpkg-dev equivs \
    reprepro \
    gnupg gpg-agent dirmngr \
    curl wget git ca-certificates lsb-release \
    && rm -rf /var/cache/apt/archives/*.deb

# Install Docker CLI from Docker's official repo (Debian's docker.io is too old).
RUN install -m 0755 -d /etc/apt/keyrings \
    && curl -fsSL https://download.docker.com/linux/debian/gpg -o /etc/apt/keyrings/docker.asc \
    && chmod a+r /etc/apt/keyrings/docker.asc \
    && echo "deb [arch=$(dpkg --print-architecture) signed-by=/etc/apt/keyrings/docker.asc] \
       https://download.docker.com/linux/debian bookworm stable" > /etc/apt/sources.list.d/docker.list \
    && apt-get update \
    && apt-get install -y --no-install-recommends docker-ce-cli \
    && rm -rf /var/cache/apt/archives/*.deb

COPY --from=build /src/bin/ /usr/local/bin/
COPY --from=build /src/utils/reprepro-template /usr/share/irgsh/reprepro-template
COPY --from=build /src/utils/config.yaml /usr/share/irgsh/config.yaml
