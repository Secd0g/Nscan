#!/bin/sh
set -eu

PATH="/usr/local/bin:/opt/homebrew/bin:$PATH"
export PATH

ROOT_DIR=$(CDPATH= cd -- "$(dirname -- "$0")/.." && pwd)
ENV_FILE=${ENV_FILE:-$ROOT_DIR/deploy/.env.production}

if [ ! -f "$ENV_FILE" ]; then
  cp "$ROOT_DIR/deploy/.env.production.example" "$ENV_FILE"
  echo "已创建配置文件：$ENV_FILE，请填写 DOMAIN 后重新执行。" >&2
  exit 1
fi

set_env() {
  key=$1
  value=$2
  escaped=$(printf '%s' "$value" | sed 's/[\\&|]/\\&/g')
  sed -i.bak "s|^${key}=.*|${key}=${escaped}|" "$ENV_FILE"
}

random_hex() {
  openssl rand -hex "$1" 2>/dev/null || od -An -N"$1" -tx1 /dev/urandom | tr -d ' \n'
}

# 用户只需配置域名和镜像仓库；其余密钥首次安装时自动生成。
. "$ENV_FILE"
if [ "${RESET_MONGO:-false}" = "true" ]; then
  echo "RESET_MONGO=true：清理当前项目 MongoDB 数据卷。"
  COMPOSE_PROJECT_NAME="${COMPOSE_PROJECT_NAME:-nscan-production}"
  docker compose --project-name "$COMPOSE_PROJECT_NAME" --env-file "$ENV_FILE" -f "$ROOT_DIR/deploy/docker-compose.prod.yaml" down
  docker volume rm "${COMPOSE_PROJECT_NAME}_mongo_data" 2>/dev/null || true
fi
case "${DOMAIN:-}" in
  ""|nscan.example.com|localhost)
    echo "请先在 $ENV_FILE 中填写真实 DOMAIN" >&2
    exit 1
    ;;
esac
case "${AUTH_TOKEN:-}" in ""|*CHANGE_ME*) set_env AUTH_TOKEN "$(random_hex 20)" ;; esac
case "${JWT_SECRET:-}" in ""|*CHANGE_ME*) set_env JWT_SECRET "$(random_hex 32)" ;; esac
case "${ADMIN_PASS:-}" in ""|*CHANGE_ME*) set_env ADMIN_PASS "$(random_hex 12)" ;; esac
case "${MONGO_ROOT_PASS:-}" in ""|*CHANGE_ME*) set_env MONGO_ROOT_PASS "$(random_hex 20)" ;; esac
case "${REDIS_PASSWORD:-}" in ""|*CHANGE_ME*) set_env REDIS_PASSWORD "$(random_hex 20)" ;; esac
rm -f "$ENV_FILE.bak"

CERT_DIR="$ROOT_DIR/deploy/certs"
mkdir -p "$CERT_DIR"
if [ ! -f "$CERT_DIR/server.crt" ] || [ ! -f "$CERT_DIR/server.key" ]; then
  echo "未检测到证书，生成自签名证书..."
  openssl req -x509 -nodes -newkey rsa:2048 \
    -keyout "$CERT_DIR/server.key" \
    -out "$CERT_DIR/server.crt" \
    -days 3650 \
    -subj "/CN=$DOMAIN" \
    -addext "subjectAltName=DNS:$DOMAIN" >/dev/null 2>&1
  chmod 600 "$CERT_DIR/server.key"
fi

echo "正在部署 nscan..."
if docker image inspect nscan-server:release >/dev/null 2>&1 && \
   docker image inspect nscan-scanner:release >/dev/null 2>&1; then
  BUILD_LOCAL=false
  echo "检测到本地镜像，跳过重复构建。"
else
  BUILD_LOCAL=true
  echo "首次部署，开始构建镜像。"
fi
ENV_FILE="$ENV_FILE" BUILD_LOCAL="$BUILD_LOCAL" USE_LOCAL_IMAGES=true "$ROOT_DIR/deploy/deploy.sh"

. "$ENV_FILE"
echo "部署完成： https://${DOMAIN}"
echo "管理员账号：${ADMIN_USER}"
echo "管理员密码：${ADMIN_PASS}"
