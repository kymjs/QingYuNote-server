package wxpay

import (
	"crypto"
	"crypto/rand"
	"crypto/rsa"
	"crypto/sha256"
	"encoding/base64"
	"fmt"
	"time"
)

// AppInvokeSign 生成 APP 调起支付所需签名（APIv3）：appId\ntimeStamp\nnonceStr\nprepayId\n
func AppInvokeSign(appID, prepayID string, privateKey *rsa.PrivateKey) (timeStamp, nonceStr, sign string, err error) {
	ts := fmt.Sprintf("%d", time.Now().Unix())
	nonce, err := randomNonce(32)
	if err != nil {
		return "", "", "", err
	}
	msg := fmt.Sprintf("%s\n%s\n%s\n%s\n", appID, ts, nonce, prepayID)
	sum := sha256.Sum256([]byte(msg))
	sig, err := rsa.SignPKCS1v15(rand.Reader, privateKey, crypto.SHA256, sum[:])
	if err != nil {
		return "", "", "", err
	}
	return ts, nonce, base64.StdEncoding.EncodeToString(sig), nil
}

func randomNonce(n int) (string, error) {
	const letters = "abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ0123456789"
	b := make([]byte, n)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	for i := range b {
		b[i] = letters[int(b[i])%len(letters)]
	}
	return string(b), nil
}
