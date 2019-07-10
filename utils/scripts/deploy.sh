#!/bin/bash
tar -xvf release.tar.gz
sed -i /wget/d install.sh
./install.sh
