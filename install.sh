#!/usr/bin/env bash
TEMP_PATH=/tmp

sudo apt-get install -y gpg pbuilder debootstrap devscripts python-apt reprepro jq

if [ $IRGSH_CLEAN_INSTALL -eq 1 ]; then
	TEMP_PATH=$(pwd)/target
else
	# Download and extract
	DOWNLOAD_URL=$(curl -ksL "https://api.github.com/repos/x64dbg/x64dbg/releases/latest" | jq -r ".assets[0].browser_download_url")
	echo "Downloading ... "
	echo "$DOWNLOAD_URL"
	sudo rm -f $TEMP_PATH/release.tar.gz && cd $TEMP_PATH && curl -L -f -o ./release.tar.gz $DOWNLOAD_URL
	if test $? -gt 0; then
	  echo "Downloding [FAILED]"
	  exit 1
	fi
	echo "Downloding [OK]"
	echo ""
fi

pushd $TEMP_PATH

echo "Extracting ... "
sudo rm -rf irgsh-go && sudo tar -xf release.tar.gz
echo "Extracting [OK]"
echo ""

# Stop any existing instances
echo "Stopping existing instance(s) ... "
sudo systemctl daemon-reload
sudo /etc/init.d/irgsh-chief stop || true
sudo /etc/init.d/irgsh-builder stop || true
sudo /etc/init.d/irgsh-iso stop || true
sudo /etc/init.d/irgsh-repo stop || true
sudo killall irgsh-chief || true
sudo killall irgsh-builder || true
sudo killall irgsh-iso || true
sudo killall irgsh-repo || true
echo "Stopping existing instance(s) [OK]"
echo ""

if [ $IRGSH_CLEAN_INSTALL -eq 1 ]; then
	# Clean up
	sudo rm -rf/etc/irgsh/config.yml
  sudo rm -rf /var/lib/irgsh/chief
  sudo rm -rf /var/lib/irgsh/repo
  sudo rm -rf /var/lib/irgsh/gnupg
	# Do not overwrite /var/lib/irgsh/builder
  #sudo rm -rf /var/lib/irgsh/builder
fi

# Create required dirs
sudo mkdir -p /etc/irgsh
sudo mkdir -p /usr/share/irgsh
sudo mkdir -p /var/lib/irgsh/chief/submissions
sudo mkdir -p /var/lib/irgsh/chief/artifacts
sudo mkdir -p /var/lib/irgsh/chief/logs
sudo mkdir -p /var/lib/irgsh/builder
sudo mkdir -p /var/lib/irgsh/iso
sudo mkdir -p /var/lib/irgsh/repo
sudo mkdir -p /var/log/irgsh

# Install the files
echo "Installing files ... "
sudo cp $TEMP_PATH/irgsh-go/usr/bin/* /usr/bin/
sudo cp $TEMP_PATH/irgsh-go/etc/init.d/* /etc/init.d/
sudo cp -R $TEMP_PATH/irgsh-go/usr/share/irgsh/* /usr/share/irgsh/
# Configuration file
if [ ! -f "/etc/irgsh/config.yml" ]; then
	sudo cp -v $TEMP_PATH/irgsh-go/etc/irgsh/config.yml /etc/irgsh/config.yml
fi
echo "Installing files [OK]"
echo ""

ME=irgsh
sudo useradd -d /var/lib/irgsh -s /bin/bash -G root -u 880 -U $ME
sudo chown -R $ME:$ME /var/lib/irgsh
sudo chmod -R u+rw /var/lib/irgsh

# GPG key
if [ $IRGSH_CLEAN_INSTALL -eq 1 ]; then
	GPG_KEY_NAME="BlankOn Project"
	GPG_KEY_EMAIL="blankon-dev@googlegroups.com"
	echo "Generating GPG key ..."
	sudo su -c "mkdir -p /var/lib/irgsh/gnupg/private-keys-v1.d" -s /bin/bash irgsh
	sudo su -c "echo 'export GNUPGHOME=/var/lib/irgsh/gnupg' > ~/.bashrc" -s /bin/bash irgsh
	sudo su -c "echo 'cd ~/' >> ~/.bashrc" -s /bin/bash irgsh
	sudo su -c "echo '%no-protection' > ~/gen-key-script" -s /bin/bash irgsh
	sudo su -c "echo 'Key-Type: 1' >> ~//gen-key-script" -s /bin/bash irgsh
	sudo su -c "echo 'Key-Length: 4096' >> ~//gen-key-script" -s /bin/bash irgsh
	sudo su -c "echo 'Subkey-Type: 1' >> ~//gen-key-script" -s /bin/bash irgsh
	sudo su -c "echo 'Subkey-Length: 4096' >> ~//gen-key-script" -s /bin/bash irgsh
	sudo su -c "echo 'Name-Real: $GPG_KEY_NAME' >> ~//gen-key-script" -s /bin/bash irgsh
	sudo su -c "echo 'Name-Email: $GPG_KEY_EMAIL' >> ~//gen-key-script" -s /bin/bash irgsh
	sudo su -c "echo 'Expire-Date: 5y' >> ~//gen-key-script" -s /bin/bash irgsh
	sudo su -c "GNUPGHOME=/var/lib/irgsh/gnupg gpg -k > /dev/null" -s /bin/bash irgsh
	sudo su -c "GNUPGHOME=/var/lib/irgsh/gnupg gpg --batch --gen-key ~/gen-key-script > /dev/null" -s /bin/bash irgsh
	GPG_SIGN_KEY=$(sudo su -c "GNUPGHOME=/var/lib/irgsh/gnupg gpg -K | grep uid -B 1 | head -n 1 | xargs" -s /bin/bash irgsh)
	sudo sed -i "s/GPG_SIGN_KEY/$GPG_SIGN_KEY/g" /etc/irgsh/config.yml
	sudo su -c "chmod -R 700 /var/lib/irgsh/gnupg" -s /bin/bash irgsh
	echo "Generating GPG key [OK]"
	gpg --armor --export > /tmp/pubkey
	sudo su -c "GNUPGHOME=/var/lib/irgsh/gnupg gpg --import < /tmp/pubkey" -s /bin/bash irgsh
fi

popd

echo ""
echo "Happy hacking!"
