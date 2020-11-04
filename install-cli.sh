#!/usr/bin/env bash

DIST=$(cat /etc/*release | grep "^ID=" | cut -d '=' -f 2)

# Require sudo or root privilege
if [ $EUID != 0 ]; then
	sudo "$0" "$@"
	exit $?
fi

TEMP_PATH=/tmp
DEV_INSTALL=0

apt update
apt install -y gnupg jq

if [ -f ./target/release.tar.gz ]; then
	# For development/testing purpose
	TEMP_PATH=$(pwd)/target
	DEV_INSTALL=1
else
	# Download and extract
	DOWNLOAD_URL=$(curl -ksL "https://api.github.com/repos/BlankOn/irgsh-go/releases/latest" | jq -r ".assets[0].browser_download_url")
	echo "Downloading ... "
	echo "$DOWNLOAD_URL"
	rm -f $TEMP_PATH/release.tar.gz && cd $TEMP_PATH && curl -L -f -o ./release.tar.gz $DOWNLOAD_URL
	if test $? -gt 0; then
		echo "Downloding [FAILED]"
		exit 1
	fi
	echo "Downloding [OK]"
	echo
fi

pushd $TEMP_PATH

echo "Extracting ... "
rm -rf irgsh-go && tar -xf release.tar.gz
echo "Extracting [OK]"
echo

echo "Installing files ... "
cp -v $TEMP_PATH/irgsh-go/usr/bin/irgsh-cli /usr/bin/irgsh-cli

popd >/dev/null

echo "Happy hacking!"
