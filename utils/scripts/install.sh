#!/usr/bin/env bash

# Download and extract
sudo rm -f /tmp/irgsh-go.tar.gz && cd /tmp && wget https://github.com/BlankOn/irgsh-go/releases/download/v0.0.10-alpha/release.tar.gz -O ./irgsh-go.tar.gz && tar -xvf irgsh-go.tar.gz 

# Create required dirs
sudo mkdir -p /etc/irgsh
sudo mkdir -p /usr/share/irgsh
sudo mkdir -p /var/irgsh/chief
sudo mkdir -p /var/irgsh/builder
sudo mkdir -p /var/irgsh/repo
sudo mkdir -p /var/log/irgsh

# Install the files

# Binaries
sudo cp -v /tmp/irgsh-go/usr/bin/* /usr/bin/
# Templates
sudo cp -vR /tmp/irgsh-go/usr/share/irgsh/* /usr/share/irgsh/
# Configuration
sudo cp -v /tmp/irgsh-go/etc/irgsh/config.yml /etc/irgsh/config.yml
# Daemon
sudo cp -v /tmp/irgsh-go/etc/irgsh/config.yml /etc/irgsh/config.yml
