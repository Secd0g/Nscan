#!/bin/sh
set -eu

ROOT_DIR=$(CDPATH= cd -- "$(dirname -- "$0")/.." && pwd)
ENV_FILE=${ENV_FILE:-$ROOT_DIR/deploy/.env.production}

if [ ! -f "$ENV_FILE" ]; then
  echo "缺少 $ENV_FILE，请先复制 .env.production.example" >&2
  exit 1
fi

# shellcheck disable=SC1090
. "$ENV_FILE"
: "${SERVER_IMAGE:?SERVER_IMAGE 未配置}"
: "${SCANNER_IMAGE:?SCANNER_IMAGE 未配置}"

cd "$ROOT_DIR"
GO_IMAGE=${GO_IMAGE:-public.ecr.aws/docker/library/golang:1.26-bookworm}
NODE_IMAGE=${NODE_IMAGE:-public.ecr.aws/docker/library/node:20-slim}
DEBIAN_IMAGE=${DEBIAN_IMAGE:-public.ecr.aws/docker/library/debian:bookworm-slim}

docker build --pull=false \
  --build-arg "GO_IMAGE=$GO_IMAGE" \
  --build-arg "NODE_IMAGE=$NODE_IMAGE" \
  --build-arg "DEBIAN_IMAGE=$DEBIAN_IMAGE" \
  -f Dockerfile.server -t "$SERVER_IMAGE" .
docker build --pull=false \
  --build-arg "GO_IMAGE=$GO_IMAGE" \
  --build-arg "DEBIAN_IMAGE=$DEBIAN_IMAGE" \
  --build-arg "INSTALL_AI_TOOLS=${INSTALL_AI_TOOLS:-false}" \
  --build-arg "INSTALL_OPTIONAL_TOOLS=${INSTALL_OPTIONAL_TOOLS:-true}" \
  -f Dockerfile.scanner -t "$SCANNER_IMAGE" .

if [ "${PUSH_IMAGES:-false}" = "true" ]; then
  docker push "$SERVER_IMAGE"
  docker push "$SCANNER_IMAGE"
fi

echo "镜像已完成："
echo "  $SERVER_IMAGE"
echo "  $SCANNER_IMAGE"
