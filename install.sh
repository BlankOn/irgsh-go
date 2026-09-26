#!/usr/bin/env bash

# stop when error
set -e

DIST=$(cat /etc/*release | grep "^ID=" | cut -d '=' -f 2)

# Require sudo or root privilege
if [ $EUID != 0 ]; then
	sudo "$0" "$@"
	exit $?
fi

TEMP_PATH=/tmp
DEV_INSTALL=0

apt update

apt install -y gnupg sbuild mmdebstrap uidmap dpkg-dev devscripts ca-certificates debhelper python3-apt reprepro jq

SBUILD_VERSION=$(sbuild --version | sed -n 's/^sbuild (Debian sbuild) \([^ ]*\).*/\1/p')
if [ -z "$SBUILD_VERSION" ] || ! dpkg --compare-versions "$SBUILD_VERSION" ge 0.87.0; then
	echo 'sbuild >= 0.87.0 is required; configure a supported package source before installing IRGSH' >&2
	exit 1
fi

if [ -f ./target/release.tar.gz ]; then
	# For development/testing purpose
	TEMP_PATH=$(pwd)/target
	DEV_INSTALL=1
else
	# Download and extract
	DOWNLOAD_URL=$(curl -ksL "https://api.github.com/repos/BlankOn/irgsh-go/releases/latest" | jq -r '.assets | .[] | select(.name == "release.tar.gz")| .browser_download_url')
	echo "Downloading ... "
	echo "$DOWNLOAD_URL"
	rm -f $TEMP_PATH/release.tar.gz && cd $TEMP_PATH && curl -L -f -o ./release.tar.gz $DOWNLOAD_URL
	if test $? -gt 0; then
		echo "Downloading [FAILED]"
		exit 1
	fi
	echo "Downloading [OK]"
	echo
fi

pushd $TEMP_PATH

echo "Extracting ... "
rm -rf irgsh-go && tar -xf release.tar.gz
echo "Extracting [OK]"
echo

# Stop any existing instances if installed
if [ -x "$(command -v irgsh-chief )" ]; then
	echo "Stopping existing instance(s) ... "
	systemctl daemon-reload
	systemctl stop irgsh-chief || true
	systemctl stop irgsh-builder || true
	systemctl stop irgsh-repo || true
	killall irgsh-chief || true
	killall irgsh-builder || true
	killall irgsh-repo || true
	echo "Stopping existing instance(s) [OK]"
	echo
fi

if [ $DEV_INSTALL = 1 ]; then
	# For development/testing purpose
	# Clean up
	rm -rf /etc/irgsh/config.yaml
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
cp -v $TEMP_PATH/irgsh-go/lib/systemd/system/* /lib/systemd/system/
systemctl daemon-reload
# Configuration file
if [ ! -f "/etc/irgsh/config.yaml" ]; then
	cp -v $TEMP_PATH/irgsh-go/etc/irgsh/config.yaml /etc/irgsh/config.yaml
fi
if ! getent group irgsh >/dev/null; then
	addgroup --system irgsh
fi
if ! getent passwd irgsh >/dev/null; then
	adduser --system --home /var/lib/irgsh --no-create-home \
		--ingroup irgsh --disabled-password --shell /bin/bash \
		--gecos "IRGSH System User" irgsh
fi
if ! getent group irgsh-builder >/dev/null; then
	addgroup --system irgsh-builder
fi
if ! getent passwd irgsh-builder >/dev/null; then
	adduser --system --home /var/lib/irgsh/builder --no-create-home \
		--ingroup irgsh-builder --disabled-password --shell /usr/sbin/nologin \
		--gecos "IRGSH Builder" irgsh-builder
fi
adduser irgsh-builder irgsh
chown irgsh:irgsh /var/lib/irgsh
for state in chief repo iso gnupg; do
	if [ -d "/var/lib/irgsh/$state" ]; then
		chown -R irgsh:irgsh "/var/lib/irgsh/$state"
	fi
done
chown -R irgsh:irgsh /var/log/irgsh
chmod 0755 /var/lib/irgsh /var/log/irgsh
install -d -o irgsh-builder -g irgsh-builder -m 0755 /var/lib/irgsh/builder
chown root:irgsh /etc/irgsh /etc/irgsh/config.yaml
chmod 0750 /etc/irgsh
chmod 0640 /etc/irgsh/config.yaml
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
	sed -i "/dist_signing_key/c\  dist_signing_key: 'GPG_SIGN_KEY'" /etc/irgsh/config.yaml
	sed -i "s/GPG_SIGN_KEY/$GPG_SIGN_KEY/g" /etc/irgsh/config.yaml
	su -c "chmod -R 700 /var/lib/irgsh/gnupg" -s /bin/bash irgsh
	echo "Generating GPG key [OK]"
	gpg --armor --export >/tmp/pubkey
	su -c "GNUPGHOME=/var/lib/irgsh/gnupg gpg --import < /tmp/pubkey" -s /bin/bash irgsh

	# reinit repo
	su -c "irgsh-repo -c /etc/irgsh/config.yaml init > /dev/null" -s /bin/bash irgsh

fi

popd >/dev/null

# Enable the services
systemctl enable irgsh-chief
systemctl enable irgsh-builder
systemctl enable irgsh-repo

echo "Happy hacking!"
