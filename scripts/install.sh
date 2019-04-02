#!/usr/bin/env bash
sudo rm -f /tmp/irgsh-go.tar.gz && cd /tmp && wget https://github.com/BlankOn/irgsh-go/releases/download/v0.0.9-alpha/release.tar.gz -O ./irgsh-go.tar.gz && tar -xvf irgsh-go.tar.gz 
sudo cp -v /tmp/irgsh-go/usr/bin/* /usr/bin/
sudo mkdir -p /usr/share/irgsh && sudo cp -vR /tmp/irgsh-go/usr/share/irgsh/* /usr/share/irgsh/
sudo mkdir -p /etc/irgsh && sudo cp -v /tmp/irgsh-go/etc/irgsh/config.yml /etc/irgsh/config.yml
sudo mkdir -p /var/irgsh/chief
sudo mkdir -p /var/irgsh/builder
sudo mkdir -p /var/irgsh/repo
sudo chmod -vR a+rw /var/irgsh/*
