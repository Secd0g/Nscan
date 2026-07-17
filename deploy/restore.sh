#!/bin/sh
set -eu

if [ "$#" -ne 1 ]; then
  echo "用法：$0 backups/YYYYMMDD-HHMMSS" >&2
  exit 2
fi

ROOT_DIR=$(CDPATH= cd -- "$(dirname -- "$0")/.." && pwd)
ENV_FILE=${ENV_FILE:-$ROOT_DIR/deploy/.env.production}
. "$ENV_FILE"
COMPOSE="docker compose --env-file $ENV_FILE -f $ROOT_DIR/deploy/docker-compose.prod.yaml"
BACKUP_DIR=$1

test -f "$BACKUP_DIR/mongodb.archive.gz"
$COMPOSE exec -T mongodb mongorestore --username "$MONGO_ROOT_USER" --password "$MONGO_ROOT_PASS" --authenticationDatabase admin --drop --archive --gzip < "$BACKUP_DIR/mongodb.archive.gz"
echo "MongoDB 恢复完成。Redis RDB 需要在维护窗口替换后重启 Redis：$BACKUP_DIR/redis.rdb"
