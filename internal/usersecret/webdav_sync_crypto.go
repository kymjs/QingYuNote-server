package usersecret

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/md5"
	"encoding/base64"
	"encoding/hex"
	"errors"
	"fmt"
	"strconv"
	"strings"
)

const webdavSyncAAD = "qingyu-webdav-sync-aad"
const qingyunoteSuffix = "qingyunote"

// DeriveWebDAVSyncKeyBytes 与客户端一致：MD5( MD5(userID) 十六进制 + "qingyunote" ) 的 16 字节摘要。
func DeriveWebDAVSyncKeyBytes(userID int64) []byte {
	idStr := strconv.FormatInt(userID, 10)
	idMD5 := md5.Sum([]byte(idStr))
	hexStr := hex.EncodeToString(idMD5[:])
	combined := hexStr + qingyunoteSuffix
	key := md5.Sum([]byte(combined))
	return key[:]
}

// DecryptWebDAVSyncField 解密客户端 AES-GCM 密文（格式 nonce.ciphertext+tag）。
func DecryptWebDAVSyncField(cipherText string, userID int64) (string, error) {
	cipherText = strings.TrimSpace(cipherText)
	if cipherText == "" {
		return "", nil
	}
	segments := strings.Split(cipherText, ".")
	if len(segments) != 2 && len(segments) != 3 {
		return "", errors.New("invalid encrypted payload format")
	}
	nonce, err := base64.StdEncoding.DecodeString(segments[0])
	if err != nil {
		return "", fmt.Errorf("invalid nonce: %w", err)
	}
	var cipherBytes, macBytes []byte
	if len(segments) == 2 {
		combined, err := base64.StdEncoding.DecodeString(segments[1])
		if err != nil {
			return "", fmt.Errorf("invalid ciphertext: %w", err)
		}
		if len(combined) <= 16 {
			return "", errors.New("invalid encrypted payload length")
		}
		cipherBytes = combined[:len(combined)-16]
		macBytes = combined[len(combined)-16:]
	} else {
		macBytes, err = base64.StdEncoding.DecodeString(segments[1])
		if err != nil {
			return "", fmt.Errorf("invalid mac: %w", err)
		}
		cipherBytes, err = base64.StdEncoding.DecodeString(segments[2])
		if err != nil {
			return "", fmt.Errorf("invalid ciphertext: %w", err)
		}
	}
	block, err := aes.NewCipher(DeriveWebDAVSyncKeyBytes(userID))
	if err != nil {
		return "", err
	}
	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return "", err
	}
	sealed := append(append([]byte{}, cipherBytes...), macBytes...)
	plain, err := gcm.Open(nil, nonce, sealed, []byte(webdavSyncAAD))
	if err != nil {
		return "", fmt.Errorf("decrypt failed: %w", err)
	}
	return string(plain), nil
}
