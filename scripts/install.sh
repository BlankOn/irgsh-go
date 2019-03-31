#!/bin/bash
cd /tmp && wget https://github.com/BlankOn/irgsh-go/archive/master.tar.gz && tar -xvf master.tar.gz && cp irgsh-go-master/bin/* /bin && mkdir -p /usr/share/irgsh && cp -R irgsh-go-master/share /usr/share/irgsh

