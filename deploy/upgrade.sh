#!/bin/sh
set -eu

if [ "$#" -ne 1 ]; then
  echo "用法：$0 <版本号>" >&2
  exit 2
fi

ROOT_DIR=$(CDPATH= cd -- "$(dirname -- "$0")/.." && pwd)
ENV_FILE=${ENV_FILE:-$ROOT_DIR/deploy/.env.production}
OLD_TAG=$(grep '^IMAGE_TAG=' "$ENV_FILE" | cut -d= -f2-)
NEW_TAG=$1
sed "s/^IMAGE_TAG=.*/IMAGE_TAG=$NEW_TAG/" "$ENV_FILE" > "$ENV_FILE.tmp"
sed -i.bak "s#^SERVER_IMAGE=.*#SERVER_IMAGE=\${REGISTRY}/server:${NEW_TAG}#; s#^SCANNER_IMAGE=.*#SCANNER_IMAGE=\${REGISTRY}/scanner:${NEW_TAG}#" "$ENV_FILE.tmp"
mv "$ENV_FILE.tmp" "$ENV_FILE"

if ! "$ROOT_DIR/deploy/deploy.sh"; then
  echo "升级失败，回滚到 $OLD_TAG" >&2
  "$ROOT_DIR/deploy/rollback.sh" "$OLD_TAG"
  exit 1
fi
