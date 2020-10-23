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
apt install -y gnupg pbuilder debootstrap devscripts python-apt reprepro jq

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

# Stop any existing instances
echo "Stopping existing instance(s) ... "
systemctl daemon-reload
/etc/init.d/irgsh-chief stop || true
/etc/init.d/irgsh-builder stop || true
/etc/init.d/irgsh-repo stop || true
systemctl stop irgsh-chief
systemctl stop irgsh-builder
systemctl stop irgsh-repo
killall irgsh-chief || true
killall irgsh-builder || true
killall irgsh-repo || true
echo "Stopping existing instance(s) [OK]"
echo

if [ $DEV_INSTALL = 1 ]; then
	# For development/testing purpose
	# Clean up
	rm -rf /etc/irgsh/config.yml
	rm -rf /var/lib/irgsh/chief
	rm -rf /var/lib/irgsh/repo
	rm -rf /var/lib/irgsh/gnupg
	# Do not overwrite /var/lib/irgsh/builder
	#rm -rf /var/lib/irgsh/builder
fi

# Create required dirs
mkdir -p /etc/irgsh
mkdir -p /usr/share/irgsh
mkdir -p /var/lib/irgsh/chief/submissions
mkdir -p /var/lib/irgsh/chief/artifacts
mkdir -p /var/lib/irgsh/chief/logs
mkdir -p /var/lib/irgsh/builder
mkdir -p /var/lib/irgsh/repo
mkdir -p /var/log/irgsh

# Install the files
echo "Installing files ... "
cp -v $TEMP_PATH/irgsh-go/usr/bin/* /usr/bin/
cp -v $TEMP_PATH/irgsh-go/usr/share/irgsh/init.sh /usr/bin/irgsh-init
cp -vR $TEMP_PATH/irgsh-go/usr/share/irgsh/* /usr/share/irgsh/
cp -v $TEMP_PATH/irgsh-go/etc/init.d/* /etc/init.d/
systemctl daemon-reload
# Configuration file
if [ ! -f "/etc/irgsh/config.yml" ]; then
	cp -v $TEMP_PATH/irgsh-go/etc/irgsh/config.yml /etc/irgsh/config.yml
fi
# irgsh user
groupadd irgsh || true
if getent passwd irgsh >/dev/null 2>&1; then
	echo "irgsh user is already exists"
else
	useradd -d /var/lib/irgsh -s /bin/bash -G root -u 880 -U irgsh
	chown -R irgsh:irgsh /var/lib/irgsh
	chmod -R u+rw /var/lib/irgsh
	usermod -aG docker irgsh
	echo "irgsh user added to system"
fi
usermod -aG irgsh irgsh
echo "Installing files [OK]"
echo

if [ $DEV_INSTALL = 1 ]; then
	# For development/testing purpose
	GPG_KEY_NAME="BlankOn Project"
	GPG_KEY_EMAIL="blankon-dev@googlegroups.com"
	echo "Generating GPG key ..."
	su -c "mkdir -p /var/lib/irgsh/gnupg/private-keys-v1.d" -s /bin/bash irgsh
	su -c "echo 'export GNUPGHOME=/var/lib/irgsh/gnupg' > ~/.bashrc" -s /bin/bash irgsh
	su -c "echo 'cd ~/' >> ~/.bashrc" -s /bin/bash irgsh
	su -c "echo '%no-protection' > ~/gen-key-script" -s /bin/bash irgsh
	su -c "echo 'Key-Type: 1' >> ~//gen-key-script" -s /bin/bash irgsh
	su -c "echo 'Key-Length: 4096' >> ~//gen-key-script" -s /bin/bash irgsh
	su -c "echo 'Subkey-Type: 1' >> ~//gen-key-script" -s /bin/bash irgsh
	su -c "echo 'Subkey-Length: 4096' >> ~//gen-key-script" -s /bin/bash irgsh
	su -c "echo 'Name-Real: $GPG_KEY_NAME' >> ~//gen-key-script" -s /bin/bash irgsh
	su -c "echo 'Name-Email: $GPG_KEY_EMAIL' >> ~//gen-key-script" -s /bin/bash irgsh
	su -c "echo 'Expire-Date: 5y' >> ~//gen-key-script" -s /bin/bash irgsh
	su -c "GNUPGHOME=/var/lib/irgsh/gnupg gpg -k > /dev/null" -s /bin/bash irgsh
	su -c "GNUPGHOME=/var/lib/irgsh/gnupg gpg --batch --gen-key ~/gen-key-script > /dev/null" -s /bin/bash irgsh
	GPG_SIGN_KEY=$(su -c "GNUPGHOME=/var/lib/irgsh/gnupg gpg -K | grep uid -B 1 | head -n 1 | xargs" -s /bin/bash irgsh)
	sed -i "/dist_signing_key/c\  dist_signing_key: 'GPG_SIGN_KEY'" /etc/irgsh/config.yml
	sed -i "s/GPG_SIGN_KEY/$GPG_SIGN_KEY/g" /etc/irgsh/config.yml
	su -c "chmod -R 700 /var/lib/irgsh/gnupg" -s /bin/bash irgsh
	echo "Generating GPG key [OK]"
	gpg --armor --export >/tmp/pubkey
	su -c "GNUPGHOME=/var/lib/irgsh/gnupg gpg --import < /tmp/pubkey" -s /bin/bash irgsh

	# reinit repo
	su -c "irgsh-repo init > /dev/null" -s /bin/bash irgsh

fi

popd >/dev/null

# Enable the services
/lib/systemd/systemd-sysv-install enable irgsh-chief
/lib/systemd/systemd-sysv-install enable irgsh-builder
/lib/systemd/systemd-sysv-install enable irgsh-repo

echo "Happy hacking!"
