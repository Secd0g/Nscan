#!/bin/sh
set -eu

ROOT_DIR=$(CDPATH= cd -- "$(dirname -- "$0")/.." && pwd)
ENV_FILE=${ENV_FILE:-$ROOT_DIR/deploy/.env.production}
. "$ENV_FILE"
COMPOSE="docker compose --env-file $ENV_FILE -f $ROOT_DIR/deploy/docker-compose.prod.yaml"

$COMPOSE ps
$COMPOSE exec -T server curl -fsS http://127.0.0.1/healthz >/dev/null

if ! $COMPOSE ps --status running --services | grep -qx gateway; then
  echo "gateway 未运行" >&2
  exit 1
fi

echo "nscan 健康检查通过"
