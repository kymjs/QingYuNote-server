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
#
# 环境文件不用 bash source：MYSQL_DSN 中含 tcp(...) 时未加引号会导致 syntax error；
# 此处按 KEY=VALUE 逐行解析并用 printf %q 导出，与 systemd EnvironmentFile 常见写法兼容。
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

load_env_file_safe() {
  local path="$1"
  [[ -n "${path}" ]] || return 0
  [[ -f "${path}" ]] || return 0
  [[ -r "${path}" ]] || return 0

  local line key val
  while IFS= read -r line || [[ -n "${line}" ]]; do
    line="${line%$'\r'}"
    line="${line#"${line%%[![:space:]]*}"}"
    [[ -z "${line}" || "${line}" =~ ^[[:space:]]*# ]] && continue
    if [[ "${line}" =~ ^[[:space:]]*export[[:space:]]+(.+)$ ]]; then
      line="${BASH_REMATCH[1]}"
    fi
    [[ "${line}" == *'='* ]] || continue
    key="${line%%=*}"
    val="${line#*=}"
    [[ "${key}" =~ ^[A-Za-z_][A-Za-z0-9_]*$ ]] || continue
    if [[ "${val}" =~ ^\'(.*)\'$ ]]; then
      val="${BASH_REMATCH[1]}"
    elif [[ "${val}" =~ ^\"(.*)\"$ ]]; then
      val="${BASH_REMATCH[1]}"
    fi
    eval "$(printf 'export %s=%q' "${key}" "${val}")"
  done < "${path}"
}

if [[ -n "${NOTEAPI_ENV_FILE:-}" ]]; then
  load_env_file_safe "${NOTEAPI_ENV_FILE}"
fi
load_env_file_safe "${ROOT}/.env"
load_env_file_safe "/etc/noteapi.env"

exec go run ./cmd/issue_redemption_codes "$@"
