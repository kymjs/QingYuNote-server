# 轻羽云 Note API — 服务端技术说明

本文描述 `server/` 下 Go 服务的**对外 HTTP 接口**、**功能模块**、**数据库结构**及 **`store` 数据访问层**，与仓库内实现保持一致（若代码变更请以源码为准）。

---

## 1. 架构概览

| 层级 | 路径 | 职责 |
|------|------|------|
| 入口 | `cmd/noteapi/main.go` | 加载配置、连接 MySQL、构造 `api.Server`、监听 `LISTEN_ADDR` |
| HTTP | `internal/api/server.go` | 路由、鉴权中间件 `auth`、各 Handler |
| 鉴权 | `internal/auth/jwt.go` | HS256 JWT：`sub` 为 `users.id`，默认 TTL 7 天 |
| 配置 | `internal/config/config.go` | 环境变量 → `Config`，套餐金额与周期 |
| 持久化 | `internal/store/*.go` | MySQL：`Store` 封装 CRUD / 事务 |
| 订阅逻辑 | `internal/subscription/extend.go` | 支付成功后顺延订阅；对外状态枚举 |
| 微信 OAuth | `internal/wechat/oauth.go` | `code` → `openid` |
| 华为 OAuth | `internal/huawei/oauth.go` | `authorization_code` → `open_id` / `id_token.sub` |
| Apple | `internal/appleid/verify.go` | `identity_token` ES256 + JWKS |
| 微信支付 | `internal/wxpay/appsign.go` 等 | APP 调起签名；回调验签见 `internal/wxnotify` |
| 轻羽限流 | `internal/api/qingyu_guard.go` | `GET /qingyu/webdav` 每分钟限流 + 45s 缓存 |

进程内**无** Redis；会话仅靠 JWT。

---

## 2. 对客户端 HTTP 接口一览

**通用约定**

- `Content-Type: application/json`（除微信支付回调由 SDK 解析）。
- 需要登录的接口：`Authorization: Bearer <access_token>`。
- JWT 由 `/api/v1/auth/*` 返回的 `access_token` 字段承载。

### 2.1 健康检查

| 方法 | 路径 | 鉴权 | 说明 |
|------|------|------|------|
| GET | `/healthz` | 否 | 返回正文 `ok`，HTTP 200 |

### 2.2 登录（换取 JWT）

下列接口**无需** Bearer；成功后返回统一字段：

`access_token`、`expires_in`（秒）、`user_id`（`users.id`，JSON 数字）。

| 方法 | 路径 | 请求体 | 说明 |
|------|------|--------|------|
| POST | `/api/v1/auth/wechat` | `{ "code": "<微信 OAuth code>" }` | 换 openid， upsert 用户与身份 |
| POST | `/api/v1/auth/huawei` | `{ "authorization_code", "redirect_uri?" }` | 需服务端配置 `HUAWEI_*` |
| POST | `/api/v1/auth/apple` | `{ "identity_token": "<Apple JWT>" }` | 需 `APPLE_CLIENT_ID` |

**常见错误 JSON**：`{"error":"invalid_body"}` 400；OAuth 失败带 `wechat_oauth_failed` / `huawei_oauth_failed` / `apple_token_invalid`；华为/Apple 未配置时 503 及对应 `*_not_configured`。

### 2.3 绑定第三方身份（需登录）

将当前 JWT 用户与新的 provider 绑定；若该身份已被**其他**用户占用 → **409** `identity_already_linked`。

| 方法 | 路径 | 请求体 |
|------|------|--------|
| POST | `/api/v1/me/link/wechat` | `{ "code" }` |
| POST | `/api/v1/me/link/huawei` | `{ "authorization_code", "redirect_uri?" }` |
| POST | `/api/v1/me/link/apple` | `{ "identity_token" }` |

成功：`{"ok":"true"}`（字符串值的 JSON）。

### 2.4 账号合并（需登录）

语义：**保留当前 JWT 用户**，用凭据证明的另一身份若属于「另一个用户」，则把对方账号**并入**当前用户（订阅合并、订单迁移、`user_identities` 迁移，删除被吸收用户行）。

| 方法 | 路径 | 请求体 |
|------|------|--------|
| POST | `/api/v1/me/merge/wechat` | `{ "code" }` |
| POST | `/api/v1/me/merge/huawei` | `{ "authorization_code", "redirect_uri?" }` |
| POST | `/api/v1/me/merge/apple` | `{ "identity_token" }` |

成功示例：

- `{"ok":true,"action":"noop"}` — 已绑定到当前账号  
- `{"ok":true,"action":"linked"}` — 身份原先不存在，仅完成绑定  
- `{"ok":true,"action":"merged","absorbed_user_id":123}` — 已合并另一用户  

失败：`merge_failed` 500 等。

### 2.5 订阅

| 方法 | 路径 | 鉴权 |
|------|------|------|
| GET | `/api/v1/me/subscription` | Bearer |

**200 响应**（JSON）：

- `state`：`none` | `active` | `expired` | `lifetime`  
- `expires_at`：到期日 `YYYY-MM-DD`（`lifetime` 时可能为空）  
- `is_lifetime`：bool  

### 2.6 轻羽云 WebDAV 下发

| 方法 | 路径 | 鉴权 |
|------|------|------|
| GET | `/api/v1/qingyu/webdav` | Bearer |

**条件**：订阅状态为 `active` 或 `lifetime`；且环境变量配置完整 `QINGYU_WEBDAV_*`。

**200 响应**：`base_url`、`username`、`password`、`notes_dir`（形如 `/{users.id}/`）。

**常见错误**：403 `subscription_required`；503 `qingyu_webdav_not_configured`；429 限流（带 `Retry-After`）。

### 2.7 订单与微信支付

| 方法 | 路径 | 鉴权 | 说明 |
|------|------|------|------|
| POST | `/api/v1/orders` | Bearer | body：`{ "plan_id": "monthly" \| "half_year" \| "yearly" }` |
| POST | `/api/v1/orders/{id}/wechat/prepay` | Bearer | APP 调起参数；商户未配置 → 503 `wechat_pay_not_configured` |
| GET | `/api/v1/orders/{id}` | Bearer | 查询订单 |

**套餐与金额（分）**（`internal/config/config.go`）：

- `monthly` → 1 个月，1000 分  
- `half_year` → **7 个月**（代码 `ParsePlanMonths`），6000 分  
- `yearly` → 12 个月，10000 分  

创建订单成功：`id`、`out_trade_no`、`plan_id`、`amount_total`、`status`（`pending`）。

Prepay 200：`app_id`、`partner_id`、`prepay_id`、`package`、`nonce_str`、`timestamp`、`sign`、`sign_type`。

### 2.8 微信支付异步通知（服务端对微信）

| 方法 | 路径 | 鉴权 |
|------|------|------|
| POST | `/api/v1/webhooks/wechat/pay` | 微信签名（需配置平台证书与 APIv3 Key） |

成功处理返回微信约定 JSON（含 `code":"SUCCESS"`）。未配置验签处理器时返回 503 `notify_not_configured`。

---

## 3. 数据库表与字段

脚本位置：`migrations/001_init.sql`、`migrations/002_user_identities.sql`、`migrations/003_user_profile.sql`（资料列）。

### 3.1 `users`

| 字段 | 类型 | 说明 |
|------|------|------|
| id | BIGINT PK AI | 用户主键，JWT `sub` |
| folder_key | VARCHAR(128) | 历史/兼容目录键；新建用户多为 `u{id}` |
| wechat_openid | VARCHAR(64) NULL UNIQUE | 兼容列；多身份以 `user_identities` 为准 |
| display_name | VARCHAR(191) NULL | 用户资料（`003`） |
| avatar_url | VARCHAR(512) NULL | 头像 URL |
| phone | VARCHAR(32) NULL | 手机号 |
| email | VARCHAR(191) NULL | 邮箱 |
| password_hash | VARCHAR(255) NULL | bcrypt，仅存哈希 |
| created_at / updated_at | DATETIME(3) | — |

### 3.2 `user_identities`

| 字段 | 类型 | 说明 |
|------|------|------|
| id | BIGINT PK AI | — |
| user_id | BIGINT FK → users.id | ON DELETE CASCADE |
| provider | VARCHAR(16) | `wechat` / `huawei` / `apple` |
| subject | VARCHAR(191) | 第三方主体（openid、华为 sub、Apple sub） |
| created_at | DATETIME(3) | — |
| UNIQUE(provider, subject) | | 全局唯一 |

### 3.3 `subscriptions`

| 字段 | 类型 | 说明 |
|------|------|------|
| user_id | BIGINT PK FK | — |
| expires_at | DATE NULL | 到期日；终身可为 NULL |
| is_lifetime | TINYINT(1) | — |
| updated_at | DATETIME(3) | — |

### 3.4 `orders`

| 字段 | 类型 | 说明 |
|------|------|------|
| id | BIGINT PK AI | — |
| user_id | BIGINT FK | — |
| out_trade_no | VARCHAR(64) UNIQUE | 商户订单号 |
| plan_id | VARCHAR(32) | 套餐 id |
| amount_total | INT | 金额（分） |
| status | VARCHAR(24) | 如 `pending`、`paid` |
| created_at / paid_at | DATETIME(3) | — |
| transaction_id | VARCHAR(128) NULL | 微信支付单号 |

---

## 4. Store 层方法（操作数据库）

定义于 `internal/store`，对外通过 `*store.Store` 注入 `api.Server`。

| 方法 | 文件 | 作用 |
|------|------|------|
| `OpenMySQL(dsn)` | mysql.go | 建立连接池 |
| `UpsertUserByWechat` | mysql.go | 委托 `EnsureUserForIdentity(wechat)` |
| `GetUserByWechatOpenID` | mysql.go | 按 openid 查用户 |
| `GetUserByID` | mysql.go | 按 id 查用户 |
| `GetSubscription` | mysql.go | 读订阅行 |
| `UpsertSubscriptionExpiry` | mysql.go | 插入或更新订阅 |
| `CreateOrder` | mysql.go | 创建订单 |
| `GetOrderByID` / `GetOrderByOutTradeNo` | mysql.go | 查单 |
| `MarkOrderPaid` | mysql.go | 标记已支付 |
| `LookupUserIDByIdentity` | identities.go | provider+subject → user_id |
| `EnsureUserForIdentity` | identities.go | 无则创建用户+身份 |
| `LinkIdentity` | identities.go | 绑定身份；冲突返回 `ErrIdentityLinkedOtherUser` |
| `MergeUserAbsorb` | merge.go | 合并两用户（订阅/订单/身份/删源用户） |
| `QingyuNotesDirForAuthenticatedUser` | mysql.go | 生成 NAS 目录 `/{id}/` |

---

## 5. 环境变量（与 `internal/config` 对应）

| 变量 | 用途 |
|------|------|
| `LISTEN_ADDR` | 监听地址，默认 `:9443` |
| `MYSQL_DSN` | MySQL DSN，必填 |
| `JWT_SECRET` | JWT 密钥 |
| `WECHAT_APP_ID` / `WECHAT_APP_SECRET` | 微信开放平台应用 |
| `HUAWEI_CLIENT_ID` / `HUAWEI_CLIENT_SECRET` / `HUAWEI_REDIRECT_URI` | 华为 OAuth |
| `APPLE_CLIENT_ID` | Sign in with Apple 的 `aud` |
| `QINGYU_WEBDAV_BASE_URL` / `USERNAME` / `PASSWORD` | 轻羽 NAS 下发 |
| `PUBLIC_BASE_URL` | 回调域名前缀 |
| `WECHAT_PAY_*` | 商户号、序列号、私钥路径、APIv3 Key |
| `WECHAT_PAY_PLATFORM_CERT_PEM_PATH` | 微信平台证书 PEM（回调验签） |
| `WECHAT_PAY_NOTIFY_PATH` | 可选，默认 `/api/v1/webhooks/wechat/pay` |

---

## 6. 相关文档

- 部署与脚本：`DEPLOYMENT.md`、`scripts/deploy.sh`
