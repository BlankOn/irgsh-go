#!/bin/bash

# Bundled BlankOn live-build script, run by irgsh-iso.
#
# irgsh-iso runs it as its unprivileged account inside a new user, mount, and
# PID namespace, in the worker's configured workdir (iso.workdir). Every job
# rebuilds config/ and auto/ from the selected live-build revision, and
# lb clean --purge removes the previous chroot and cache.
#
# Deployment values come from a .env file in the current directory, which
# irgsh-iso writes from its own config before every run.
#
# Usage: iso-build.sh <repo-url> <branch> [commit] [skip-lock]

if [ -f .env ]; then
  source .env
fi

JAHITAN_PATH="$BUILD_JAHITAN_PATH"
PUBLISH_URL="$BUILD_PUBLISH_URL"

if [ -z "$JAHITAN_PATH" ]; then
  echo "Error: BUILD_JAHITAN_PATH is missing. Please check your .env file."
  exit 1
fi

LOCKFILE="${BUILD_LOCKFILE:-/tmp/blankon-build.lock}"

if [ -z "$4" ]; then
    if ! exec 9> "$LOCKFILE"; then
        echo "Error: cannot open lock file $LOCKFILE"
        exit 1
    fi
    if ! flock -n 9; then
        echo "Error: Build already in progress. Exiting."
        exit 1
    fi
fi

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
    if [ -n "$REPO" ] && [ -n "$BRANCH" ]; then
        if [ -n "$COMMIT_URL" ]; then
            send_telegram "💿 Jahitan harian $TODAY-$TODAY_COUNT [ revisi <a href=\\\"$COMMIT_URL\\\">$COMMIT</a> ] dari $REPO_NAME cabang $BRANCH $RESULT. $FAILURE_REASON $ACTION di ${PUBLISH_URL}/$TODAY-$TODAY_COUNT/"
        else
            send_telegram "💿 Jahitan harian $TODAY-$TODAY_COUNT dari $REPO_NAME cabang $BRANCH $RESULT. $FAILURE_REASON "
        fi
    fi
}

trap cleanup EXIT

fail() {
    FAILURE_REASON="$1"
    echo "$FAILURE_REASON"
    exit 1
}

prepare_config() {
    local source=$1
    local directory
    rm -rf config auto variant
    if [ -f "$source/variant" ]; then
        LAYOUT=variant
        VARIANT=$(<"$source/variant")
        if ! [[ "$VARIANT" =~ ^[a-z0-9][a-z0-9-]*$ ]]; then
            fail "Error: Invalid variant: $VARIANT"
        fi
        for directory in config/common "config/$VARIANT" auto; do
            if [ ! -d "$source/$directory" ]; then
                fail "Error: $directory is missing from $BRANCH"
            fi
        done
        mkdir config &&
            cp -a "$source/config/common/." config/ &&
            cp -a "$source/config/$VARIANT/." config/ &&
            cp -a "$source/auto" auto &&
            cp "$source/variant" variant ||
            fail "Error: Failed to assemble the $VARIANT configuration"
        IMAGE_NAME="blankon-live-image-$VARIANT-$ARCH"
    else
        LAYOUT=legacy
        cp -R "$source/config" config || fail "Error: config is missing from $BRANCH"
        if [ -d "$source/auto" ]; then
            cp -a "$source/auto" auto || fail "Error: Failed to copy auto from $BRANCH"
        fi
    fi
}

apply_archive_config() {
    local conf=config/includes.chroot/etc/blankon/archive.conf
    local applied
    if [ ! -r "$conf" ]; then
        if [ "$LAYOUT" = variant ]; then
            fail "Error: $conf is missing"
        fi
        echo "archive.conf is absent; using the mirrors from config/bootstrap"
        return
    fi
    ARCHIVE_URI=$(. ./config/includes.chroot/etc/blankon/archive.conf && printf '%s' "$ARCHIVE_URI")
    if ! [[ "$ARCHIVE_URI" =~ ^https?://[A-Za-z0-9.-]+(:[0-9]+)?(/[A-Za-z0-9._~-]+)*/?$ ]]; then
        fail "Error: $conf defines an invalid ARCHIVE_URI: $ARCHIVE_URI"
    fi
    sed -i -E "s#^(LB_(PARENT_)?MIRROR_[A-Z_]+=)\".*\"#\\1\"${ARCHIVE_URI}\"#" config/bootstrap
    applied=$(grep -cE "^LB_(PARENT_)?MIRROR_[A-Z_]+=\"${ARCHIVE_URI}\"" config/bootstrap 2>/dev/null)
    if [ "${applied:-0}" -eq 0 ]; then
        fail "Error: no mirror entries in config/bootstrap were set to $ARCHIVE_URI"
    fi
    echo "Pointed $applied mirror entries in config/bootstrap at $ARCHIVE_URI"
}

publish() {
    local suffix sum
    for suffix in contents files packages hybrid.iso; do
        cp -v "$IMAGE_NAME.$suffix" "$TARGET_DIR/$IMAGE_NAME.$suffix" || return 1
    done
    zsyncmake -u "${PUBLISH_URL}/current/$IMAGE_NAME.hybrid.iso" -o "$TARGET_DIR/$IMAGE_NAME.hybrid.iso.zsync" "$TARGET_DIR/$IMAGE_NAME.hybrid.iso" || return 1
    sum=$(sha256sum "$TARGET_DIR/$IMAGE_NAME.hybrid.iso") || return 1
    printf '%s  %s\n' "${sum%% *}" "$IMAGE_NAME.hybrid.iso" > "$TARGET_DIR/$IMAGE_NAME.hybrid.iso.sha256sum" || return 1
    rm -rf "$JAHITAN_PATH/current.new" "$JAHITAN_PATH/current.old" &&
        cp -R "$TARGET_DIR" "$JAHITAN_PATH/current.new" &&
        echo "$TODAY-$TODAY_COUNT" | tee "$JAHITAN_PATH/current.new/current.txt" > /dev/null ||
        return 1
    if [ -e "$JAHITAN_PATH/current" ]; then
        mv "$JAHITAN_PATH/current" "$JAHITAN_PATH/current.old" || return 1
    fi
    if ! mv "$JAHITAN_PATH/current.new" "$JAHITAN_PATH/current"; then
        if [ -e "$JAHITAN_PATH/current.old" ]; then
            mv "$JAHITAN_PATH/current.old" "$JAHITAN_PATH/current"
        fi
        return 1
    fi
    rm -rf "$JAHITAN_PATH/current.old"
}

RESULT="gagal terbit ❌"
ACTION="Log build dapat disimak"
FAILURE_REASON=""

REPO=$1
BRANCH=$2
COMMIT=$3
REPO_NAME=$(echo "$REPO" | sed -E 's|.*github.com[:/]([^/]+/[^/.]+)(\.git)?|\1|')

ARCH=amd64
IMAGE_NAME="blankon-live-image-$ARCH"

START=$(date +%s)

if [ -z "$REPO" ] || [ -z "$BRANCH" ]
then
  lb clean
  lb config --architectures $ARCH
  time lb build | tee -a blankon-live-image-$ARCH.build.log
  exit $?
fi

echo "Processing $REPO $BRANCH $COMMIT ..."

TODAY=$(date '+%Y%m%d')

TODAY_COUNT=$(find -H "$JAHITAN_PATH" -mindepth 1 -maxdepth 1 -type d -name "$TODAY-*" | wc -l)
TODAY_COUNT=$(($TODAY_COUNT + 1))

TARGET_DIR=$JAHITAN_PATH/$TODAY-$TODAY_COUNT
SOURCE_DIR=./tmp/$TODAY-$TODAY_COUNT

mkdir -p "$TARGET_DIR"
mkdir -p tmp

if ! git clone -b "$BRANCH" "$REPO" "$SOURCE_DIR" 2>&1; then
    fail "Error: Failed to clone $REPO branch $BRANCH"
fi
if [ ! -d "$SOURCE_DIR/.git" ]; then
    fail "Error: Clone directory is missing or incomplete"
fi

if [ -n "$COMMIT" ]; then
    git -C "$SOURCE_DIR" checkout -q "$COMMIT" || fail "Error: Failed to checkout commit $COMMIT"
    git -C "$SOURCE_DIR" merge-base --is-ancestor "$COMMIT" "refs/remotes/origin/$BRANCH" || fail "Error: Commit $COMMIT is not on branch $BRANCH"
    [ "$(git -C "$SOURCE_DIR" rev-parse HEAD)" = "$COMMIT" ] || fail "Error: HEAD does not match commit $COMMIT"
fi

COMMIT_FULL=$(git -C "$SOURCE_DIR" rev-parse HEAD)
COMMIT=$(git -C "$SOURCE_DIR" rev-parse --short HEAD)
CLEAN_REPO_URL=$(echo "$REPO" | sed 's/\.git$//')
COMMIT_URL="$CLEAN_REPO_URL/commit/$COMMIT"

prepare_config "$SOURCE_DIR"
sed -i 's/BUILD_NUMBER/'"$TODAY-$TODAY_COUNT"'/g' config/bootloaders/syslinux_common/splash.svg

lb clean --purge || fail "Error: lb clean failed"
lb config --architectures $ARCH || fail "Error: lb config failed"
apply_archive_config

echo "[ ISO BUILD ] repo=$REPO branch=$BRANCH commit=$COMMIT_FULL layout=$LAYOUT variant=${VARIANT:-none} archive=${ARCHIVE_URI:-config/bootstrap}"

rm -rf "$IMAGE_NAME.build.log"
lb build 2>&1 | tee "$IMAGE_NAME.build.log"
LB_STATUS=${PIPESTATUS[0]}

BUILD_FAILED=0
if [ "$LB_STATUS" -eq 0 ] && tail -n 10 "$IMAGE_NAME.build.log" | grep -q "P: Build completed successfully"; then
  if publish; then
    RESULT="telah terbit ✅"
    ACTION="Berkas citra dapat diunduh"
  else
    BUILD_FAILED=1
    FAILURE_REASON="Error: Failed to publish $IMAGE_NAME."
    echo "$FAILURE_REASON"
  fi
else
  BUILD_FAILED=1
  FAILURE_REASON="Error: lb build did not complete successfully (exit $LB_STATUS)."
  echo "$FAILURE_REASON"
fi

END=$(date +%s)
DURATION=$((END - START))
TOTAL_DURATION="Done in $(date -d@$DURATION -u +%H:%M:%S)."
echo $TOTAL_DURATION
echo $TOTAL_DURATION >> "$IMAGE_NAME.build.log"
tail -n 100 "$IMAGE_NAME.build.log" > "$TARGET_DIR/$IMAGE_NAME.tail100.build.log.txt"
cp -v "$IMAGE_NAME.build.log" "$TARGET_DIR/$IMAGE_NAME.build.log.txt"

exit $BUILD_FAILED
