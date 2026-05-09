#!/usr/bin/env bash
# 在服务器项目根目录执行：根据 .env 中的 MYSQL_DSN、REDEMPTION_ISSUE_SECRET、
# FEISHU_REDEMPTION_WEBHOOK_URL 签发兑换码并逐条推送到飞书。
set -euo pipefail
ROOT="$(cd "$(dirname "$0")/.." && pwd)"
cd "$ROOT"
if [[ -f .env ]]; then
  set -a
  # shellcheck disable=SC1091
  source .env
  set +a
fi
exec go run ./cmd/issue_redemption_codes "$@"
