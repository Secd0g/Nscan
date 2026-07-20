#!/bin/sh
set -e

SERVER_ADDR="${SERVER_ADDR:-localhost:9000}"
TOKEN="${TOKEN:-change-me-in-production}"
NODE_NAME="${NODE_NAME:-node-$(cat /proc/sys/kernel/random/uuid | cut -c1-8)}"
MAX_TASKS="${MAX_TASKS:-5}"
REDIS_ADDR="${REDIS_ADDR:-}"
REDIS_PASS="${REDIS_PASS:-}"
QUEUE_WORKERS="${QUEUE_WORKERS:-1}"
CAPABILITIES="${CAPABILITIES:-search,subdomain,shuffledns,bbot,findomain,port,http,crawler,nuclei,brute,dir,sensitive,ai-pentest}"

mkdir -p /app/configs

cat > /app/configs/scanner.yaml <<EOF
scanner:
  name: "${NODE_NAME}"
  server_addr: "${SERVER_ADDR}"
  token: "${TOKEN}"
  max_tasks: ${MAX_TASKS}
  capabilities: [${CAPABILITIES}]
  tls:
    enabled: true
    insecure_skip_verify: true
  queue:
    redis_addr: "${REDIS_ADDR}"
    redis_pass: "${REDIS_PASS}"
    num_workers: ${QUEUE_WORKERS}
log:
  level: info
  format: json
EOF

exec nscan-scanner --config /app/configs/scanner.yaml
