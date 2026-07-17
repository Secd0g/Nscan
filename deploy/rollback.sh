#!/bin/sh
set -eu

if [ "$#" -ne 1 ]; then
  echo "用法：$0 <版本号>" >&2
  exit 2
fi

ROOT_DIR=$(CDPATH= cd -- "$(dirname -- "$0")/.." && pwd)
ENV_FILE=${ENV_FILE:-$ROOT_DIR/deploy/.env.production}
TAG=$1
sed "s/^IMAGE_TAG=.*/IMAGE_TAG=$TAG/" "$ENV_FILE" > "$ENV_FILE.tmp"
sed -i.bak "s#^SERVER_IMAGE=.*#SERVER_IMAGE=\${REGISTRY}/server:${TAG}#; s#^SCANNER_IMAGE=.*#SCANNER_IMAGE=\${REGISTRY}/scanner:${TAG}#" "$ENV_FILE.tmp"
mv "$ENV_FILE.tmp" "$ENV_FILE"
"$ROOT_DIR/deploy/deploy.sh"
