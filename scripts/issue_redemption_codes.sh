#!/usr/bin/env bash
# 在服务器项目根目录执行：加载环境变量后签发兑换码（MYSQL_DSN、REDEMPTION_ISSUE_SECRET、
# FEISHU_REDEMPTION_WEBHOOK_URL 等）。
#
# 环境文件加载顺序（后者覆盖前者同名变量）：
#   1) $NOTEAPI_ENV_FILE（若设置且可读）
#   2) 项目根目录 .env（若可读）
#   3) /etc/noteapi.env（若可读；部署常用，普通用户无权限时需 sudo 执行本脚本）
#
# 默认使用国内模块代理（与 scripts/deploy.sh 一致）；可 export GOPROXY / GOSUMDB 覆盖。
set -euo pipefail

# sudo 后 PATH 常不含官方 Go；与 deploy.sh 一致
if [[ -x /usr/local/go/bin/go ]]; then
  export PATH="/usr/local/go/bin:${PATH}"
fi

ROOT="$(cd "$(dirname "$0")/.." && pwd)"
cd "$ROOT"

: "${GOPROXY:=https://goproxy.cn,direct}"
: "${GOSUMDB:=sum.golang.google.cn}"
export GOPROXY GOSUMDB

source_env_if_readable() {
  local path="$1"
  [[ -n "${path}" ]] || return 0
  [[ -f "${path}" ]] || return 0
  if [[ ! -r "${path}" ]]; then
    return 0
  fi
  set -a
  # shellcheck disable=SC1090
  source "${path}"
  set +a
}

if [[ -n "${NOTEAPI_ENV_FILE:-}" ]]; then
  source_env_if_readable "${NOTEAPI_ENV_FILE}"
fi
source_env_if_readable "${ROOT}/.env"
source_env_if_readable "/etc/noteapi.env"

exec go run ./cmd/issue_redemption_codes "$@"
