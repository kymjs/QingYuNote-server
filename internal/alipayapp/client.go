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

// NewClient 使用证书模式：须同时配置应用私钥、应用公钥证书、支付宝公钥证书、支付宝根证书。
func NewClient(cfg *config.Config) (*alipay.Client, error) {
	if !cfg.AlipayCoreConfigured() {
		return nil, errors.New("alipay: core not configured")
	}
	pk := strings.TrimSpace(cfg.AlipayAppPrivateKey)
	if p := strings.TrimSpace(cfg.AlipayAppPrivateKeyPath); p != "" {
		b, err := os.ReadFile(p)
		if err != nil {
			return nil, fmt.Errorf("alipay: read private key file: %w", err)
		}
		pk = string(b)
	}
	cli, err := alipay.New(strings.TrimSpace(cfg.AlipayAppID), pk, cfg.AlipayProduction)
	if err != nil {
		return nil, fmt.Errorf("alipay: new client: %w", err)
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
