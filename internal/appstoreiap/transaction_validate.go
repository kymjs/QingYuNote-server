package appstoreiap

import (
	"errors"
	"fmt"
	"strings"

	appstore "github.com/laishere/app-store-server-library-go"
)

var (
	// ErrTransactionRevoked 交易已被撤销 / 退款（RevocationDate 存在）。
	ErrTransactionRevoked = errors.New("apple transaction revoked")
	// ErrMissingBundleInPayload JWS 载荷中缺少 bundleId。
	ErrMissingBundleInPayload = errors.New("missing bundle id in apple transaction")
	// ErrBundleMismatch 与配置的 APPLE_IAP_BUNDLE_ID 不一致。
	ErrBundleMismatch = errors.New("apple transaction bundle id mismatch")
	// ErrInvalidTransactionType 非 App Store 定义的 IAP 类型。
	ErrInvalidTransactionType = errors.New("invalid apple in-app purchase type")
	// ErrMissingPurchaseDate 缺少购买时间，视为未完成扣款的有效凭据。
	ErrMissingPurchaseDate = errors.New("missing apple purchase date")
)

// ValidateTransactionEligibleForCredit 在校验 JWS 签名通过后，进一步判断交易是否处于「已购买且应入账」状态。
func ValidateTransactionEligibleForCredit(p *appstore.JWSTransactionDecodedPayload, wantBundleID string) error {
	if p == nil {
		return fmt.Errorf("nil payload")
	}
	if p.RevocationDate != nil && !p.RevocationDate.IsZero() {
		return fmt.Errorf("%w", ErrTransactionRevoked)
	}
	got := strings.TrimSpace(p.BundleId)
	want := strings.TrimSpace(wantBundleID)
	if got == "" {
		return fmt.Errorf("%w", ErrMissingBundleInPayload)
	}
	if want != "" && got != want {
		return fmt.Errorf("%w", ErrBundleMismatch)
	}
	if !p.Type.IsValid() {
		return fmt.Errorf("%w", ErrInvalidTransactionType)
	}
	if p.PurchaseDate.IsZero() {
		return fmt.Errorf("%w", ErrMissingPurchaseDate)
	}
	return nil
}

// PaymentVerifiedFields 供成功响应附加给客户端的苹果侧状态字段（不含敏感信息）。
func PaymentVerifiedFields(p *appstore.JWSTransactionDecodedPayload, transactionID string) map[string]any {
	out := map[string]any{
		"payment_verified":     true,
		"apple_transaction_id": strings.TrimSpace(transactionID),
	}
	if p == nil {
		return out
	}
	if pid := strings.TrimSpace(p.ProductId); pid != "" {
		out["apple_product_id"] = pid
	}
	if p.Environment.IsValid() {
		out["apple_environment"] = p.Environment.Raw()
	}
	if p.Type.IsValid() {
		out["apple_transaction_type"] = p.Type.Raw()
	}
	if !p.PurchaseDate.IsZero() {
		out["apple_purchase_date_ms"] = p.PurchaseDate.UnixMilli()
	}
	return out
}
