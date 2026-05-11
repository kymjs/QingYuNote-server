# Note API 部署说明（Ubuntu）

本文默认你的服务端为 **Ubuntu Server**（推荐 **22.04 / 24.04 LTS**，amd64），使用 **apt** 与 **systemd**。全新机器上的首次部署、日常更新、回滚要点如下。

---

## 0. 系统与网络（Ubuntu）

- 使用 `sudo` 执行安装与 `systemctl` 操作；生产环境建议仅开放 SSH 与 80/443。  
- **本机防火墙（UFW）**示例（确认 SSH 未断线后再 `enable`）：

  ```bash
  sudo ufw allow OpenSSH
  sudo ufw allow 80/tcp
  sudo ufw allow 443/tcp
  sudo ufw enable
  ```

- 若 MySQL 装在本机且需从外网管理，再单独放通 3306（**不建议对公网开放**，优先内网或 SSH 隧道）。

---

## 1. 你需要准备的内容

- 已解析到本机的域名（如 `noteapi.kymjs.com`），HTTPS 证书（Let’s Encrypt 或云厂商证书）。
- MySQL 5.7+ / 8.0（本机安装 **或** 云 RDS）。
- 微信开放平台 AppID/Secret、（可选）华为 AGC OAuth、Apple Sign In、微信支付商户与证书等——按业务填写到环境变量，**勿提交仓库**。若启用 **iOS App Store 内购**校验，还需在环境变量中配置 `APPLE_IAP_*` 与（正式环境）`APPLE_APP_STORE_APP_ID`，见 `server/.env.example`。
- 本机安装 **Go 1.24+**（用于在服务器上编译；也可在 CI 编译后只上传二进制）。

---

## 2. 首次部署（步骤摘要）

1. **安装依赖**（示例，按需裁剪）：

   ```bash
   sudo apt update
   sudo apt install -y git ca-certificates curl
   # 本机跑 MySQL 时：sudo apt install -y mysql-server
   # 反代：sudo apt install -y nginx
   # 编译本服务：需 Go 1.24+（Ubuntu 仓库版本若偏旧，请从 https://go.dev/dl 安装官方包）
   ```

2. **MySQL**：创建数据库与用户，授予该库 `ALL` 权限。
3. **执行迁移**（按顺序）：
   - `migrations/001_init.sql`
   - `migrations/002_user_identities.sql`
   - `migrations/003_user_profile.sql`（用户资料列；与其它迁移一样可通过 `deploy.sh migrate` 执行）
4. **配置环境变量**：复制 `server/.env.example` 为机器上的机密文件（例如 `/etc/noteapi.env`），填写 `MYSQL_DSN`、`JWT_SECRET`、各业务变量。
5. **编译**：在 `server` 目录执行 `go build -o /usr/local/bin/noteapi ./cmd/noteapi`（路径可自定）。
6. **systemd**：使用 `scripts/noteapi.service` 模板，把 `EnvironmentFile` 指向上面的 env 文件，`ExecStart` 指向二进制与监听地址。
7. **反向代理**：Nginx/Caddy 将 `https://noteapi.kymjs.com` 反代到 `127.0.0.1:9443`（或你在 `LISTEN_ADDR` 设的端口）。  
   - **头像上传**：反代默认 **`client_max_body_size` 常为 1m**，大于该值的请求会在到达 Go 之前被 **Nginx 以 413 HTML** 拒绝（与业务「最多 5MB」无关）。请在对应 `server` 或 `location` 中放宽，例如：
     ```nginx
     client_max_body_size 10m;
     ```
8. **防火墙**：在 Ubuntu 上通常用 **ufw**（见上文）；反代只监听 443 时，Go 可只绑 `127.0.0.1:9443`（`LISTEN_ADDR=127.0.0.1:9443`），不必对公网开放 9443。

---

## 3. 代码更新后如何处理

1. 拉取最新代码（或上传新压缩包解压）。
2. **若有新的 SQL 迁移**：在维护窗口先备份数据库（见下文「回滚」），再执行迁移，最后再起新版本二进制。
   - 一键：`sudo ./scripts/deploy.sh migrate`（按 `deploy.local.env` 连接数据库，顺序执行 `001`～`004`，含兑换码表）。
   - `001`、`002` 本身具备幂等或可重复特性；**`003_user_profile.sql` 通过检测列是否已存在跳过重复 `ALTER`**，已在运行且库里已有数据的实例也可安全执行（重复执行不会报错）。
   - 若你只上线过旧二进制、从未执行过 `003`，发布含「个人信息」的版本前**必须先跑完 `003`**，再重启服务，否则读写资料相关接口会缺列。
3. **若环境变量有新增项**：更新 `/etc/noteapi.env`（按 `server/.env.example` 逐项核对；例如用户头像依赖 **`AVATAR_WEBDAV_*`** 与 **`AVATAR_PUBLIC_BASE_URL`**（详见 `TECHNICAL.md` §2.7）；App Store 内购依赖 **`APPLE_IAP_*`**、**`APPLE_APP_STORE_APP_ID`**）。
4. **重新编译并重启服务**（或使用脚本）：
   ```bash
   cd /path/to/note/server
   sudo ./scripts/deploy.sh update
   ```
   等价手工步骤：`go build ...` 后 `sudo systemctl restart noteapi`。
5. **验证**：`curl -fsS https://你的域名/healthz` 应返回 `ok`；按需调用登录与个人资料相关接口做冒烟测试。

**推荐上线顺序（服务端已在跑、库里有数据）**：`mysqldump` 备份 → `sudo ./scripts/deploy.sh migrate` → `sudo ./scripts/deploy.sh update`。

**`update` 行为摘要**：覆盖二进制前默认将旧版备份为 `${DEPLOY_ROOT}/bin/noteapi.prev`（可用 `BACKUP_BIN_ON_UPDATE=0` 关闭）；重启后会检查 systemd 是否为 `active`，失败则打印最近日志并退出非零。可选在 `deploy.local.env` 设置 `NOTEAPI_HEALTH_URL=http://127.0.0.1:9443/healthz`（与 `LISTEN_ADDR` 一致），以便脚本用 `curl` 做一次 `/healthz` 冒烟。

---

## 4. 一键脚本与 `deploy.local.env`（机密）

**请勿把 MySQL 密码写进 `deploy.sh`**（脚本会进 Git）。在本机创建 **`server/scripts/deploy.local.env`**（已在 `server/.gitignore` 中忽略）：

```bash
cp scripts/deploy.local.env.example scripts/deploy.local.env
chmod 600 scripts/deploy.local.env
nano scripts/deploy.local.env   # 填写 MYSQL_HOST / MYSQL_USER / MYSQL_PASSWORD / MYSQL_DATABASE
```

`deploy.sh` 会自动加载同目录下的 `deploy.local.env`（若存在）。

**`deploy.sh` 子命令**：

- `first-time`：安装常用依赖、创建部署目录、编译、安装 systemd（需 root）。
- `migrate`：按 `deploy.local.env` 中的 **`MYSQL_*`** 顺序执行 `migrations/001`～`004`（需本机已安装 `mysql-client`）。
- `update`：`git pull`（可选）、编译、重启 `noteapi`（需 root）。

仍可通过环境变量覆盖 **`DEPLOY_ROOT`**、**`ENV_FILE`** 等。

**执行示例**：

```bash
cd /path/to/note/server
chmod +x scripts/deploy.sh

# 首次（阅读脚本内注释后）
sudo ./scripts/deploy.sh first-time

# 日常更新
sudo ./scripts/deploy.sh update

# 首次执行数据库迁移（填写好 deploy.local.env 后）
sudo ./scripts/deploy.sh migrate
```

---

## 5. `go build` 拉依赖超时（大陆网络）

若出现 `proxy.golang.org` / `i/o timeout`，脚本已默认设置：

- `GOPROXY=https://goproxy.cn,direct`
- `GOSUMDB=sum.golang.google.cn`

仍失败时可在执行前临时：

```bash
export GOPROXY=https://goproxy.io,direct
export GOSUMDB=off
sudo -E ./scripts/deploy.sh first-time
```

或在 `deploy.local.env` 里写上 `GOPROXY` / `GOSUMDB`（**`GOSUMDB=off` 会关闭校验，仅作权宜**）。

---

## 6. 回滚建议

- 部署前备份：`mysqldump` 业务库。
- 二进制：`deploy.sh update` 默认在覆盖前生成 **`${DEPLOY_ROOT}/bin/noteapi.prev`**；异常时可  
  `sudo systemctl stop noteapi && sudo cp -a "${DEPLOY_ROOT}/bin/noteapi.prev" "${DEPLOY_ROOT}/bin/noteapi" && sudo systemctl start noteapi`（路径按实际 `DEPLOY_ROOT` 调整）。
- 亦可手工：`cp noteapi noteapi.bak` 再覆盖；异常时还原后再 `systemctl start`。

---

## 7. 安全清单

- `JWT_SECRET` 使用足够长的随机串。
- `.env` / `/etc/noteapi.env` 权限 `600`，属主 root 或服务用户。
- 微信支付私钥、平台证书仅驻留在服务器文件系统，不进 Git。
