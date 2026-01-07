#!/bin/bash

# Require sudo or root privilege
if [ $EUID != 0 ]; then
	sudo "$0" "$@"
	exit $?
fi

# Doctor, check requirements

OVERWRITE_WORKDIR=0
OVERWRITE_GPG=0
OVERWRITE_BASE_TGZ=0
OVERWRITE_PBUILDER=0
OVERWRITE_REPO=0

if [ ! -f "/etc/irgsh/config.yml" ]; then
	cp -v /usr/share/irgsh/config.yml /etc/irgsh/config.yml
fi

echo "Before continue, please consider to review your onfiguration file (/etc/irgsh/config.yml)"
read -p "Do you want to continue? (y/N) " -n 1 -r
echo
if [[ $REPLY =~ ^[Yy]$ ]]; then
	echo
else
	exit 0
fi

if [ -d "/var/lib/irgsh" ]; then
	echo "Working dir (/var/lib/irgsh) is already exists."
	read -p "Do you want to recreate it? (y/N) " -n 1 -r
	echo
	if [[ $REPLY =~ ^[Yy]$ ]]; then
		OVERWRITE_WORKDIR=1
		OVERWRITE_REPO=1
		OVERWRITE_GPG=1
		echo
		echo "Please enter your GPG key configuration,"
		read -p 'Name  : ' GPG_KEY_NAME
		read -p 'Email : ' GPG_KEY_EMAIL
	else
		if [ ! -f "/var/lib/irgsh/repo/init.log" ]; then
			OVERWRITE_REPO=1
		fi
		if [ -d "/var/lib/irgsh/gnupg/private-keys-v1.d" ]; then
			echo
			echo "GPG key is already exists. Regenerating GPG key will also recreate your repository"
			read -p "Do you want to regenerate it? (y/N) " -n 1 -r
			echo
			if [[ $REPLY =~ ^[Yy]$ ]]; then
				if [ ! -f "/var/lib/irgsh/builder/pbocker/base.tgz" ]; then
					OVERWRITE_BASE_TGZ=1
					OVERWRITE_PBUILDER=1
				fi
				OVERWRITE_REPO=1
				OVERWRITE_GPG=1
				echo
				echo "Please enter your GPG key configuration,"
				read -p 'Name  : ' GPG_KEY_NAME
				read -p 'Email : ' GPG_KEY_EMAIL
			fi
		fi
	fi
fi

if [ ! -f "/var/lib/irgsh/builder/pbocker/base.tgz" ]; then
	OVERWRITE_BASE_TGZ=1
	OVERWRITE_PBUILDER=1
else
	if [ "$(docker images | grep pbocker | cut -d ' ' -f 1)" = "pbocker" ]; then
		echo
	else
		OVERWRITE_PBUILDER=1
	fi
fi

if [ $OVERWRITE_REPO = 0 ]; then
	if [ $OVERWRITE_GPG = 0 ]; then
		echo
		read -p "Do you want to recreate the repository? (y/N) " -n 1 -r
		echo
		if [[ $REPLY =~ ^[Yy]$ ]]; then
			OVERWRITE_REPO=1
		fi
	fi
fi

# ====================================================================
echo
echo
echo "Please review your current init configuration:"
echo "---------------------------"
echo "OVERWRITE_WORKDIR=$OVERWRITE_WORKDIR"
echo "OVERWRITE_BASE_TGZ=$OVERWRITE_BASE_TGZ"
echo "OVERWRITE_PBUILDER=$OVERWRITE_PBUILDER"
echo "OVERWRITE_REPO=$OVERWRITE_REPO"
echo "OVERWRITE_GPG=$OVERWRITE_WORKDIR"
echo "GPG_KEY_NAME=$GPG_KEY_NAME"
echo "GPG_KEY_EMAIL=$GPG_KEY_EMAIL"
echo
read -p "Do you want to continue? (y/N) " -n 1 -r
echo
if [[ $REPLY =~ ^[Yy]$ ]]; then
	echo
else
	exit 0
fi
echo
echo
# Working directory
if [ $OVERWRITE_WORKDIR = 1 ]; then
	rm -rf /var/lib/irgsh
	mkdir -p /var/lib/irgsh/chief/submissions
	mkdir -p /var/lib/irgsh/chief/artifacts
	mkdir -p /var/lib/irgsh/chief/logs
	mkdir -p /var/lib/irgsh/builder
	mkdir -p /var/lib/irgsh/iso
	mkdir -p /var/lib/irgsh/repo
fi

chown -R irgsh:irgsh /var/lib/irgsh
chmod -R u+rw /var/lib/irgsh

# GPG key
if [ $OVERWRITE_GPG = 1 ]; then
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
	su -c "GNUPGHOME=/var/lib/irgsh/gnupg gpg -K" -s /bin/bash irgsh
	echo "Generating GPG key [OK]"
fi

# Repo init
if [ $OVERWRITE_REPO = 1 ]; then
	su -c "GNUPGHOME=/var/lib/irgsh/gnupg irgsh-repo init" -s /bin/bash irgsh
fi

# Base.tgz init
if [ $OVERWRITE_BASE_TGZ = 1 ]; then
	irgsh-builder init-base
fi

# Pbuilder init
if [ $OVERWRITE_PBUILDER = 1 ]; then
	echo
	su -c "irgsh-builder init-builder" -s /bin/bash irgsh
fi

if [ $OVERWRITE_GPG = 1 ]; then
	echo
	echo "IMPORTANT: Do not forget add maintaner's public key(s) to irgsh's GPG keystore"
	echo "Initialization done!"
fi
