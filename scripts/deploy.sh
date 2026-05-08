#!/usr/bin/env bash
# Note API 部署辅助脚本 — 针对 Ubuntu Server（apt + systemd）
# 其他 Debian 系可尝试使用；非 systemd 环境请只使用 build-only 并自行守护进程。
#
# 【机密不要写在本文件里】请复制 scripts/deploy.local.env.example 为
#   scripts/deploy.local.env
# 填写 MYSQL_* 等，chmod 600；该文件已列入 server/.gitignore。
# 也可用环境变量覆盖下方各项。

set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
SERVER_ROOT="$(cd "${SCRIPT_DIR}/.." && pwd)"

LOCAL_ENV_FILE="${SCRIPT_DIR}/deploy.local.env"
if [[ -f "${LOCAL_ENV_FILE}" ]]; then
  # shellcheck disable=SC1090
  source "${LOCAL_ENV_FILE}"
fi

# 使用 sudo 时 root 的 PATH 往往不含官方 Go；从官网解压到 /usr/local/go 时需前置该目录
if [[ -x /usr/local/go/bin/go ]]; then
  export PATH="/usr/local/go/bin:${PATH}"
fi

DEPLOY_ROOT="${DEPLOY_ROOT:-/opt/noteapi}"
BIN_NAME="${BIN_NAME:-noteapi}"
BIN_PATH="${DEPLOY_ROOT}/bin/${BIN_NAME}"
ENV_FILE="${ENV_FILE:-/etc/noteapi.env}"
SERVICE_NAME="${SERVICE_NAME:-noteapi}"
GIT_REMOTE_PULL="${GIT_REMOTE_PULL:-1}"

MYSQL_HOST="${MYSQL_HOST:-127.0.0.1}"
MYSQL_PORT="${MYSQL_PORT:-3306}"

# 中国大陆等网络访问 proxy.golang.org 易超时；未指定时使用国内代理与校验镜像（可用环境变量覆盖）
: "${GOPROXY:=https://goproxy.cn,direct}"
: "${GOSUMDB:=sum.golang.google.cn}"
export GOPROXY GOSUMDB

log() { printf '[deploy] %s\n' "$*"; }
die() { printf '[deploy] ERROR: %s\n' "$*" >&2; exit 1; }

require_root_for_systemd() {
  if [[ "${EUID:-0}" -ne 0 ]]; then
    die "请使用 root 运行（或使用 sudo），以便写入 ${DEPLOY_ROOT}、${ENV_FILE}、systemd"
  fi
}

ensure_go() {
  if ! command -v go >/dev/null 2>&1 && [[ -x /usr/local/go/bin/go ]]; then
    export PATH="/usr/local/go/bin:${PATH}"
  fi
  if ! command -v go >/dev/null 2>&1; then
    die "未检测到 go。若已安装到 /usr/local/go，请确认存在 /usr/local/go/bin/go；否则请安装 Go 1.22+： https://go.dev/dl （sudo 下若仍报错，可先执行: export PATH=/usr/local/go/bin:\$PATH）"
  fi
  go version
}

build_binary() {
  ensure_go
  mkdir -p "$(dirname "${BIN_PATH}")"
  log "编译: ${SERVER_ROOT}/cmd/noteapi -> ${BIN_PATH}（GOPROXY=${GOPROXY}）"
  (cd "${SERVER_ROOT}" && CGO_ENABLED=0 go build -trimpath -ldflags="-s -w" -o "${BIN_PATH}" ./cmd/noteapi)
  chmod 755 "${BIN_PATH}"
  log "编译完成"
}

install_systemd_unit() {
  require_root_for_systemd
  local unit_src="${SCRIPT_DIR}/noteapi.service"
  [[ -f "${unit_src}" ]] || die "缺少 ${unit_src}"
  sed -e "s|__BIN_PATH__|${BIN_PATH}|g" \
      -e "s|__ENV_FILE__|${ENV_FILE}|g" \
      -e "s|__DEPLOY_ROOT__|${DEPLOY_ROOT}|g" \
      "${unit_src}" > "/etc/systemd/system/${SERVICE_NAME}.service"
  systemctl daemon-reload
  systemctl enable "${SERVICE_NAME}.service"
  log "已安装 systemd 单元 /etc/systemd/system/${SERVICE_NAME}.service"
}

cmd_first_time() {
  require_root_for_systemd
  log "=== 首次部署 ==="
  apt-get update -qq
  DEBIAN_FRONTEND=noninteractive apt-get install -y -qq git ca-certificates curl >/dev/null || true

  mkdir -p "${DEPLOY_ROOT}/bin"
  build_binary
  install_systemd_unit

  if [[ ! -f "${ENV_FILE}" ]]; then
    log "创建示例环境文件 ${ENV_FILE}（请务必编辑真实密钥与 MYSQL_DSN）"
    if [[ -f "${SERVER_ROOT}/.env.example" ]]; then
      cp "${SERVER_ROOT}/.env.example" "${ENV_FILE}"
      chmod 600 "${ENV_FILE}"
    else
      touch "${ENV_FILE}"
      chmod 600 "${ENV_FILE}"
    fi
  fi

  log ""
  log "后续手工步骤："
  log "  1) 编辑 ${ENV_FILE}（至少 MYSQL_DSN、JWT_SECRET、业务密钥）"
  log "  2) MySQL 执行迁移:"
  log "       mysql ... < ${SERVER_ROOT}/migrations/001_init.sql"
  log "       mysql ... < ${SERVER_ROOT}/migrations/002_user_identities.sql"
  log "  3) systemctl start ${SERVICE_NAME} && systemctl status ${SERVICE_NAME}"
  log "  4) 配置 Nginx/Caddy 反代到 LISTEN_ADDR（默认 :8080）"
  log ""
}

cmd_update() {
  require_root_for_systemd
  log "=== 更新部署 ==="
  if [[ "${GIT_REMOTE_PULL}" == "1" ]] && [[ -d "${SERVER_ROOT}/.git" ]]; then
    log "git pull（SERVER_ROOT=${SERVER_ROOT}）"
    (cd "${SERVER_ROOT}" && git pull --ff-only)
  fi
  build_binary
  systemctl restart "${SERVICE_NAME}.service"
  log "已重启 ${SERVICE_NAME}"
  systemctl --no-pager -l status "${SERVICE_NAME}.service" || true
}

cmd_build_only() {
  log "=== 仅编译（不写 systemd）==="
  mkdir -p "$(dirname "${BIN_PATH}")"
  build_binary
  log "二进制: ${BIN_PATH}"
}

cmd_migrate() {
  [[ -n "${MYSQL_USER:-}" ]] || die "请在 deploy.local.env 中设置 MYSQL_USER（或导出环境变量）"
  [[ -n "${MYSQL_PASSWORD:-}" ]] || die "请在 deploy.local.env 中设置 MYSQL_PASSWORD"
  [[ -n "${MYSQL_DATABASE:-}" ]] || die "请在 deploy.local.env 中设置 MYSQL_DATABASE"
  command -v mysql >/dev/null 2>&1 || die "未安装 mysql 客户端，请执行: apt install -y mysql-client"

  export MYSQL_PWD="${MYSQL_PASSWORD}"
  run_sql() {
    local f="$1"
    log "执行迁移: ${f}"
    mysql -h"${MYSQL_HOST}" -P"${MYSQL_PORT}" -u"${MYSQL_USER}" "${MYSQL_DATABASE}" < "${f}"
  }
  run_sql "${SERVER_ROOT}/migrations/001_init.sql"
  run_sql "${SERVER_ROOT}/migrations/002_user_identities.sql"
  unset MYSQL_PWD
  log "迁移完成（001 + 002）"
}

usage() {
  cat <<EOF
用法: $(basename "$0") <命令>

命令:
  first-time   首次安装依赖、编译、安装 systemd（需 root）
  update       git pull（若存在 .git）、编译、重启服务（需 root）
  build-only   仅编译到 \${DEPLOY_ROOT}/bin/\${BIN_NAME}（默认不需 root）
  migrate      按 deploy.local.env 中的 MYSQL_* 执行 migrations/001、002（需 root；需 mysql 客户端）

机密配置（勿提交 Git）:
  复制 scripts/deploy.local.env.example -> scripts/deploy.local.env 并填写

环境变量（可选）:
  DEPLOY_ROOT=${DEPLOY_ROOT}
  BIN_PATH 由 DEPLOY_ROOT/BIN_NAME 推导
  ENV_FILE=${ENV_FILE}
  SERVICE_NAME=${SERVICE_NAME}
  GIT_REMOTE_PULL=${GIT_REMOTE_PULL}   # update 时是否 git pull，设为 0 可跳过

示例:
  sudo DEPLOY_ROOT=/opt/noteapi ./scripts/deploy.sh first-time
  sudo ./scripts/deploy.sh update
EOF
}

main() {
  local sub="${1:-}"
  case "${sub}" in
    first-time) cmd_first_time ;;
    update)     cmd_update ;;
    build-only) cmd_build_only ;;
    migrate)    cmd_migrate ;;
    ""|-h|--help|help) usage ;;
    *) die "未知命令: ${sub}" ;;
  esac
}

main "$@"
