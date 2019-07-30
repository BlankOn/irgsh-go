#!/usr/bin/env bash

## Estu (andro.medh4@gmail.com)

TEMP_PATH=/tmp
DEV_INSTALL=0

# Install paket
apt install -y gnupg pbuilder debootstrap devscripts python-apt reprepro jq tree

# Download paket
cd $TEMP_PATH && wget -c https://github.com/BlankOn/irgsh-go/releases/download/v0.0.23-alpha/release.tar.gz

# Extrack
echo "Extracting ... "
rm -rf irgsh-go && tar -xf release.tar.gz
echo "Extracting [OK]"

## Systemctl
cp -v $TEMP_PATH/irgsh-go/etc/Systemctl/* /lib/systemd/system/
systemctl daemon-reload
systemctl enable irgsh-repo
systemctl enable irgsh-iso
systemctl enable irgsh-chief

# Create required dirs
mkdir -p /etc/irgsh
mkdir -p /usr/share/irgsh
mkdir -p /var/lib/irgsh/chief/submissions
mkdir -p /var/lib/irgsh/chief/artifacts
mkdir -p /var/lib/irgsh/chief/logs
mkdir -p /var/lib/irgsh/builder
mkdir -p /var/lib/irgsh/iso
mkdir -p /var/lib/irgsh/repo
mkdir -p /var/log/irgsh

# Install the files | error
echo "Installing files ... "
cp -v $TEMP_PATH/irgsh-go/usr/bin/* /usr/bin/
cp -v $TEMP_PATH/irgsh-go/usr/share/irgsh/init.sh /usr/bin/irgsh-init
cp -vR $TEMP_PATH/irgsh-go/usr/share/irgsh/* /usr/share/irgsh/

if [ ! -f "/etc/irgsh/config.yml" ]; then
  cp -v $TEMP_PATH/irgsh-go/etc/irgsh/config.yml /etc/irgsh/config.yml
fi

# irgsh user
if getent passwd irgsh >/dev/null 2>&1; then
  echo "irgsh user is already exists"
else
  useradd -d /var/lib/irgsh -s /bin/bash -G root -u 880 -U irgsh
  chown -R irgsh:irgsh /var/lib/irgsh
  chmod -R u+rw /var/lib/irgsh
  usermod -aG docker irgsh
  echo "irgsh user added to system"
fi
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

echo "Happy hacking!"