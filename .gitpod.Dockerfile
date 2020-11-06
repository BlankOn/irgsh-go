FROM debian:buster-slim
RUN apt update && apt install -y gpg pbuilder debootstrap devscripts python-apt reprepro make && \
  curl -O https://storage.googleapis.com/golang/go1.13.14.linux-amd64.tar.gz && \
  tar -xf go1.13.14.linux-amd64.tar.gz && \
  mv go /usr/local && \
  rm -rf go1.13.14.linux-amd64.tar.gz
ENV PATH="${PATH}:/usr/local/go/bin"
