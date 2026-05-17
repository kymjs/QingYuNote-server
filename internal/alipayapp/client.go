// Package alipayapp 从环境变量配置构建支付宝「公钥证书」模式客户端（App 支付 / 异步通知验签）。
package alipayapp

import (
	"errors"
	"fmt"
	"os"
	"strings"

	"github.com/kymjs/noteapi/internal/config"
	alipay "github.com/smartwalle/alipay/v3"
)

func normalizeAppPrivateKeyPEM(s string) string {
	s = strings.TrimSpace(s)
	s = strings.TrimPrefix(s, "\ufeff") // 部分编辑器在 PEM 文件头写入 BOM，会导致 asn1 解析失败
	return s
}

func validateAppPrivateKeyPEMHeader(pem string) error {
	p := strings.TrimSpace(pem)
	if p == "" {
		return errors.New("应用私钥为空")
	}
	if strings.Contains(p, "BEGIN CERTIFICATE") {
		return errors.New("当前内容像「证书」而非「私钥」：请使用应用私钥 PEM（RSA PRIVATE KEY / PRIVATE KEY），勿把 .crt 填进 ALIPAY_APP_PRIVATE_KEY*")
	}
	if !strings.Contains(p, "BEGIN RSA PRIVATE KEY") &&
		!strings.Contains(p, "BEGIN PRIVATE KEY") &&
		!strings.Contains(p, "BEGIN ENCRYPTED PRIVATE KEY") {
		return errors.New("私钥 PEM 须以 -----BEGIN RSA PRIVATE KEY----- 或 -----BEGIN PRIVATE KEY----- 开头（PKCS#1 / PKCS#8）；支付宝应用私钥一般为未加密 PEM")
	}
	if strings.Contains(p, "BEGIN ENCRYPTED PRIVATE KEY") {
		return errors.New("不支持加密的私钥 PEM，请使用未加密的 PKCS#8 或 PKCS#1 应用私钥")
	}
	return nil
}

func wrapNewClientKeyError(err error) error {
	if err == nil {
		return nil
	}
	msg := err.Error()
	if strings.Contains(msg, "asn1") || strings.Contains(msg, "structure error") {
		return fmt.Errorf("%w；常见原因：1) ALIPAY_APP_PRIVATE_KEY_PATH 指向了证书/公钥而非应用私钥 2) PEM 多行在 env 里被截断或少了换行 3) 使用了非 RSA 密钥 4) 文件含 UTF-8 BOM（本服务已尝试去掉行首 BOM，请仍核对文件内容）", err)
	}
	return err
}

// NewPcClient 使用证书模式构建 PC 网站支付专用客户端（alipay.trade.page.pay）。
func NewPcClient(cfg *config.Config) (*alipay.Client, error) {
	if !cfg.AlipayPcCoreConfigured() {
		return nil, errors.New("alipay pc: core not configured")
	}
	pk := normalizeAppPrivateKeyPEM(cfg.AlipayPcAppPrivateKey)
	if p := strings.TrimSpace(cfg.AlipayPcAppPrivateKeyPath); p != "" {
		b, err := os.ReadFile(p)
		if err != nil {
			return nil, fmt.Errorf("alipay pc: read private key file: %w", err)
		}
		pk = normalizeAppPrivateKeyPEM(string(b))
	}
	if err := validateAppPrivateKeyPEMHeader(pk); err != nil {
		return nil, fmt.Errorf("alipay pc: invalid app private key PEM: %w", err)
	}
	cli, err := alipay.New(strings.TrimSpace(cfg.AlipayPcAppID), pk, cfg.AlipayProduction)
	if err != nil {
		return nil, fmt.Errorf("alipay pc: new client: %w", wrapNewClientKeyError(err))
	}
	if err := cli.LoadAppCertPublicKeyFromFile(strings.TrimSpace(cfg.AlipayPcAppCertPublicPath)); err != nil {
		return nil, fmt.Errorf("alipay pc: load app cert: %w", err)
	}
	if err := cli.LoadAlipayCertPublicKeyFromFile(strings.TrimSpace(cfg.AlipayPcPlatformCertPublicPath)); err != nil {
		return nil, fmt.Errorf("alipay pc: load alipay cert: %w", err)
	}
	if err := cli.LoadAliPayRootCertFromFile(strings.TrimSpace(cfg.AlipayPcRootCertPath)); err != nil {
		return nil, fmt.Errorf("alipay pc: load root cert: %w", err)
	}
	return cli, nil
}

// NewClient 使用证书模式：须同时配置应用私钥、应用公钥证书、支付宝公钥证书、支付宝根证书。
func NewClient(cfg *config.Config) (*alipay.Client, error) {
	if !cfg.AlipayCoreConfigured() {
		return nil, errors.New("alipay: core not configured")
	}
	pk := normalizeAppPrivateKeyPEM(cfg.AlipayAppPrivateKey)
	if p := strings.TrimSpace(cfg.AlipayAppPrivateKeyPath); p != "" {
		b, err := os.ReadFile(p)
		if err != nil {
			return nil, fmt.Errorf("alipay: read private key file: %w", err)
		}
		pk = normalizeAppPrivateKeyPEM(string(b))
	}
	if err := validateAppPrivateKeyPEMHeader(pk); err != nil {
		return nil, fmt.Errorf("alipay: invalid app private key PEM: %w", err)
	}
	cli, err := alipay.New(strings.TrimSpace(cfg.AlipayAppID), pk, cfg.AlipayProduction)
	if err != nil {
		return nil, fmt.Errorf("alipay: new client: %w", wrapNewClientKeyError(err))
	}
	if err := cli.LoadAppCertPublicKeyFromFile(strings.TrimSpace(cfg.AlipayAppCertPublicPath)); err != nil {
		return nil, fmt.Errorf("alipay: load app cert: %w", err)
	}
	if err := cli.LoadAlipayCertPublicKeyFromFile(strings.TrimSpace(cfg.AlipayPlatformCertPublicPath)); err != nil {
		return nil, fmt.Errorf("alipay: load alipay cert: %w", err)
	}
	if err := cli.LoadAliPayRootCertFromFile(strings.TrimSpace(cfg.AlipayRootCertPath)); err != nil {
		return nil, fmt.Errorf("alipay: load root cert: %w", err)
	}
	return cli, nil
}
