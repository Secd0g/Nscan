#!/bin/sh
set -eu

PATH="/usr/local/bin:/opt/homebrew/bin:$PATH"
export PATH

ROOT_DIR=$(CDPATH= cd -- "$(dirname -- "$0")/.." && pwd)
ENV_FILE=${ENV_FILE:-$ROOT_DIR/deploy/.env.production}
COMPOSE_FILE=$ROOT_DIR/deploy/docker-compose.prod.yaml

if [ ! -f "$ENV_FILE" ]; then
  echo "缺少 $ENV_FILE，请先复制 .env.production.example 并填写生产配置。" >&2
  exit 1
fi

# shellcheck disable=SC1090
. "$ENV_FILE"
: "${SERVER_IMAGE:?SERVER_IMAGE 未配置}"
: "${SCANNER_IMAGE:?SCANNER_IMAGE 未配置}"
: "${DOMAIN:?DOMAIN 未配置}"

for value in "$AUTH_TOKEN" "$JWT_SECRET" "$ADMIN_PASS" "$MONGO_ROOT_PASS" "$REDIS_PASSWORD"; do
  case "$value" in
    ""|*CHANGE_ME*|change-me-in-production|super-secret-jwt-key|admin123)
      echo "检测到未替换的生产密钥，请检查 $ENV_FILE" >&2
      exit 1
      ;;
  esac
done

command -v docker >/dev/null 2>&1 || { echo "未找到 Docker" >&2; exit 1; }
docker compose version >/dev/null 2>&1 || { echo "未找到 Docker Compose 插件" >&2; exit 1; }

COMPOSE="docker compose --env-file $ENV_FILE -f $COMPOSE_FILE"
if [ "${BUILD_LOCAL:-false}" = "true" ]; then
  "$ROOT_DIR/deploy/build-images.sh"
elif [ "${USE_LOCAL_IMAGES:-false}" = "true" ]; then
  echo "使用本地镜像：$SERVER_IMAGE / $SCANNER_IMAGE"
else
  docker pull "$SERVER_IMAGE"
  docker pull "$SCANNER_IMAGE"
fi

$COMPOSE up -d --remove-orphans

i=0
while [ "$i" -lt 30 ]; do
  if "$ROOT_DIR/deploy/healthcheck.sh" >/dev/null 2>&1; then
    echo "nscan 部署完成：http://$DOMAIN:${WEB_PORT:-80}"
    exit 0
  fi
  i=$((i + 1))
  sleep 2
done

echo "部署后健康检查失败，最近日志：" >&2
$COMPOSE logs --tail=100 mongodb redis server scanner gateway >&2 || true
exit 1
