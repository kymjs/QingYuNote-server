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
- 微信开放平台 AppID/Secret、（可选）华为 AGC OAuth、Apple Sign In、微信支付商户与证书等——按业务填写到环境变量，**勿提交仓库**。
- 本机安装 **Go 1.22+**（用于在服务器上编译；也可在 CI 编译后只上传二进制）。

---

## 2. 首次部署（步骤摘要）

1. **安装依赖**（示例，按需裁剪）：

   ```bash
   sudo apt update
   sudo apt install -y git ca-certificates curl
   # 本机跑 MySQL 时：sudo apt install -y mysql-server
   # 反代：sudo apt install -y nginx
   # 编译本服务：需 Go 1.22+（Ubuntu 仓库版本若偏旧，请从 https://go.dev/dl 安装官方包）
   ```

2. **MySQL**：创建数据库与用户，授予该库 `ALL` 权限。
3. **执行迁移**（按顺序）：
   - `migrations/001_init.sql`
   - `migrations/002_user_identities.sql`
4. **配置环境变量**：复制 `server/.env.example` 为机器上的机密文件（例如 `/etc/noteapi.env`），填写 `MYSQL_DSN`、`JWT_SECRET`、各业务变量。
5. **编译**：在 `server` 目录执行 `go build -o /usr/local/bin/noteapi ./cmd/noteapi`（路径可自定）。
6. **systemd**：使用 `scripts/noteapi.service` 模板，把 `EnvironmentFile` 指向上面的 env 文件，`ExecStart` 指向二进制与监听地址。
7. **反向代理**：Nginx/Caddy 将 `https://noteapi.kymjs.com` 反代到 `127.0.0.1:8080`（或你在 `LISTEN_ADDR` 设的端口）。
8. **防火墙**：在 Ubuntu 上通常用 **ufw**（见上文）；反代只监听 443 时，Go 可只绑 `127.0.0.1:8080`（`LISTEN_ADDR=127.0.0.1:8080`），不必对公网开放 8080。

---

## 3. 代码更新后如何处理

1. 拉取最新代码（或上传新压缩包解压）。
2. **若有新的 SQL 迁移**：在维护窗口执行新迁移文件（当前仓库仅 `001`、`002`；以后若有 `003_xxx.sql` 需按发布说明执行）。
3. **若环境变量有新增项**：更新 `/etc/noteapi.env`。
4. **重新编译并重启服务**：
   ```bash
   cd /path/to/note/server
   go build -o /usr/local/bin/noteapi ./cmd/noteapi
   sudo systemctl restart noteapi
   ```
5. **验证**：`curl -fsS https://你的域名/healthz` 应返回 `ok`。

---

## 4. 一键脚本

仓库提供 **`scripts/deploy.sh`**，支持：

- `first-time`：安装常用依赖、创建部署目录、编译、提示迁移与 systemd（需 root 部分自选）。
- `update`：在同一目录拉代码（可选）、`go build`、重启 `noteapi` 服务。

使用前请编辑脚本顶部的 **`DEPLOY_ROOT`**、**`ENV_FILE`** 等变量，或导出同名环境变量覆盖。

**执行示例**：

```bash
cd /path/to/note/server
chmod +x scripts/deploy.sh

# 首次（阅读脚本内注释后）
sudo ./scripts/deploy.sh first-time

# 日常更新
sudo ./scripts/deploy.sh update
```

---

## 5. 回滚建议

- 部署前备份：`mysqldump` 业务库。
- 二进制保留上一份：`cp noteapi noteapi.bak` 再覆盖；异常时 `systemctl stop noteapi && cp noteapi.bak noteapi && systemctl start noteapi`。

---

## 6. 安全清单

- `JWT_SECRET` 使用足够长的随机串。
- `.env` / `/etc/noteapi.env` 权限 `600`，属主 root 或服务用户。
- 微信支付私钥、平台证书仅驻留在服务器文件系统，不进 Git。
