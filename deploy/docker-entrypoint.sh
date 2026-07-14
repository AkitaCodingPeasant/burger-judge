#!/bin/sh
set -eu

# 在 API 容器內啟動判題專用 Docker daemon。
dockerd-entrypoint.sh dockerd > /var/log/dockerd.log 2>&1 &

# 等待 daemon 可用，避免 API 收到提交後才發現 Docker 尚未啟動。
attempt=0
until docker info > /dev/null 2>&1; do
  attempt=$((attempt + 1))
  if [ "$attempt" -ge 60 ]; then
    echo "Docker daemon did not become ready" >&2
    cat /var/log/dockerd.log >&2
    exit 1
  fi
  sleep 1
done

exec /app/regs

