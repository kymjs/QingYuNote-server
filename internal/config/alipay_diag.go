package config

import (
	"log"
	"os"
	"strings"
)

// LogAlipayDiagnostic 输出 Alipay 接入条件拆解，便于排查「已配置仍报 alipay_not_configured」。
// 不打印私钥 PEM；仅打印私钥长度、路径及证书文件是否存在。
func (c *Config) LogAlipayDiagnostic(context string) {
	if c == nil {
		return
	}
	ctx := strings.TrimSpace(context)
	if ctx == "" {
		ctx = "unknown"
	}
	appID := strings.TrimSpace(c.AlipayAppID)
	priv := strings.TrimSpace(c.AlipayAppPrivateKey)
	path := strings.TrimSpace(c.AlipayAppPrivateKeyPath)

	pathOK := false
	pathErr := ""
	if path != "" {
		if fi, err := os.Stat(path); err != nil {
			pathErr = err.Error()
		} else {
			pathOK = true
			_ = fi.Size()
		}
	}

	log.Printf(
		"alipay_diag[%s] AlipayAppPayConfigured=%v AlipayCoreConfigured=%v production=%v public_base_url_len=%d",
		ctx, c.AlipayAppPayConfigured(), c.AlipayCoreConfigured(), c.AlipayProduction, len(strings.TrimSpace(c.PublicBaseURL)),
	)
	log.Printf(
		"alipay_diag[%s] app_id_len=%d app_id_suffix=%q priv_inline_len=%d priv_path=%q priv_path_readable=%v priv_path_stat_err=%q",
		ctx, len(appID), alipaySuffix(appID, 6), len(priv), path, pathOK, pathErr,
	)

	checkPath := func(name, p string) {
		p = strings.TrimSpace(p)
		ok := false
		errS := ""
		if p != "" {
			if _, err := os.Stat(p); err != nil {
				errS = err.Error()
			} else {
				ok = true
			}
		}
		log.Printf("alipay_diag[%s] cert_%s path_non_empty=%v path=%q exists=%v stat_err=%q",
			ctx, name, p != "", p, ok, errS)
	}
	checkPath("app_public", c.AlipayAppCertPublicPath)
	checkPath("platform_public", c.AlipayPlatformCertPublicPath)
	checkPath("root", c.AlipayRootCertPath)

	// 常见：进程未加载到 env（systemd EnvironmentFile / docker env 未挂载到该容器）
	log.Printf(
		"alipay_diag[%s] hint: 须为**运行 noteapi 的进程**设置 ALIPAY_APP_ID、ALIPAY_APP_PRIVATE_KEY 或 ALIPAY_APP_PRIVATE_KEY_PATH、ALIPAY_APP_CERT_PUBLIC_PATH、ALIPAY_PLATFORM_CERT_PUBLIC_PATH、ALIPAY_ROOT_CERT_PATH；键名须与文档一致，修改后需重启",
		ctx,
	)
}

func alipaySuffix(s string, n int) string {
	if n <= 0 || len(s) <= n {
		return s
	}
	return "…" + s[len(s)-n:]
}
