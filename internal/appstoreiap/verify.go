package appstoreiap

import (
	_ "embed"
	"errors"
	"fmt"

	appstore "github.com/laishere/app-store-server-library-go"
)

//go:embed AppleRootCA-G3.cer
var appleRootCA []byte

// VerifySignedTransaction 校验 StoreKit 2 / App Store Server API 格式的 signedTransaction（JWS）。
// 依次尝试 Sandbox、Xcode（本地 StoreKit 配置）、Production（需配置 App Store Connect 数字 App ID）。
func VerifySignedTransaction(bundleID string, appStoreAppID int64, signedTx string) (*appstore.JWSTransactionDecodedPayload, error) {
	if signedTx == "" {
		return nil, errors.New("empty signed_transaction")
	}
	roots := [][]byte{appleRootCA}
	attempts := []struct {
		env   appstore.Environment
		appID int64
	}{
		{appstore.ENVIRONMENT_SANDBOX, 0},
		{appstore.ENVIRONMENT_XCODE, 0},
	}
	if appStoreAppID != 0 {
		attempts = append(attempts, struct {
			env   appstore.Environment
			appID int64
		}{appstore.ENVIRONMENT_PRODUCTION, appStoreAppID})
	}
	var lastErr error
	for _, a := range attempts {
		v, err := appstore.NewSignedDataVerifier(roots, false, a.env, bundleID, a.appID)
		if err != nil {
			lastErr = err
			continue
		}
		p, err := v.VerifyAndDecodeSignedTransaction(signedTx)
		if err == nil {
			return p, nil
		}
		lastErr = err
	}
	if lastErr == nil {
		lastErr = errors.New("verification failed")
	}
	return nil, fmt.Errorf("apple signed transaction: %w", lastErr)
}
