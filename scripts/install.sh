#!/usr/bin/env bash
sudo rm -f /tmp/irgsh-go.tar.gz && cd /tmp && wget https://github.com/BlankOn/irgsh-go/releases/download/v0.0.8-alpha/release.tar.gz -O ./irgsh-go.tar.gz && tar -xvf irgsh-go.tar.gz 
sudo cp /tmp/irgsh-go/usr/bin/* /usr/bin
sudo mkdir -p /usr/share/irgsh && sudo cp -R /tmp/irgsh-go/usr/share/irgsh/* /usr/share/irgsh/
sudo mkdir -p /etc/irgsh && sudo cp -R /tmp/irgsh-go/etc/irgsh/* /etc/irgsh/
sudo mkdir -p /var/irgsh/chief
sudo mkdir -p /var/irgsh/builder
sudo mkdir -p /var/irgsh/repo
sudo chmod -vR a+rw /var/irgsh/*
