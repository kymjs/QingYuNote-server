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
ISSUE_TOOL_NAME="${ISSUE_TOOL_NAME:-issue_redemption_codes}"
ISSUE_TOOL_PATH="${DEPLOY_ROOT}/bin/${ISSUE_TOOL_NAME}"
ENV_FILE="${ENV_FILE:-/etc/noteapi.env}"
SERVICE_NAME="${SERVICE_NAME:-noteapi}"
GIT_REMOTE_PULL="${GIT_REMOTE_PULL:-1}"
# update 时若为 1，则在编译前自动执行 migrate（需在 deploy.local.env 配置 MYSQL_*）
RUN_MIGRATE_ON_UPDATE="${RUN_MIGRATE_ON_UPDATE:-0}"
# update 时若为 1，在覆盖二进制前将现有 noteapi / issue_redemption_codes 复制为 *.prev（便于 rollback）
BACKUP_BIN_ON_UPDATE="${BACKUP_BIN_ON_UPDATE:-1}"
# 若设置（如 http://127.0.0.1:9443/healthz），update 在重启成功后会 curl 做一次冒烟（需本机可访问监听地址）
NOTEAPI_HEALTH_URL="${NOTEAPI_HEALTH_URL:-}"

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
  mkdir -p "$(dirname "${ISSUE_TOOL_PATH}")"
  log "编译 noteapi: ${SERVER_ROOT}/cmd/noteapi -> ${BIN_PATH}（GOPROXY=${GOPROXY}）"
  (cd "${SERVER_ROOT}" && CGO_ENABLED=0 go build -trimpath -ldflags="-s -w" -o "${BIN_PATH}" ./cmd/noteapi)
  log "编译兑换码签发工具: ./cmd/issue_redemption_codes -> ${ISSUE_TOOL_PATH}"
  (cd "${SERVER_ROOT}" && CGO_ENABLED=0 go build -trimpath -ldflags="-s -w" -o "${ISSUE_TOOL_PATH}" ./cmd/issue_redemption_codes)
  chmod 755 "${BIN_PATH}" "${ISSUE_TOOL_PATH}"
  log "编译完成（noteapi + ${ISSUE_TOOL_NAME}）"
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
  log "  1) 编辑 ${ENV_FILE}（至少 MYSQL_DSN、JWT_SECRET、业务密钥；头像上传需 AVATAR_WEBDAV_USERNAME/PASSWORD；兑换码签发需 REDEMPTION_ISSUE_SECRET、FEISHU_REDEMPTION_WEBHOOK_URL，见 .env.example）"
  log "  2) MySQL 迁移（推荐在填写 scripts/deploy.local.env 的 MYSQL_* 后执行）:"
  log "       sudo ${SCRIPT_DIR}/deploy.sh migrate"
  log "     （按文件名排序执行 migrations/[0-9][0-9][0-9]_*.sql；详见 DEPLOYMENT.md）"
  log "  3) systemctl start ${SERVICE_NAME} && systemctl status ${SERVICE_NAME}"
  log "  4) 配置 Nginx/Caddy 反代到 LISTEN_ADDR（默认 :9443）"
  log "  5) 签发兑换码（需在同一主机 source ${ENV_FILE} 或导出 MYSQL_DSN、REDEMPTION_ISSUE_SECRET）:"
  log "       ${ISSUE_TOOL_PATH} -plan monthly -count 1 -issuer-secret \"\$REDEMPTION_ISSUE_SECRET\""
  log ""
}

cmd_update() {
  require_root_for_systemd
  log "=== 更新部署 ==="
  if [[ "${GIT_REMOTE_PULL}" == "1" ]] && [[ -d "${SERVER_ROOT}/.git" ]]; then
    log "git pull（SERVER_ROOT=${SERVER_ROOT}）"
    # sudo 下 root 使用 /root/.ssh，通常没有部署用户的密钥；改由发起 sudo 的用户执行 pull
    if [[ -n "${SUDO_USER:-}" ]] && [[ "${SUDO_USER}" != root ]] && id "${SUDO_USER}" >/dev/null 2>&1; then
      (cd "${SERVER_ROOT}" && sudo -u "${SUDO_USER}" -H git pull --ff-only)
    else
      (cd "${SERVER_ROOT}" && git pull --ff-only)
    fi
  fi
  if [[ "${RUN_MIGRATE_ON_UPDATE}" == "1" ]]; then
    log "RUN_MIGRATE_ON_UPDATE=1：执行数据库迁移（migrations 目录全部 *.sql）"
    cmd_migrate
  fi
  if [[ "${BACKUP_BIN_ON_UPDATE}" == "1" ]]; then
    if [[ -f "${BIN_PATH}" ]]; then
      log "备份现有二进制: ${BIN_PATH} -> ${BIN_PATH}.prev"
      cp -a "${BIN_PATH}" "${BIN_PATH}.prev"
    fi
    if [[ -f "${ISSUE_TOOL_PATH}" ]]; then
      log "备份兑换码工具: ${ISSUE_TOOL_PATH} -> ${ISSUE_TOOL_PATH}.prev"
      cp -a "${ISSUE_TOOL_PATH}" "${ISSUE_TOOL_PATH}.prev"
    fi
  fi
  build_binary
  systemctl restart "${SERVICE_NAME}.service"
  sleep 0.5
  if ! systemctl is-active --quiet "${SERVICE_NAME}.service"; then
    log "最近日志（${SERVICE_NAME}）："
    journalctl -u "${SERVICE_NAME}.service" -n 30 --no-pager 2>/dev/null || true
    die "${SERVICE_NAME} 未能进入 active，请检查配置与日志"
  fi
  log "已重启 ${SERVICE_NAME}（active）"
  if [[ -n "${NOTEAPI_HEALTH_URL}" ]]; then
    if command -v curl >/dev/null 2>&1; then
      log "健康检查: curl -fsS ${NOTEAPI_HEALTH_URL}"
      curl -fsS --connect-timeout 3 --max-time 10 "${NOTEAPI_HEALTH_URL}" >/dev/null \
        && log "health 检查通过" \
        || log "health 检查失败（服务已 active，请确认 LISTEN_ADDR / 反代；可忽略或修正 NOTEAPI_HEALTH_URL）"
    else
      log "未安装 curl，跳过 NOTEAPI_HEALTH_URL 检查"
    fi
  fi
  systemctl --no-pager -l status "${SERVICE_NAME}.service" || true
}

cmd_build_only() {
  log "=== 仅编译（不写 systemd）==="
  mkdir -p "$(dirname "${BIN_PATH}")"
  mkdir -p "$(dirname "${ISSUE_TOOL_PATH}")"
  build_binary
  log "二进制: ${BIN_PATH}"
  log "兑换码签发: ${ISSUE_TOOL_PATH}"
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

  shopt -s nullglob
  local mig_files=("${SERVER_ROOT}/migrations/"[0-9][0-9][0-9]_*.sql)
  shopt -u nullglob
  if [[ ${#mig_files[@]} -eq 0 ]]; then
    unset MYSQL_PWD
    die "未找到 ${SERVER_ROOT}/migrations/[0-9][0-9][0-9]_*.sql"
  fi
  local sorted=()
  mapfile -t sorted < <(printf '%s\n' "${mig_files[@]}" | LC_ALL=C sort -V)
  local f
  for f in "${sorted[@]}"; do
    [[ -n "${f}" ]] || continue
    run_sql "${f}"
  done
  unset MYSQL_PWD
  log "迁移完成（共 ${#sorted[@]} 个 SQL；003 等脚本可按列检测跳过重复 ALTER）"
}

cmd_rollback() {
  require_root_for_systemd
  local prev="${BIN_PATH}.prev"
  [[ -f "${prev}" ]] || die "不存在 ${prev}，无法回滚（请先成功执行过一次带 BACKUP_BIN_ON_UPDATE=1 的 update）"
  log "=== 回滚二进制（${prev} -> ${BIN_PATH}）==="
  cp -a "${prev}" "${BIN_PATH}"
  chmod 755 "${BIN_PATH}"
  local issue_prev="${ISSUE_TOOL_PATH}.prev"
  if [[ -f "${issue_prev}" ]]; then
    log "同时回滚兑换码工具: ${issue_prev} -> ${ISSUE_TOOL_PATH}"
    cp -a "${issue_prev}" "${ISSUE_TOOL_PATH}"
    chmod 755 "${ISSUE_TOOL_PATH}"
  fi
  systemctl restart "${SERVICE_NAME}.service"
  sleep 0.5
  if ! systemctl is-active --quiet "${SERVICE_NAME}.service"; then
    journalctl -u "${SERVICE_NAME}.service" -n 30 --no-pager 2>/dev/null || true
    die "${SERVICE_NAME} 未能进入 active，请检查日志"
  fi
  log "已回滚并重启 ${SERVICE_NAME}（active）"
  if [[ -n "${NOTEAPI_HEALTH_URL}" ]] && command -v curl >/dev/null 2>&1; then
    curl -fsS --connect-timeout 3 --max-time 10 "${NOTEAPI_HEALTH_URL}" >/dev/null \
      && log "health 检查通过" \
      || log "health 检查失败（请确认 NOTEAPI_HEALTH_URL）"
  fi
}

usage() {
  cat <<EOF
用法: $(basename "$0") <命令>

命令:
  first-time   首次安装依赖、编译、安装 systemd（需 root）
  update       git pull（若存在 .git）、编译、重启服务（需 root）；若 export RUN_MIGRATE_ON_UPDATE=1 则先执行 migrate
  build-only   仅编译 noteapi 与 issue_redemption_codes 到 \${DEPLOY_ROOT}/bin/（默认不需 root）
  migrate      按 deploy.local.env 中的 MYSQL_* 顺序执行 migrations/[0-9][0-9][0-9]_*.sql（需 mysql 客户端）
  rollback     用上次 update 备份的 *.prev 覆盖当前二进制并重启服务（需 root；依赖先前 BACKUP_BIN_ON_UPDATE=1）

机密配置（勿提交 Git）:
  复制 scripts/deploy.local.env.example -> scripts/deploy.local.env 并填写

环境变量（可选）:
  DEPLOY_ROOT=${DEPLOY_ROOT}
  BIN_PATH 由 DEPLOY_ROOT/BIN_NAME 推导
  ENV_FILE=${ENV_FILE}
  SERVICE_NAME=${SERVICE_NAME}
  GIT_REMOTE_PULL=${GIT_REMOTE_PULL}   # update 时是否 git pull，设为 0 可跳过
  RUN_MIGRATE_ON_UPDATE=${RUN_MIGRATE_ON_UPDATE}   # 设为 1 时 update 会先执行 migrate（须已配置 MYSQL_*）
  BACKUP_BIN_ON_UPDATE=${BACKUP_BIN_ON_UPDATE}     # 设为 0 可跳过编译前备份 noteapi/issue 工具为 .prev
  NOTEAPI_HEALTH_URL   # 例如 http://127.0.0.1:9443/healthz；设置后 update 重启成功会 curl 冒烟

示例:
  sudo DEPLOY_ROOT=/opt/noteapi ./scripts/deploy.sh first-time
  sudo ./scripts/deploy.sh update
  sudo ./scripts/deploy.sh rollback   # 新版本异常时恢复上一版二进制
EOF
}

main() {
  local sub="${1:-}"
  case "${sub}" in
    first-time) cmd_first_time ;;
    update)     cmd_update ;;
    build-only) cmd_build_only ;;
    migrate)    cmd_migrate ;;
    rollback)   cmd_rollback ;;
    ""|-h|--help|help) usage ;;
    *) die "未知命令: ${sub}" ;;
  esac
}

main "$@"
