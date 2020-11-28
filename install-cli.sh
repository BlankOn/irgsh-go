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
apt install -y gnupg build-essential devscripts debhelper jq 

if [ -f ./target/release.tar.gz ]; then
	# For development/testing purpose
	TEMP_PATH=$(pwd)/target
	DEV_INSTALL=1

  pushd $TEMP_PATH

  echo "Extracting ... "
  rm -rf irgsh-go && tar -xf release.tar.gz
  echo "Extracting [OK]"
  echo

  echo "Installing files ... "
  cp -v $TEMP_PATH/irgsh-go/usr/bin/irgsh-cli /usr/bin/irgsh-cli
else
	# Download and extract
  pushd $TEMP_PATH
	
  DOWNLOAD_URL=$(curl -ksL "https://api.github.com/repos/BlankOn/irgsh-go/releases/latest" | jq -r '.assets | .[] | select(.name == "irgsh-cli")| .browser_download_url')
	echo "Downloading ... "
	echo "$DOWNLOAD_URL"
	rm -f $TEMP_PATH/irgsh-cli && curl -L -f -o ./irgsh-cli $DOWNLOAD_URL
	if test $? -gt 0; then
		echo "Downloding [FAILED]"
		exit 1
	fi
	echo "Downloding [OK]"
	echo

  echo "Installing file ... "
  cp -v $TEMP_PATH/irgsh-cli /usr/bin/irgsh-cli
  chmod +x /usr/bin/irgsh-cli
fi

popd >/dev/null

echo "Happy hacking!"
