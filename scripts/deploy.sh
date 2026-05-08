#!/usr/bin/env bash
# Note API 部署辅助脚本 — 针对 Ubuntu Server（apt + systemd）
# 其他 Debian 系可尝试使用；非 systemd 环境请只使用 build-only 并自行守护进程。
# 使用前请根据服务器修改下列默认值，或通过环境变量覆盖。

set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
SERVER_ROOT="$(cd "${SCRIPT_DIR}/.." && pwd)"
DEPLOY_ROOT="${DEPLOY_ROOT:-/opt/noteapi}"
BIN_NAME="${BIN_NAME:-noteapi}"
BIN_PATH="${DEPLOY_ROOT}/bin/${BIN_NAME}"
ENV_FILE="${ENV_FILE:-/etc/noteapi.env}"
SERVICE_NAME="${SERVICE_NAME:-noteapi}"
GIT_REMOTE_PULL="${GIT_REMOTE_PULL:-1}"

log() { printf '[deploy] %s\n' "$*"; }
die() { printf '[deploy] ERROR: %s\n' "$*" >&2; exit 1; }

require_root_for_systemd() {
  if [[ "${EUID:-0}" -ne 0 ]]; then
    die "请使用 root 运行（或使用 sudo），以便写入 ${DEPLOY_ROOT}、${ENV_FILE}、systemd"
  fi
}

ensure_go() {
  if ! command -v go >/dev/null 2>&1; then
    die "未检测到 go，请先安装 Go 1.22+（例如: apt install golang-go 或从 https://go.dev/dl 安装）"
  fi
  go version
}

build_binary() {
  ensure_go
  mkdir -p "$(dirname "${BIN_PATH}")"
  log "编译: ${SERVER_ROOT}/cmd/noteapi -> ${BIN_PATH}"
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

usage() {
  cat <<EOF
用法: $(basename "$0") <命令>

命令:
  first-time   首次安装依赖、编译、安装 systemd（需 root）
  update       git pull（若存在 .git）、编译、重启服务（需 root）
  build-only   仅编译到 \${DEPLOY_ROOT}/bin/\${BIN_NAME}（默认不需 root）

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
    ""|-h|--help|help) usage ;;
    *) die "未知命令: ${sub}" ;;
  esac
}

main "$@"
