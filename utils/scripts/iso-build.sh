#!/bin/bash

# Load configuration from .env file
if [ -f .env ]; then
  source .env
fi

## Default messages
RESULT="gagal terbit ❌"
ACTION="Log build dapat disimak"

## Args
REPO=$1
BRANCH=$2
OUTPUT_DIR=$3
REPO_NAME=$(echo "$REPO" | sed -E 's|.*github.com[:/]([^/]+/[^/.]+)(\.git)?|\1|')
# Optional
COMMIT=$3
ARCH=amd64

START=$(date +%s)

sudo umount $(mount | grep live-build | cut -d ' ' -f 3) || true
sudo rm -rf ./chroot ./local ./cache ./build || true
sudo mkdir ${OUTPUT_DIR}

## Skip further steps if this is a build in local computer
if [ -z "$REPO" ] || [ -z "$BRANCH" ]
then
  sudo lb clean --purge
  sudo lb config --architectures $ARCH
  sudo time lb build | sudo tee -a blankon-live-image-$ARCH.build.log
  exit $?
fi

echo "Processing $REPO $BRANCH $COMMIT ..."

## Assume that this is in prod
JAHITAN_PATH=$OUTPUT_DIR
TODAY=$(date '+%Y%m%d')
TODAY_COUNT=$(ls $JAHITAN_PATH | grep $TODAY | wc -l)
TODAY_COUNT=$(($TODAY_COUNT + 1))
TARGET_DIR=$JAHITAN_PATH/$TODAY-$TODAY_COUNT

mkdir -p $TARGET_DIR

## Preparation
git clone --depth 1 -b $BRANCH $REPO /tmp/$TODAY-$TODAY_COUNT

# If a specific commit was passed, switch to it.
# If not, stay on the latest code from the branch.
if [ -n "$COMMIT" ]; then
     git -C /tmp/$TODAY-$TODAY_COUNT checkout $COMMIT
fi
COMMIT=$(git -C /tmp/$TODAY-$TODAY_COUNT rev-parse --short HEAD)
CLEAN_REPO_URL=$(echo "$REPO" | sed 's/\.git$//')
COMMIT_URL="$CLEAN_REPO_URL/commit/$COMMIT"
mkdir -p /tmp/$TODAY-$TODAY_COUNT
sudo rm -rf config
cp -vR /tmp/$TODAY-$TODAY_COUNT/config config
sed -i 's/BUILD_NUMBER/'"$TODAY-$TODAY_COUNT"'/g' config/bootloaders/syslinux_common/splash.svg

## Build
sudo lb clean
sudo lb config --architectures $ARCH
rm -f blankon-live-image-$ARCH.build.log
sudo lb build 2>&1 | tee blankon-live-image-$ARCH.build.log

if tail -n 10 blankon-live-image-$ARCH.build.log | grep -q "P: Build completed successfully"; then
  RESULT="telah terbit ✅"
  ACTION="Berkas citra dapat diunduh"
  ## Export to jahitan
  cp -v blankon-live-image-$ARCH.contents $TARGET_DIR/blankon-live-image-$ARCH.contents
  cp -v blankon-live-image-$ARCH.files $TARGET_DIR/blankon-live-image-$ARCH.files
  cp -v blankon-live-image-$ARCH.hybrid.iso.zsync $TARGET_DIR/blankon-live-image-$ARCH.hybrid.iso.zsync
  cp -v blankon-live-image-$ARCH.packages $TARGET_DIR/blankon-live-image-$ARCH.packages
  cp -v blankon-live-image-$ARCH.hybrid.iso $TARGET_DIR/blankon-live-image-$ARCH.hybrid.iso
  sha256sum $TARGET_DIR/blankon-live-image-$ARCH.hybrid.iso > $TARGET_DIR/blankon-live-image-$ARCH.hybrid.iso.sha256sum
  rm $JAHITAN_PATH/current
  ln -s $TARGET_DIR $JAHITAN_PATH/current
  echo "$TODAY-$TODAY_COUNT" > $JAHITAN_PATH/current/current.txt
fi

END=$(date +%s)
DURATION=$((END - START))
TOTAL_DURATION="Done in $(date -d@$DURATION -u +%H:%M:%S)."
echo $TOTAL_DURATION
echo $TOTAL_DURATION >> blankon-live-image-$ARCH.build.log
tail -n 100 blankon-live-image-$ARCH.build.log > $TARGET_DIR/blankon-live-image-$ARCH.tail100.build.log.txt
cp -v blankon-live-image-$ARCH.build.log $TARGET_DIR/blankon-live-image-$ARCH.build.log.txt

## Clean up the mounted entities
sudo umount $(mount | grep live-build | cut -d ' ' -f 3) || true
