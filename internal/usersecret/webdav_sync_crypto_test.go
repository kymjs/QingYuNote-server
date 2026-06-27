package usersecret_test

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/md5"
	"crypto/rand"
	"encoding/base64"
	"encoding/hex"
	"strconv"
	"testing"

	"github.com/kymjs/noteapi/internal/usersecret"
)

func encryptLikeClient(t *testing.T, plain string, userID int64) string {
	t.Helper()
	if plain == "" {
		return ""
	}
	block, err := aes.NewCipher(usersecret.DeriveWebDAVSyncKeyBytes(userID))
	if err != nil {
		t.Fatal(err)
	}
	gcm, err := cipher.NewGCM(block)
	if err != nil {
		t.Fatal(err)
	}
	nonce := make([]byte, 12)
	if _, err := rand.Read(nonce); err != nil {
		t.Fatal(err)
	}
	sealed := gcm.Seal(nil, nonce, []byte(plain), []byte("qingyu-webdav-sync-aad"))
	return base64.StdEncoding.EncodeToString(nonce) + "." + base64.StdEncoding.EncodeToString(sealed)
}

func TestDeriveWebDAVSyncKeyBytes(t *testing.T) {
	key := usersecret.DeriveWebDAVSyncKeyBytes(42)
	if len(key) != 16 {
		t.Fatalf("expected 16 bytes, got %d", len(key))
	}
	idStr := strconv.FormatInt(42, 10)
	sum := md5.Sum([]byte(idStr))
	combined := hex.EncodeToString(sum[:]) + "qingyunote"
	want := md5.Sum([]byte(combined))
	if string(key) != string(want[:]) {
		t.Fatal("key mismatch")
	}
}

func TestDecryptWebDAVSyncFieldRoundTrip(t *testing.T) {
	const uid int64 = 1001
	cipher := encryptLikeClient(t, "secret-pass", uid)
	plain, err := usersecret.DecryptWebDAVSyncField(cipher, uid)
	if err != nil {
		t.Fatal(err)
	}
	if plain != "secret-pass" {
		t.Fatalf("got %q", plain)
	}
}
