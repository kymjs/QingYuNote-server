package config

import (
	"log"
	"os"
	"strconv"
	"strings"
)

// Config 来自环境变量（部署时用 systemd/docker envfile 注入）。
type Config struct {
	ListenAddr string

	MySQLDSN string

	JWTSecret string

	WechatAppID     string
	WechatAppSecret string

	// 华为帐号 OAuth（authorization_code 换 token）；redirect_uri 须与 AGC 配置一致。
	HuaweiClientID     string
	HuaweiClientSecret string
	HuaweiRedirectURI  string

	// Apple Sign In：校验 identity_token 的 aud（App ID / Services ID）。
	AppleClientID string

	// App Store 内购（轻羽云）：校验客户端上报的 signedTransaction（JWS）。
	AppleIAPBundleID          string
	AppleIAPProductMonthly    string
	AppleIAPProductHalfYear   string
	AppleIAPProductYearly     string
	AppleAppStoreAppID        int64 // App Store Connect「App 信息」中的 Apple ID（数字）；Production 校验链需要。

	QingyuWebDAVBaseURL  string
	QingyuWebDAVUsername string
	QingyuWebDAVPassword string

	// 用户头像：上传到 NAS WebDAV；对外 CDN URL 写入 users.avatar_url。
	AvatarWebDAVBaseURL  string
	AvatarWebDAVUsername string
	AvatarWebDAVPassword string
	AvatarPublicBaseURL  string

	PublicBaseURL string

	WechatPayMchID          string
	WechatPayCertSerial     string
	WechatPayPrivateKeyPath string
	WechatPayAPIv3Key       string

	WechatPayNotifyPath string
}

func getenv(key, def string) string {
	v := strings.TrimSpace(os.Getenv(key))
	if v == "" {
		return def
	}
	return v
}

func Load() *Config {
	c := &Config{
		ListenAddr: getenv("LISTEN_ADDR", ":9443"),

		MySQLDSN: getenv("MYSQL_DSN", ""),

		JWTSecret: getenv("JWT_SECRET", ""),

		WechatAppID:     getenv("WECHAT_APP_ID", ""),
		WechatAppSecret: getenv("WECHAT_APP_SECRET", ""),

		HuaweiClientID:     getenv("HUAWEI_CLIENT_ID", ""),
		HuaweiClientSecret: getenv("HUAWEI_CLIENT_SECRET", ""),
		HuaweiRedirectURI:  getenv("HUAWEI_REDIRECT_URI", ""),

		AppleClientID: getenv("APPLE_CLIENT_ID", ""),

		AppleIAPBundleID: getenv("APPLE_IAP_BUNDLE_ID", "com.kymjs.note"),
		// 须与 App Store Connect 中 IAP 商品 ID 一致（Flutter 构建时可覆盖默认值）。
		AppleIAPProductMonthly:  getenv("APPLE_IAP_PRODUCT_MONTHLY", "com.kymjs.note.qingyu.monthly"),
		AppleIAPProductHalfYear: getenv("APPLE_IAP_PRODUCT_HALF_YEAR", "com.kymjs.note.qingyu.half_year"),
		AppleIAPProductYearly:   getenv("APPLE_IAP_PRODUCT_YEARLY", "com.kymjs.note.qingyu.yearly"),

		// 轻羽 WebDAV 仅通过部署环境注入，仓库内不设默认值，避免公开仓库泄露凭据。
		QingyuWebDAVBaseURL:  getenv("QINGYU_WEBDAV_BASE_URL", ""),
		QingyuWebDAVUsername: getenv("QINGYU_WEBDAV_USERNAME", ""),
		QingyuWebDAVPassword: getenv("QINGYU_WEBDAV_PASSWORD", ""),

		AvatarWebDAVBaseURL:  getenv("AVATAR_WEBDAV_BASE_URL", "https://nas.therouter.cn:5006/cdn/note-avatar"),
		AvatarWebDAVUsername: getenv("AVATAR_WEBDAV_USERNAME", ""),
		AvatarWebDAVPassword: getenv("AVATAR_WEBDAV_PASSWORD", ""),
		AvatarPublicBaseURL:  strings.TrimRight(getenv("AVATAR_PUBLIC_BASE_URL", "http://cdn.kymjs.com:8843/note-avatar"), "/"),

		PublicBaseURL: strings.TrimRight(getenv("PUBLIC_BASE_URL", "https://noteapi.kymjs.com"), "/"),

		WechatPayMchID:          getenv("WECHAT_PAY_MCH_ID", ""),
		WechatPayCertSerial:     getenv("WECHAT_PAY_CERT_SERIAL", ""),
		WechatPayPrivateKeyPath: getenv("WECHAT_PAY_PRIVATE_KEY_PATH", ""),
		WechatPayAPIv3Key:       getenv("WECHAT_PAY_API_V3_KEY", ""),

		WechatPayNotifyPath: getenv("WECHAT_PAY_NOTIFY_PATH", "/api/v1/webhooks/wechat/pay"),
	}

	if c.JWTSecret == "" {
		log.Printf("warning: JWT_SECRET empty — auth tokens will be insecure; set in production")
	}
	if !c.QingyuWebDAVConfigured() {
		log.Printf("warning: QINGYU_WEBDAV_BASE_URL / USERNAME / PASSWORD unset — light-cloud WebDAV endpoint returns 503 until set")
	}
	if !c.AvatarWebDAVConfigured() {
		log.Printf("warning: AVATAR_WEBDAV_USERNAME / PASSWORD unset — avatar upload returns 503 until set")
	}
	// App Store 数字 App ID：用于 JWS 在 Production 环境的校验；沙盒可留 0。
	if raw := strings.TrimSpace(getenv("APPLE_APP_STORE_APP_ID", "")); raw != "" {
		if id, err := strconv.ParseInt(raw, 10, 64); err == nil {
			c.AppleAppStoreAppID = id
		} else {
			log.Printf("warning: APPLE_APP_STORE_APP_ID invalid: %v", err)
		}
	}
	return c
}

func (c *Config) WechatPayConfigured() bool {
	return c.WechatPayMchID != "" &&
		c.WechatPayCertSerial != "" &&
		c.WechatPayPrivateKeyPath != "" &&
		c.WechatPayAPIv3Key != ""
}

// QingyuWebDAVConfigured 为 true 时方可向下发 NAS 端点与账号（password 仅运行时存在于进程中）。
func (c *Config) QingyuWebDAVConfigured() bool {
	return strings.TrimSpace(c.QingyuWebDAVBaseURL) != "" &&
		strings.TrimSpace(c.QingyuWebDAVUsername) != "" &&
		strings.TrimSpace(c.QingyuWebDAVPassword) != ""
}

// AvatarWebDAVConfigured 为 true 时允许 POST /api/v1/me/avatar。
func (c *Config) AvatarWebDAVConfigured() bool {
	return strings.TrimSpace(c.AvatarWebDAVBaseURL) != "" &&
		strings.TrimSpace(c.AvatarWebDAVUsername) != "" &&
		strings.TrimSpace(c.AvatarWebDAVPassword) != ""
}

func (c *Config) HuaweiOAuthConfigured() bool {
	return strings.TrimSpace(c.HuaweiClientID) != "" &&
		strings.TrimSpace(c.HuaweiClientSecret) != ""
}

func (c *Config) AppleSignInConfigured() bool {
	return len(c.AppleClientIDs()) > 0
}

// AppleClientIDs 返回允许的 identity_token aud 列表（逗号分隔）。
// iOS 原生登录 aud 为 Xcode Bundle ID；网页 OAuth 常见为 Services ID。
func (c *Config) AppleClientIDs() []string {
	raw := strings.TrimSpace(c.AppleClientID)
	if raw == "" {
		return nil
	}
	var out []string
	for _, p := range strings.Split(raw, ",") {
		p = strings.TrimSpace(p)
		if p != "" {
			out = append(out, p)
		}
	}
	return out
}

func (c *Config) NotifyURL() string {
	return c.PublicBaseURL + c.WechatPayNotifyPath
}

// AppleIAPVerifyConfigured 为 true 时允许校验苹果内购 JWS（Bundle ID 与各档位商品 ID 已配置）。
func (c *Config) AppleIAPVerifyConfigured() bool {
	return strings.TrimSpace(c.AppleIAPBundleID) != "" &&
		strings.TrimSpace(c.AppleIAPProductMonthly) != "" &&
		strings.TrimSpace(c.AppleIAPProductHalfYear) != "" &&
		strings.TrimSpace(c.AppleIAPProductYearly) != ""
}

// PlanFromAppleProductID 将 App Store 商品 ID 映射为订单 plan_id；无法识别时返回空串。
func (c *Config) PlanFromAppleProductID(productID string) string {
	p := strings.TrimSpace(productID)
	if p == "" {
		return ""
	}
	switch p {
	case strings.TrimSpace(c.AppleIAPProductMonthly):
		return "monthly"
	case strings.TrimSpace(c.AppleIAPProductHalfYear):
		return "half_year"
	case strings.TrimSpace(c.AppleIAPProductYearly):
		return "yearly"
	default:
		return ""
	}
}

func ParsePlanMonths(plan string) int {
	switch plan {
	case "monthly":
		return 1
	case "half_year":
		return 7
	case "yearly":
		return 12
	default:
		return 0
	}
}

func PlanAmountFen(plan string) int {
	switch plan {
	case "monthly":
		return 1000
	case "half_year":
		return 6000
	case "yearly":
		return 10000
	default:
		return 0
	}
}

func ParseOrderIDParam(v string) int64 {
	id, err := strconv.ParseInt(strings.TrimSpace(v), 10, 64)
	if err != nil {
		return 0
	}
	return id
}
