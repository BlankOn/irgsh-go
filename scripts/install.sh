#!/bin/bash
cd /tmp && wget https://github.com/BlankOn/irgsh-go/releases/download/v0.0.2-alpha/release.tar.gz && tar -xvf irgsh.tar.gz && cp irgsh-go/bin/* /bin && mkdir -p /usr/share/irgsh && cp -R irgsh-go/share /usr/share/irgsh

