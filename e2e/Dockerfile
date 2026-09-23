FROM golang:1.25-bookworm AS build

WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY . .
RUN make build

FROM debian:bookworm

ENV DEBIAN_FRONTEND=noninteractive

RUN apt-get update && apt-get install -y --no-install-recommends \
    build-essential \
    devscripts dpkg-dev \
    reprepro \
    gnupg gpg-agent dirmngr \
    curl wget git ca-certificates \
    && rm -rf /var/cache/apt/archives/*.deb

COPY --from=build /src/bin/ /usr/local/bin/
COPY --from=build /src/utils/reprepro-template /usr/share/irgsh/reprepro-template
COPY --from=build /src/utils/config.yaml /usr/share/irgsh/config.yaml
