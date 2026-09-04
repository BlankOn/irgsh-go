#!/bin/bash

# Bundled BlankOn live-build script, run by irgsh-iso.
#
# It is meant to run inside the ISO worker's configured workdir
# (iso.workdir), which is a persistent live-build tree: chroot/, cache/,
# auto/ and local/ are deliberately reused between builds. Pass --no-cache to
# irgsh-cli build-iso to have the worker clear them beforehand.
#
# Deployment values come from a .env file in the current directory, which
# irgsh-iso writes from its own config before every run.
#
# Usage: iso-build.sh <repo-url> <branch> [commit] [skip-lock]

# Load configuration from .env file

if [ -f .env ]; then
  source .env
fi

JAHITAN_PATH="$BUILD_JAHITAN_PATH"
PUBLISH_URL="$BUILD_PUBLISH_URL"

if [ -z "$JAHITAN_PATH" ]; then
  echo "Error: BUILD_JAHITAN_PATH is missing. Please check your .env file."
  exit 1
fi

# Create Lockfile
LOCKFILE="${BUILD_LOCKFILE:-/tmp/blankon-build.lock}"

if [ -z "$4" ]; then
    if [ -f "$LOCKFILE" ]; then
        OLD_PID=$(cat "$LOCKFILE")
        if ps -p "$OLD_PID" > /dev/null 2>&1; then
            echo "Error: Build already in progress (PID: $OLD_PID). Exiting."
            exit 1
        else
            echo "Warning: Removing stale lock file from PID $OLD_PID"
            rm -f "$LOCKFILE"
        fi
    fi
    echo $$ > "$LOCKFILE"
fi

# Helper function. Announcing to Telegram is optional: irgsh deployments do not
# have to carry a bot token just to build an ISO.
send_telegram() {
    local message="$1"
    if [ -z "$TELEGRAM_BOT_KEY" ]; then
        return 0
    fi
    curl -X POST -H 'Content-Type: application/json' \
        -d "{\"chat_id\": \"-1001067745576\", \"message_thread_id\": \"51909\", \"parse_mode\": \"HTML\", \"disable_web_page_preview\": true, \"text\": \"$message\", \"disable_notification\": true}" \
        https://api.telegram.org/bot$TELEGRAM_BOT_KEY/sendMessage
}

cleanup() {
    rm -f "$LOCKFILE"
    if [ -n "$REPO" ] && [ -n "$BRANCH" ]; then
        if [ -n "$COMMIT_URL" ]; then
            # Clone succeeded, we have commit info
            send_telegram "💿 Jahitan harian $TODAY-$TODAY_COUNT [ revisi <a href=\\\"$COMMIT_URL\\\">$COMMIT</a> ] dari $REPO_NAME cabang $BRANCH $RESULT. $FAILURE_REASON $ACTION di ${PUBLISH_URL}/$TODAY-$TODAY_COUNT/"
        else
            # Clone failed, no commit info available
            send_telegram "💿 Jahitan harian $TODAY-$TODAY_COUNT dari $REPO_NAME cabang $BRANCH $RESULT. $FAILURE_REASON "
        fi
    fi
}

# Setup trap
trap cleanup EXIT

## Default messages
RESULT="gagal terbit ❌"
ACTION="Log build dapat disimak"
FAILURE_REASON=""

## Args
REPO=$1
BRANCH=$2
COMMIT=$3
REPO_NAME=$(echo "$REPO" | sed -E 's|.*github.com[:/]([^/]+/[^/.]+)(\.git)?|\1|')

# Optional
ARCH=amd64

START=$(date +%s)

sudo umount $(mount | grep live-build | cut -d ' ' -f 3) || true

## Skip further steps if this is a build in local computer
if [ -z "$REPO" ] || [ -z "$BRANCH" ]
then
  sudo lb clean
  sudo lb config --architectures $ARCH
  sudo time lb build | sudo tee -a blankon-live-image-$ARCH.build.log
  exit $?
fi

echo "Processing $REPO $BRANCH $COMMIT ..."

## Assume that this is in prod
TODAY=$(date '+%Y%m%d')

TODAY_COUNT=$(ls "$JAHITAN_PATH" | grep "$TODAY" | wc -l)
TODAY_COUNT=$(($TODAY_COUNT + 1))

TARGET_DIR=$JAHITAN_PATH/$TODAY-$TODAY_COUNT

mkdir -p "$TARGET_DIR"
sudo mkdir -p tmp || true
sudo chmod -R a+rw tmp

## Preparation
if ! git clone -b "$BRANCH" "$REPO" "./tmp/$TODAY-$TODAY_COUNT" 2>&1; then
    FAILURE_REASON="Error: Failed to clone $REPO branch $BRANCH"
    echo "$FAILURE_REASON"
    exit 1
fi
# Double-check the clone succeeded by verifying .git exists
if [ ! -d "./tmp/$TODAY-$TODAY_COUNT/.git" ]; then
    FAILURE_REASON="Error: Clone directory is missing or incomplete"
    echo "$FAILURE_REASON"
    exit 1
fi

# If a specific commit was passed, switch to it.
# If not, stay on the latest code from the branch.
if [ -n "$COMMIT" ]; then
    git -C "./tmp/$TODAY-$TODAY_COUNT" checkout "$COMMIT"
fi

COMMIT=$(git -C ./tmp/$TODAY-$TODAY_COUNT rev-parse --short HEAD)
CLEAN_REPO_URL=$(echo "$REPO" | sed 's/\.git$//')
COMMIT_URL="$CLEAN_REPO_URL/commit/$COMMIT"
mkdir -p ./tmp/$TODAY-$TODAY_COUNT
sudo rm -rf config
sudo cp -vR ./tmp/$TODAY-$TODAY_COUNT/config config
sed -i 's/BUILD_NUMBER/'"$TODAY-$TODAY_COUNT"'/g' config/bootloaders/syslinux_common/splash.svg

## Build
sudo lb clean --purge
sudo lb config --architectures $ARCH
sudo rm -rf blankon-live-image-$ARCH.build.log
sudo lb build 2>&1 | tee blankon-live-image-$ARCH.build.log
# lb build's own status, not tee's. Without this a failed build looks like a
# successful one to everything downstream.
LB_STATUS=${PIPESTATUS[0]}

BUILD_FAILED=0
if [ "$LB_STATUS" -eq 0 ] && tail -n 10 blankon-live-image-$ARCH.build.log | grep -q "P: Build completed successfully"; then
  RESULT="telah terbit ✅"
  ACTION="Berkas citra dapat diunduh"
  ## Export to jahitan
  cp -v blankon-live-image-$ARCH.contents $TARGET_DIR/blankon-live-image-$ARCH.contents
  cp -v blankon-live-image-$ARCH.files $TARGET_DIR/blankon-live-image-$ARCH.files
  cp -v blankon-live-image-$ARCH.packages $TARGET_DIR/blankon-live-image-$ARCH.packages
  cp -v blankon-live-image-$ARCH.hybrid.iso $TARGET_DIR/blankon-live-image-$ARCH.hybrid.iso
  zsyncmake -u "${PUBLISH_URL}/current/blankon-live-image-amd64.hybrid.iso" -o $TARGET_DIR/blankon-live-image-$ARCH.hybrid.iso.zsync $TARGET_DIR/blankon-live-image-$ARCH.hybrid.iso
  sha256sum $TARGET_DIR/blankon-live-image-$ARCH.hybrid.iso | sed 's#  .*/#  #' > $TARGET_DIR/blankon-live-image-$ARCH.hybrid.iso.sha256sum
  sudo rm -rf $JAHITAN_PATH/current
  #ln -s $TARGET_DIR $JAHITAN_PATH/current
  sudo cp -vR $TARGET_DIR $JAHITAN_PATH/current
  echo "$TODAY-$TODAY_COUNT" | sudo tee $JAHITAN_PATH/current/current.txt > /dev/null
else
  BUILD_FAILED=1
  FAILURE_REASON="Error: lb build did not complete successfully (exit $LB_STATUS)."
  echo "$FAILURE_REASON"
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

# Report the real outcome. The log copies above happen either way so a failed
# build is still diagnosable.
exit $BUILD_FAILED
