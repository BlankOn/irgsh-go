FROM golang:1.23-bookworm AS build

WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY . .
RUN make build

FROM debian:bookworm

# All deps needed by chief, builder, repo, and CLI.
# Keeping apt lists so builder init-base can run apt-get install without
# re-downloading package metadata.
RUN apt-get update && apt-get install -y --no-install-recommends \
    pbuilder debootstrap devscripts dpkg-dev equivs \
    reprepro \
    gnupg gpg-agent dirmngr \
    curl wget git ca-certificates lsb-release \
    && rm -rf /var/cache/apt/archives/*.deb

# Install Docker CLI only (for builder to talk to host Docker daemon).
RUN apt-get install -y --no-install-recommends docker.io \
    && rm -rf /var/cache/apt/archives/*.deb

COPY --from=build /src/bin/ /usr/local/bin/
COPY --from=build /src/utils/reprepro-template /usr/share/irgsh/reprepro-template
COPY --from=build /src/utils/config.yaml /usr/share/irgsh/config.yaml
