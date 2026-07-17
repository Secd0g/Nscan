#!/bin/sh
set -eu

PATH="/usr/local/bin:/opt/homebrew/bin:$PATH"
export PATH

ROOT_DIR=$(CDPATH= cd -- "$(dirname -- "$0")/.." && pwd)
ENV_FILE=${ENV_FILE:-$ROOT_DIR/deploy/.env.production}

if [ ! -f "$ENV_FILE" ]; then
  echo "缺少配置文件：$ENV_FILE" >&2
  echo "请先复制 deploy/.env.production.example 为 deploy/.env.production，并填写 DOMAIN。" >&2
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

echo "正在部署 nscan..."
ENV_FILE="$ENV_FILE" BUILD_LOCAL=true USE_LOCAL_IMAGES=true "$ROOT_DIR/deploy/deploy.sh"

. "$ENV_FILE"
echo "部署完成： http://${DOMAIN}"
echo "管理员账号：${ADMIN_USER}"
echo "管理员密码：${ADMIN_PASS}"
