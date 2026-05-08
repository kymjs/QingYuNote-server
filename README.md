# 轻羽云笔记 Note API（Go）

提供微信 OAuth 换 JWT、订阅状态、轻羽云 WebDAV 凭据下发、微信支付下单与回调。

**文档**：完整接口与数据库说明见 [`TECHNICAL.md`](TECHNICAL.md)；空白服务器部署与更新流程见 [`DEPLOYMENT.md`](DEPLOYMENT.md)（含 `scripts/deploy.sh`）。

## 运行

```bash
export MYSQL_DSN='user:pass@tcp(127.0.0.1:3306)/noteapi?parseTime=true&loc=Local'
export JWT_SECRET='随机长字符串'
export WECHAT_APP_ID='wx…'
export WECHAT_APP_SECRET='…'
# 轻羽 NAS WebDAV：须在部署环境注入（勿提交真实口令到仓库）
export QINGYU_WEBDAV_BASE_URL='https://nas.example.com:5006/QingYuYun'
export QINGYU_WEBDAV_USERNAME='qingyu'
export QINGYU_WEBDAV_PASSWORD='…'
export PUBLIC_BASE_URL='https://noteapi.kymjs.com'
```

导入表结构（新环境执行 `001` 后执行 `002`；已有库追加执行 `002` 即可）：

```bash
mysql "$MYSQL_DSN" < migrations/001_init.sql
mysql "$MYSQL_DSN" < migrations/002_user_identities.sql
```

多登录方式（微信 / 华为 / Apple）共用一个 `users` 行，绑定关系在表 `user_identities`；首次升级会按历史 `wechat_openid` 回填微信身份。

部署时若启用华为登录，需配置（与 AGC 中应用一致）：

```bash
export HUAWEI_CLIENT_ID='…'
export HUAWEI_CLIENT_SECRET='…'
export HUAWEI_REDIRECT_URI=''   # 与客户端、AGC 回调一致；可留空视华为控制台要求
```

若启用「通过 Apple 登录」：

```bash
export APPLE_CLIENT_ID='com.example.app'   # 与 identity_token 的 aud 一致
```

启动：

```bash
go run ./cmd/noteapi
```

默认监听 `:9443`，可通过 `LISTEN_ADDR` 修改。

## 微信支付 APIv3（可选，商户就绪后配置）

```bash
export WECHAT_PAY_MCH_ID='…'
export WECHAT_PAY_CERT_SERIAL='…'
export WECHAT_PAY_PRIVATE_KEY_PATH='/path/to/apiclient_key.pem'
export WECHAT_PAY_API_V3_KEY='32位APIv3密钥'
```

异步通知验签需微信平台证书 PEM（可从商户平台下载或 `/v3/certificates` 获取），路径：

```bash
export WECHAT_PAY_PLATFORM_CERT_PEM_PATH='/path/to/wechatpay_platform.pem'
```

回调 URL 为：`${PUBLIC_BASE_URL}/api/v1/webhooks/wechat/pay`（须与商户平台配置一致）。

未配置商户参数时，`POST /api/v1/orders/{id}/wechat/prepay` 返回 `503`，客户端提示「接入中」。

## HTTP 路由（摘要）

| 方法 | 路径 | 说明 |
|------|------|------|
| POST | `/api/v1/auth/wechat` | `{ "code" }` → JWT；同一微信主体落在同一 `users.id` |
| POST | `/api/v1/auth/huawei` | `{ "authorization_code", "redirect_uri?" }` → JWT（需 `HUAWEI_*`） |
| POST | `/api/v1/auth/apple` | `{ "identity_token" }` → JWT（需 `APPLE_CLIENT_ID`） |
| POST | `/api/v1/me/link/wechat` | Bearer `{ "code" }`，绑定微信到当前账号 |
| POST | `/api/v1/me/link/huawei` | Bearer `{ "authorization_code", "redirect_uri?" }` |
| POST | `/api/v1/me/link/apple` | Bearer `{ "identity_token" }`；冲突时 `409` `identity_already_linked` |
| POST | `/api/v1/me/merge/wechat` | Bearer `{ "code" }`：身份未占用则等同绑定；已绑定到其他用户则**并入当前账号**（订阅取终身或较晚到期、订单与 identities 迁移，删除被吸收的用户行） |
| POST | `/api/v1/me/merge/huawei` | Bearer `{ "authorization_code", "redirect_uri?" }`，同上 |
| POST | `/api/v1/me/merge/apple` | Bearer `{ "identity_token" }`，同上 |
| GET | `/api/v1/me/subscription` | Bearer |

合并成功后响应示例：`{"ok":true,"action":"merged","absorbed_user_id":123}`；若仅需绑定无冲突账号则为 `"action":"linked"`；已绑定到当前用户则为 `"noop"`。**注意**：合并仅作用于服务端数据库中的订阅与订单；被吸收账号若在 NAS 上有独立目录（如 `/旧users.id/`），笔记文件不会自动搬迁，需另行规划迁移。
| GET | `/api/v1/qingyu/webdav` | Bearer；订阅有效且已配置 `QINGYU_WEBDAV_*` 时返回 NAS；`notes_dir` 为当前 JWT 用户对应 `users.id`，形如 `/{id}/`，每人不同；单用户约 60 次/分钟限流 + 45s 响应缓存 |
| POST | `/api/v1/orders` | Bearer body `{ "plan_id": "monthly\|half_year\|yearly" }` |
| POST | `/api/v1/orders/{id}/wechat/prepay` | Bearer → APP 调起参数 |
| GET | `/api/v1/orders/{id}` | Bearer |
| POST | `/api/v1/webhooks/wechat/pay` | 微信服务器回调 |
| GET | `/healthz` | 健康检查 |
