#!/bin/sh
set -eu

ROOT_DIR=$(CDPATH= cd -- "$(dirname -- "$0")/.." && pwd)
ENV_FILE=${ENV_FILE:-$ROOT_DIR/deploy/.env.production}
. "$ENV_FILE"
COMPOSE="docker compose --env-file $ENV_FILE -f $ROOT_DIR/deploy/docker-compose.prod.yaml"
STAMP=$(date +%Y%m%d-%H%M%S)
BACKUP_DIR=${BACKUP_DIR:-$ROOT_DIR/backups/$STAMP}
mkdir -p "$BACKUP_DIR"

$COMPOSE exec -T mongodb mongodump --username "$MONGO_ROOT_USER" --password "$MONGO_ROOT_PASS" --authenticationDatabase admin --db nscan --archive --gzip > "$BACKUP_DIR/mongodb.archive.gz"
$COMPOSE exec -T redis redis-cli --rdb /tmp/nscan-dump.rdb >/dev/null
$COMPOSE cp redis:/tmp/nscan-dump.rdb "$BACKUP_DIR/redis.rdb"
$COMPOSE exec -T redis rm -f /tmp/nscan-dump.rdb

echo "备份完成：$BACKUP_DIR"
