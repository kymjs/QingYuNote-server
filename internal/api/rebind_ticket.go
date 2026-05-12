package api

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"errors"
	"strings"
	"time"
)

const rebindTicketVersion = 1
const rebindTicketTTL = 10 * time.Minute

type rebindTicketPayload struct {
	V int    `json:"v"`
	P string `json:"p"`
	S string `json:"s"`
	U int64  `json:"u"`
	E int64  `json:"e"`
}

func issueIdentityRebindTicket(secret, provider, subject string, survivorUserID int64) (string, error) {
	sec := strings.TrimSpace(secret)
	if sec == "" {
		return "", errors.New("empty_secret")
	}
	p := strings.TrimSpace(provider)
	s := strings.TrimSpace(subject)
	if p == "" || s == "" || survivorUserID <= 0 {
		return "", errors.New("invalid_ticket_fields")
	}
	pl := rebindTicketPayload{
		V: rebindTicketVersion,
		P: p,
		S: s,
		U: survivorUserID,
		E: time.Now().UTC().Add(rebindTicketTTL).Unix(),
	}
	raw, err := json.Marshal(pl)
	if err != nil {
		return "", err
	}
	b64 := base64.RawURLEncoding.EncodeToString(raw)
	mac := hmac.New(sha256.New, []byte(sec))
	_, _ = mac.Write([]byte(b64))
	sig := base64.RawURLEncoding.EncodeToString(mac.Sum(nil))
	return b64 + "." + sig, nil
}

func parseIdentityRebindTicket(secret, ticket string) (provider, subject string, survivorUserID int64, err error) {
	sec := strings.TrimSpace(secret)
	if sec == "" {
		return "", "", 0, errors.New("empty_secret")
	}
	parts := strings.Split(strings.TrimSpace(ticket), ".")
	if len(parts) != 2 {
		return "", "", 0, errors.New("invalid_ticket_format")
	}
	b64, sigB64 := parts[0], parts[1]
	mac := hmac.New(sha256.New, []byte(sec))
	_, _ = mac.Write([]byte(b64))
	wantSig := base64.RawURLEncoding.EncodeToString(mac.Sum(nil))
	gotSig, err := base64.RawURLEncoding.DecodeString(sigB64)
	if err != nil {
		return "", "", 0, errors.New("invalid_ticket_sig")
	}
	wantBytes, err := base64.RawURLEncoding.DecodeString(wantSig)
	if err != nil {
		return "", "", 0, err
	}
	if !hmac.Equal(gotSig, wantBytes) {
		return "", "", 0, errors.New("ticket_sig_mismatch")
	}
	raw, err := base64.RawURLEncoding.DecodeString(b64)
	if err != nil {
		return "", "", 0, errors.New("invalid_ticket_body")
	}
	var pl rebindTicketPayload
	if err := json.Unmarshal(raw, &pl); err != nil {
		return "", "", 0, err
	}
	if pl.V != rebindTicketVersion || pl.U <= 0 || strings.TrimSpace(pl.P) == "" || strings.TrimSpace(pl.S) == "" {
		return "", "", 0, errors.New("invalid_ticket_payload")
	}
	if time.Now().UTC().Unix() > pl.E {
		return "", "", 0, errors.New("ticket_expired")
	}
	return strings.TrimSpace(pl.P), strings.TrimSpace(pl.S), pl.U, nil
}
