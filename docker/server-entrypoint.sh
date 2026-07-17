#!/bin/sh
set -e

MONGODB_URI="${MONGODB_URI:-mongodb://mongodb:27017}"
REDIS_ADDR="${REDIS_ADDR:-redis:6379}"
REDIS_PASSWORD="${REDIS_PASSWORD:-}"
HTTP_ADDR="${HTTP_ADDR:-:8080}"
GRPC_ADDR="${GRPC_ADDR:-:9000}"
AUTH_TOKEN="${AUTH_TOKEN:-change-me-in-production}"
JWT_SECRET="${JWT_SECRET:-super-secret-jwt-key}"
ADMIN_USER="${ADMIN_USER:-admin}"
ADMIN_PASS="${ADMIN_PASS:-admin123}"
SCANNER_IMAGE="${SCANNER_IMAGE:-nscan-scanner:latest}"
QUEUE_MODE="${QUEUE_MODE:-legacy}"

mkdir -p /app/configs /app/certs

# 自动生成自签名证书（如果不存在）
if [ ! -f /app/certs/server.crt ]; then
    openssl req -x509 -newkey rsa:2048 -nodes \
        -keyout /app/certs/server.key \
        -out /app/certs/server.crt \
        -days 3650 \
        -subj "/CN=nscan-server" 2>/dev/null || true
fi

cat > /app/configs/server.yaml <<EOF
server:
  http_addr: "${HTTP_ADDR}"
  grpc_addr: "${GRPC_ADDR}"
  tls:
    enabled: true
    cert_file: "/app/certs/server.crt"
    key_file: "/app/certs/server.key"
  auth_token: "${AUTH_TOKEN}"
  admin_user: "${ADMIN_USER}"
  admin_pass: "${ADMIN_PASS}"
  jwt_secret: "${JWT_SECRET}"
  scanner_image: "${SCANNER_IMAGE}"

mongodb:
  uri: "${MONGODB_URI}"
  database: "nscan"

redis:
  addr: "${REDIS_ADDR}"
  password: "${REDIS_PASSWORD}"
  db: 0

log:
  level: "info"
  format: "json"

queue:
  mode: "${QUEUE_MODE}"
EOF

# 启动 nginx（前端）
rm -f /etc/nginx/sites-enabled/default
nginx

# 启动 server（前台）
exec nscan-server --config /app/configs/server.yaml
