#!/usr/bin/env bash
cd /tmp && wget https://github.com/BlankOn/irgsh-go/releases/download/v0.0.2-alpha/release.tar.gz -O ./irgsh-go.tar.gz && tar -xvf irgsh-go.tar.gz && cp irgsh-go/bin/* /bin && mkdir -p /usr/share/irgsh && cp -R irgsh-go/usr/share/irgsh /usr/share/irgsh

