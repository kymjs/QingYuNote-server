package wxnotify

import (
	"crypto/x509"
	"encoding/pem"
	"errors"
	"fmt"
	"os"
	"strings"

	"github.com/wechatpay-apiv3/wechatpay-go/core"
	"github.com/wechatpay-apiv3/wechatpay-go/core/auth"
	"github.com/wechatpay-apiv3/wechatpay-go/core/auth/verifiers"
)

func LoadVerifierFromPEMFile(path string) (auth.Verifier, error) {
	raw, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	certs, err := parseCertificatesPEM(raw)
	if err != nil {
		return nil, err
	}
	if len(certs) == 0 {
		return nil, errors.New("no certificates in PEM file")
	}
	return verifiers.NewSHA256WithRSAVerifier(core.NewCertificateMapWithList(certs)), nil
}

func parseCertificatesPEM(raw []byte) ([]*x509.Certificate, error) {
	var out []*x509.Certificate
	rest := raw
	for len(rest) > 0 {
		var block *pem.Block
		block, rest = pem.Decode(rest)
		if block == nil {
			break
		}
		if block.Type != "CERTIFICATE" {
			continue
		}
		c, err := x509.ParseCertificate(block.Bytes)
		if err != nil {
			return nil, fmt.Errorf("parse cert: %w", err)
		}
		out = append(out, c)
	}
	return out, nil
}

func TrimPEM(s string) string {
	return strings.TrimSpace(s)
}
