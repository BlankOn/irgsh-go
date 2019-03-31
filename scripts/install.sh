#!/usr/bin/env bash
sudo rm -f /tmp/irgsh-go.tar.gz && cd /tmp && wget https://github.com/BlankOn/irgsh-go/releases/download/v0.0.3-alpha/release.tar.gz -O ./irgsh-go.tar.gz && tar -xvf irgsh-go.tar.gz 
sudo cp /tmp/irgsh-go/bin/* /bin && sudo mkdir -p /usr/share/irgsh && sudo cp -R /tmp/irgsh-go/usr/share/irgsh/* /usr/share/irgsh/

