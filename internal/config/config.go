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

	// iOS Universal Link（微信等）：apple-app-site-association 中 appID 前缀，10 位 Team ID。
	AppleAppSiteAssociationTeamID string

	// App Store 内购（轻羽云）：校验客户端上报的 signedTransaction（JWS）。
	AppleIAPBundleID        string
	AppleIAPProductMonthly  string
	AppleIAPProductHalfYear string
	AppleIAPProductYearly   string
	AppleAppStoreAppID      int64 // App Store Connect「App 信息」中的 Apple ID（数字）；Production 校验链需要。

	// 华为应用内支付（Harmony 客户端 IAP）：OAuth 凭据见 AGC「API 管理 / OAuth 2.0 客户端」；商品 ID 与客户端常量一致。
	HuaweiIAPClientID        string
	HuaweiIAPClientSecret    string
	HuaweiIAPOrderSiteURL    string // 未传 account_flag 或 flag=0 时使用的订单域根 URL，中国大陆应用常用 DRCN
	HuaweiIAPPackageName     string // 须与 Harmony bundleName 一致，用于校验 purchaseTokenData.packageName
	HuaweiIAPProductMonthly  string
	HuaweiIAPProductHalfYear string
	HuaweiIAPProductYearly   string

	QingyuWebDAVBaseURL  string
	QingyuWebDAVUsername string
	QingyuWebDAVPassword string

	// 用户头像：上传到 NAS WebDAV；对外 CDN URL 写入 users.avatar_url。
	AvatarWebDAVBaseURL  string
	AvatarWebDAVUsername string
	AvatarWebDAVPassword string
	AvatarPublicBaseURL  string

	PublicBaseURL string

	// 支付宝 App 支付（轻羽云等）：证书模式；应用私钥仅能通过环境变量或密钥文件注入服务端，勿提交仓库。
	AlipayAppID                  string
	AlipayAppPrivateKey          string // PEM；与 AlipayAppPrivateKeyPath 二选一
	AlipayAppPrivateKeyPath      string
	AlipayAppCertPublicPath      string // 应用公钥证书 .crt（开放平台「接口加签方式」）
	AlipayPlatformCertPublicPath string // 支付宝公钥证书 alipayCertPublicKey_RSA2.crt
	AlipayRootCertPath           string // alipayRootCert.crt
	AlipayProduction             bool   // true=正式环境；false=沙箱（须与开放平台沙箱应用一致）
	// alipay.trade.page.pay 同步跳转 return_url（桌面浏览器支付完成后回跳；须 https 可访问页）。
	AlipayPagePayReturnURL string

	// 阿里云号码认证（短信验证码）：修改密码等场景 SendSmsVerifyCode / CheckSmsVerifyCode。
	AliyunAccessKeyID     string
	AliyunAccessKeySecret string
	AliyunSMSRegion       string
	AliyunSMSSignName     string
	AliyunSMSTemplateCode string
	AliyunSMSSchemeName   string
	// AliyunSMSTemplateParam JSON，默认 {"code":"##code##","min":"5"}；须与控制台模板变量一致。
	AliyunSMSTemplateParam string
}

func getenv(key, def string) string {
	v := strings.TrimSpace(os.Getenv(key))
	if v == "" {
		return def
	}
	return v
}

// parseAlipayProduction 未设置 ALIPAY_PRODUCTION 时默认正式环境；显式 false/0 为沙箱。
func parseAlipayProduction() bool {
	raw := strings.TrimSpace(os.Getenv("ALIPAY_PRODUCTION"))
	if raw == "" {
		return true
	}
	b, err := strconv.ParseBool(raw)
	if err != nil {
		return true
	}
	return b
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

		AppleAppSiteAssociationTeamID: getenv("APPLE_APP_SITE_ASSOCIATION_TEAM_ID", ""),

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

		AlipayAppID:                  getenv("ALIPAY_APP_ID", ""),
		AlipayAppPrivateKey:          getenv("ALIPAY_APP_PRIVATE_KEY", ""),
		AlipayAppPrivateKeyPath:      getenv("ALIPAY_APP_PRIVATE_KEY_PATH", ""),
		AlipayAppCertPublicPath:      getenv("ALIPAY_APP_CERT_PUBLIC_PATH", ""),
		AlipayPlatformCertPublicPath: getenv("ALIPAY_PLATFORM_CERT_PUBLIC_PATH", ""),
		AlipayRootCertPath:           getenv("ALIPAY_ROOT_CERT_PATH", ""),
		AlipayProduction:             parseAlipayProduction(),
		AlipayPagePayReturnURL:       strings.TrimSpace(getenv("ALIPAY_PAGE_PAY_RETURN_URL", "https://note.kymjs.com/private/harmony.html")),

		AliyunAccessKeyID:      getenv("ALIYUN_ACCESS_KEY_ID", ""),
		AliyunAccessKeySecret:  getenv("ALIYUN_ACCESS_KEY_SECRET", ""),
		AliyunSMSRegion:        getenv("ALIYUN_SMS_REGION", "cn-hangzhou"),
		AliyunSMSSignName:      getenv("ALIYUN_SMS_SIGN_NAME", ""),
		AliyunSMSTemplateCode:  getenv("ALIYUN_SMS_TEMPLATE_CODE", ""),
		AliyunSMSSchemeName:    getenv("ALIYUN_SMS_SCHEME_NAME", ""),
		AliyunSMSTemplateParam: getenv("ALIYUN_SMS_TEMPLATE_PARAM", `{"code":"##code##","min":"5"}`),
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

	c.HuaweiIAPClientID = getenv("HUAWEI_IAP_CLIENT_ID", "")
	c.HuaweiIAPClientSecret = getenv("HUAWEI_IAP_CLIENT_SECRET", "")
	c.HuaweiIAPOrderSiteURL = getenv("HUAWEI_IAP_ORDER_SITE_URL", "https://orders-drcn.iap.cloud.huawei.com.cn")
	c.HuaweiIAPPackageName = getenv("HUAWEI_IAP_PACKAGE_NAME", "com.kymjs.note")
	// 与 App Store 商品 ID 对齐，便于一套商品命名在双端配置。
	c.HuaweiIAPProductMonthly = getenv("HUAWEI_IAP_PRODUCT_MONTHLY", "com.kymjs.note.qingyu.monthly")
	c.HuaweiIAPProductHalfYear = getenv("HUAWEI_IAP_PRODUCT_HALF_YEAR", "com.kymjs.note.qingyu.half_year")
	c.HuaweiIAPProductYearly = getenv("HUAWEI_IAP_PRODUCT_YEARLY", "com.kymjs.note.qingyu.yearly")

	return c
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

// AppleIAPVerifyConfigured 为 true 时允许校验苹果内购 JWS（Bundle ID 与各档位商品 ID 已配置）。
func (c *Config) AppleIAPVerifyConfigured() bool {
	return strings.TrimSpace(c.AppleIAPBundleID) != "" &&
		strings.TrimSpace(c.AppleIAPProductMonthly) != "" &&
		strings.TrimSpace(c.AppleIAPProductHalfYear) != "" &&
		strings.TrimSpace(c.AppleIAPProductYearly) != ""
}

// HuaweiIAPVerifyConfigured 为 true 时允许 Harmony 内购 purchaseToken 核销。
func (c *Config) HuaweiIAPVerifyConfigured() bool {
	return strings.TrimSpace(c.HuaweiIAPClientID) != "" &&
		strings.TrimSpace(c.HuaweiIAPClientSecret) != "" &&
		strings.TrimSpace(c.HuaweiIAPProductMonthly) != "" &&
		strings.TrimSpace(c.HuaweiIAPProductHalfYear) != "" &&
		strings.TrimSpace(c.HuaweiIAPProductYearly) != ""
}

// PlanFromHuaweiProductID 将华为 IAP 商品 ID 映射为订单 plan_id。
func (c *Config) PlanFromHuaweiProductID(productID string) string {
	p := strings.TrimSpace(productID)
	if p == "" {
		return ""
	}
	switch p {
	case strings.TrimSpace(c.HuaweiIAPProductMonthly):
		return "monthly"
	case strings.TrimSpace(c.HuaweiIAPProductHalfYear):
		return "half_year"
	case strings.TrimSpace(c.HuaweiIAPProductYearly):
		return "yearly"
	default:
		return ""
	}
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

// AlipayCoreConfigured 为 true 时表示已配置证书模式所需全部文件/密钥（不校验文件是否可读）。
func (c *Config) AlipayCoreConfigured() bool {
	if strings.TrimSpace(c.AlipayAppID) == "" {
		return false
	}
	hasPK := strings.TrimSpace(c.AlipayAppPrivateKey) != "" || strings.TrimSpace(c.AlipayAppPrivateKeyPath) != ""
	if !hasPK {
		return false
	}
	return strings.TrimSpace(c.AlipayAppCertPublicPath) != "" &&
		strings.TrimSpace(c.AlipayPlatformCertPublicPath) != "" &&
		strings.TrimSpace(c.AlipayRootCertPath) != ""
}

// AlipayAppPayConfigured 为 true 时可签发 orderStr（依赖 PUBLIC_BASE_URL 拼异步通知地址）。
func (c *Config) AlipayAppPayConfigured() bool {
	return c.AlipayCoreConfigured() && strings.TrimSpace(c.PublicBaseURL) != ""
}

// AliyunSMSConfigured 为 true 时允许发送/核验短信验证码（修改密码短信流程）。
func (c *Config) AliyunSMSConfigured() bool {
	return strings.TrimSpace(c.AliyunAccessKeyID) != "" &&
		strings.TrimSpace(c.AliyunAccessKeySecret) != "" &&
		strings.TrimSpace(c.AliyunSMSSignName) != "" &&
		strings.TrimSpace(c.AliyunSMSTemplateCode) != ""
}

func ParseOrderIDParam(v string) int64 {
	id, err := strconv.ParseInt(strings.TrimSpace(v), 10, 64)
	if err != nil {
		return 0
	}
	return id
}
