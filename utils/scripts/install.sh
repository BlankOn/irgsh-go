#!/usr/bin/env bash

TEMP_PATH=/tmp
# Download and extract
sudo rm -f $TEMP_PATH/irgsh-go.tar.gz && cd $TEMP_PATH && wget https://github.com/BlankOn/irgsh-go/releases/download/v0.0.17-alpha/release.tar.gz -O ./irgsh-go.tar.gz && tar -xvf irgsh-go.tar.gz 

# Stop any existing instances
sudo systemctl daemon-reload
sudo service irgsh-chief stop || true
sudo service irgsh-builder stop || true
sudo service irgsh-iso stop || true
sudo service irgsh-repo stop || true
sudo killall irgsh-chief || true
sudo killall irgsh-builder || true
sudo killall irgsh-iso || true
sudo killall irgsh-repo || true

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
sudo cp -v $TEMP_PATH/irgsh-go/usr/bin/* /usr/bin/
sudo cp -vR $TEMP_PATH/irgsh-go/usr/share/irgsh/* /usr/share/irgsh/
sudo cp -v $TEMP_PATH/irgsh-go/etc/init.d/* /etc/init.d/

# Configuration
if [ ! -f "/etc/irgsh/config.yml" ] 
then
	sudo cp -v  $TEMP_PATH/irgsh-go/etc/irgsh/config.yml /etc/irgsh/config.yml
fi


# GPG key
if [ ! -d "/var/lib/irgsh/gnupg" ] 
then
	echo "Generate GPG key" 
	sudo echo "%no-protection" > /tmp/gen-key-script
	sudo echo "Key-Type: 1" >> /tmp/gen-key-script
	sudo echo "Key-Length: 4096" >> /tmp/gen-key-script
	sudo echo "Subkey-Type: 1" >> /tmp/gen-key-script
	sudo echo "Subkey-Length: 4096" >> /tmp/gen-key-script
	sudo echo "Name-Real: IRGSH" >> /tmp/gen-key-script
	sudo echo "Name-Email: blankon-dev@googlegroups.com" >> /tmp/gen-key-script
	sudo echo "Expire-Date: 5y" >> /tmp/gen-key-script
	sudo rm -rf /tmp/irgsh-gnupg
	sudo mkdir -p /tmp/irgsh-gnupg/private-keys-v1.d
	sudo chmod 7000 /tmp/irgsh-gnupg/private-keys-v1.d
	sudo GNUPGHOME=/tmp/irgsh-gnupg gpg --batch --gen-key /tmp/gen-key-script
	sudo sed -i "s/GPG_SIGN_KEY/$(sudo GNUPGHOME=/tmp/irgsh-gnupg gpg -K | grep uid -B 1 | head -n 1 | xargs)/g" /etc/irgsh/config.yml
	sudo mv /tmp/irgsh-gnupg /var/lib/irgsh/gnupg
	sudo echo "export GNUPGHOME=/var/lib/irgsh/gnupg" > /tmp/irgsh-bashrc
	sudo mv /tmp/irgsh-bashrc /var/lib/irgsh/.bashrc
fi


ME=irgsh
sudo useradd -d /var/lib/irgsh -s /bin/bash -G root -u 880 -U $ME
sudo chown -vR $ME:$ME /var/lib/irgsh
sudo chmod -vR u+rw /var/lib/irgsh

echo ""
echo ""
echo "Happy hacking!"
