package api

import (
	"database/sql"
	"errors"
	"log"
	"net/http"
	"strings"
	"time"

	"golang.org/x/crypto/bcrypt"

	"github.com/kymjs/noteapi/internal/aliyunsms"
	"github.com/kymjs/noteapi/internal/qingyuwebdav"
	"github.com/kymjs/noteapi/internal/store"
	"github.com/kymjs/noteapi/internal/subscription"
)

const deleteAccountConfirmPhrase = "delete_account"

type profileWire struct {
	HuaweiLinked          bool    `json:"huawei_linked"`
	WechatLinked          bool    `json:"wechat_linked"`
	AppleLinked           bool    `json:"apple_linked"`
	Username              *string `json:"username"`
	AvatarURL             *string `json:"avatar_url"`
	MembershipLevel       string  `json:"membership_level"`
	MembershipExpiresAt   string  `json:"membership_expires_at,omitempty"`
	MembershipIsLifetime  bool    `json:"membership_is_lifetime"`
	RegisteredAt          string  `json:"registered_at"`
	Phone                 *string `json:"phone"`
	Email                 *string `json:"email"`
	PasswordSet           bool    `json:"password_set"`
	QingyuSubscriptionOK  bool    `json:"qingyu_subscription_active"`
}

func strPtrOrNil(ns sql.NullString) *string {
	if !ns.Valid {
		return nil
	}
	s := ns.String
	return &s
}

func (s *Server) handleGetProfile(w http.ResponseWriter, r *http.Request, uid int64) {
	if r.Method != http.MethodGet {
		http.Error(w, `{"error":"method_not_allowed"}`, http.StatusMethodNotAllowed)
		return
	}
	ctx := r.Context()
	u, err := s.Store.GetUserByID(ctx, uid)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			writeJSON(w, http.StatusNotFound, map[string]string{"error": "user_not_found"})
			return
		}
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "db_failed"})
		return
	}
	wc, hw, ap, err := s.Store.IdentityBindings(ctx, uid)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "db_failed"})
		return
	}
	sub, err := s.Store.GetSubscription(ctx, uid)
	if err != nil && !errors.Is(err, sql.ErrNoRows) {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "db_failed"})
		return
	}
	if errors.Is(err, sql.ErrNoRows) {
		sub = nil
	}
	state, expYmd, life := subscription.RowToAPIState(sub, time.Now().UTC())
	qingyuOK := state == "active" || state == "lifetime"

	resp := profileWire{
		HuaweiLinked:         hw,
		WechatLinked:         wc,
		AppleLinked:          ap,
		Username:             strPtrOrNil(u.DisplayName),
		AvatarURL:            strPtrOrNil(u.AvatarURL),
		MembershipLevel:      state,
		MembershipExpiresAt:  expYmd,
		MembershipIsLifetime: life,
		RegisteredAt:         u.CreatedAt.UTC().Format(time.RFC3339),
		Phone:                strPtrOrNil(u.Phone),
		Email:                strPtrOrNil(u.Email),
		PasswordSet:          u.PasswordHash.Valid && strings.TrimSpace(u.PasswordHash.String) != "",
		QingyuSubscriptionOK: qingyuOK,
	}
	writeJSON(w, http.StatusOK, resp)
}

type patchProfileReq struct {
	Username       *string `json:"username"`
	AvatarURL      *string `json:"avatar_url"`
	Phone          *string `json:"phone"`
	Email          *string `json:"email"`
	OldPassword    *string `json:"old_password"`
	NewPassword    *string `json:"new_password"`
	ClearPassword  *bool   `json:"clear_password"`
	// SmsVerifyCode 非空时，修改密码走短信核验（与阿里云 CheckSmsVerifyCode），不再要求 old_password。
	SmsVerifyCode *string `json:"sms_verify_code"`
}

func (s *Server) handlePatchProfile(w http.ResponseWriter, r *http.Request, uid int64) {
	if r.Method != http.MethodPatch {
		http.Error(w, `{"error":"method_not_allowed"}`, http.StatusMethodNotAllowed)
		return
	}
	ctx := r.Context()
	u, err := s.Store.GetUserByID(ctx, uid)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			writeJSON(w, http.StatusNotFound, map[string]string{"error": "user_not_found"})
			return
		}
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "db_failed"})
		return
	}
	var req patchProfileReq
	if err := readJSON(r, &req); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid_body"})
		return
	}

	passwordSet := u.PasswordHash.Valid && strings.TrimSpace(u.PasswordHash.String) != ""

	newPw := ""
	if req.NewPassword != nil {
		newPw = strings.TrimSpace(*req.NewPassword)
	}
	if req.Phone != nil {
		trimmed := strings.TrimSpace(ptrStr(req.Phone))
		if trimmed != "" && newPw != "" {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid_body"})
			return
		}
	}

	// 修改手机号：非空新号码用短信核验（目标号码）；清空号码用短信核验当前绑定号或原密码。
	if req.Phone != nil {
		trimmed := strings.TrimSpace(ptrStr(req.Phone))
		if trimmed != "" {
			if !s.Cfg.AliyunSMSConfigured() {
				writeJSON(w, http.StatusServiceUnavailable, map[string]string{"error": "sms_not_configured"})
				return
			}
			smsCode := strings.TrimSpace(ptrStr(req.SmsVerifyCode))
			if smsCode == "" {
				writeJSON(w, http.StatusBadRequest, map[string]string{"error": "sms_verify_code_required"})
				return
			}
			newDigits := store.NormalizeLoginPhoneDigits(trimmed)
			if newDigits == "" {
				writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid_phone"})
				return
			}
			cli, err := aliyunsms.NewClient(s.Cfg.AliyunSMSRegion, s.Cfg.AliyunAccessKeyID, s.Cfg.AliyunAccessKeySecret)
			if err != nil {
				writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "sms_client_failed"})
				return
			}
			params := aliyunsms.SMSParams{
				SignName:      s.Cfg.AliyunSMSSignName,
				TemplateCode:  s.Cfg.AliyunSMSTemplateCode,
				SchemeName:    s.Cfg.AliyunSMSSchemeName,
				TemplateParam: s.Cfg.AliyunSMSTemplateParam,
			}
			ok, err := aliyunsms.CheckVerifyCode(cli, params, newDigits, smsCode)
			if err != nil || !ok {
				writeJSON(w, http.StatusForbidden, map[string]string{"error": "sms_code_invalid"})
				return
			}
		} else {
			hadPhone := u.Phone.Valid && strings.TrimSpace(u.Phone.String) != ""
			if hadPhone {
				smsCode := strings.TrimSpace(ptrStr(req.SmsVerifyCode))
				old := strings.TrimSpace(ptrStr(req.OldPassword))
				var verified bool
				if s.Cfg.AliyunSMSConfigured() && smsCode != "" {
					dbDigits := store.NormalizeLoginPhoneDigits(u.Phone.String)
					if dbDigits == "" {
						writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid_phone"})
						return
					}
					cli, err := aliyunsms.NewClient(s.Cfg.AliyunSMSRegion, s.Cfg.AliyunAccessKeyID, s.Cfg.AliyunAccessKeySecret)
					if err != nil {
						writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "sms_client_failed"})
						return
					}
					params := aliyunsms.SMSParams{
						SignName:      s.Cfg.AliyunSMSSignName,
						TemplateCode:  s.Cfg.AliyunSMSTemplateCode,
						SchemeName:    s.Cfg.AliyunSMSSchemeName,
						TemplateParam: s.Cfg.AliyunSMSTemplateParam,
					}
					ok, err := aliyunsms.CheckVerifyCode(cli, params, dbDigits, smsCode)
					verified = err == nil && ok
				}
				if !verified && passwordSet && old != "" {
					if err := bcrypt.CompareHashAndPassword([]byte(u.PasswordHash.String), []byte(old)); err == nil {
						verified = true
					}
				}
				if !verified {
					if !s.Cfg.AliyunSMSConfigured() && passwordSet {
						if old == "" {
							writeJSON(w, http.StatusBadRequest, map[string]string{"error": "old_password_required"})
							return
						}
						writeJSON(w, http.StatusForbidden, map[string]string{"error": "old_password_invalid"})
						return
					}
					if !s.Cfg.AliyunSMSConfigured() && !passwordSet {
						writeJSON(w, http.StatusServiceUnavailable, map[string]string{"error": "sms_not_configured"})
						return
					}
					if smsCode != "" {
						writeJSON(w, http.StatusForbidden, map[string]string{"error": "sms_code_invalid"})
						return
					}
					if passwordSet && old != "" {
						writeJSON(w, http.StatusForbidden, map[string]string{"error": "old_password_invalid"})
						return
					}
					writeJSON(w, http.StatusBadRequest, map[string]string{"error": "sms_verify_code_required"})
					return
				}
			}
		}
	}

	if req.ClearPassword != nil && *req.ClearPassword {
		if !passwordSet {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "password_not_set"})
			return
		}
		old := strings.TrimSpace(ptrStr(req.OldPassword))
		if old == "" {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "old_password_required"})
			return
		}
		if err := bcrypt.CompareHashAndPassword([]byte(u.PasswordHash.String), []byte(old)); err != nil {
			writeJSON(w, http.StatusForbidden, map[string]string{"error": "old_password_invalid"})
			return
		}
		if err := s.Store.SetUserPasswordHash(ctx, uid, nil); err != nil {
			writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "db_failed"})
			return
		}
		passwordSet = false
	}

	if strings.TrimSpace(newPw) != "" {
		if len(strings.TrimSpace(newPw)) < 8 {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "password_too_short"})
			return
		}
		smsCode := strings.TrimSpace(ptrStr(req.SmsVerifyCode))
		if smsCode != "" {
			if !s.Cfg.AliyunSMSConfigured() {
				writeJSON(w, http.StatusServiceUnavailable, map[string]string{"error": "sms_not_configured"})
				return
			}
			if !u.Phone.Valid || strings.TrimSpace(u.Phone.String) == "" {
				writeJSON(w, http.StatusBadRequest, map[string]string{"error": "phone_not_bound"})
				return
			}
			dbDigits := store.NormalizeLoginPhoneDigits(u.Phone.String)
			cli, err := aliyunsms.NewClient(s.Cfg.AliyunSMSRegion, s.Cfg.AliyunAccessKeyID, s.Cfg.AliyunAccessKeySecret)
			if err != nil {
				writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "sms_client_failed"})
				return
			}
			params := aliyunsms.SMSParams{
				SignName:      s.Cfg.AliyunSMSSignName,
				TemplateCode:  s.Cfg.AliyunSMSTemplateCode,
				SchemeName:    s.Cfg.AliyunSMSSchemeName,
				TemplateParam: s.Cfg.AliyunSMSTemplateParam,
			}
			ok, err := aliyunsms.CheckVerifyCode(cli, params, dbDigits, smsCode)
			if err != nil || !ok {
				writeJSON(w, http.StatusForbidden, map[string]string{"error": "sms_code_invalid"})
				return
			}
		} else if passwordSet {
			old := strings.TrimSpace(ptrStr(req.OldPassword))
			if old == "" {
				writeJSON(w, http.StatusBadRequest, map[string]string{"error": "old_password_required"})
				return
			}
			if err := bcrypt.CompareHashAndPassword([]byte(u.PasswordHash.String), []byte(old)); err != nil {
				writeJSON(w, http.StatusForbidden, map[string]string{"error": "old_password_invalid"})
				return
			}
		}
		hash, err := bcrypt.GenerateFromPassword([]byte(strings.TrimSpace(newPw)), bcrypt.DefaultCost)
		if err != nil {
			writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "hash_failed"})
			return
		}
		sHash := string(hash)
		if err := s.Store.SetUserPasswordHash(ctx, uid, &sHash); err != nil {
			writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "db_failed"})
			return
		}
	}

	if err := s.Store.PatchUserProfile(ctx, uid, req.Username, req.AvatarURL, req.Phone, req.Email); err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "db_failed"})
		return
	}
	writeJSON(w, http.StatusOK, map[string]bool{"ok": true})
}

func ptrStr(p *string) string {
	if p == nil {
		return ""
	}
	return *p
}

type deleteAccountReq struct {
	Confirm string `json:"confirm"`
}

func (s *Server) handleDeleteAccount(w http.ResponseWriter, r *http.Request, uid int64) {
	if r.Method != http.MethodDelete {
		http.Error(w, `{"error":"method_not_allowed"}`, http.StatusMethodNotAllowed)
		return
	}
	var req deleteAccountReq
	if err := readJSON(r, &req); err != nil || strings.TrimSpace(req.Confirm) != deleteAccountConfirmPhrase {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid_body"})
		return
	}
	ctx := r.Context()

	if s.Cfg.QingyuWebDAVConfigured() {
		notesDir := store.QingyuNotesDirForAuthenticatedUser(uid)
		err := qingyuwebdav.DeleteNotesTree(ctx, s.Cfg.QingyuWebDAVBaseURL, s.Cfg.QingyuWebDAVUsername, s.Cfg.QingyuWebDAVPassword, notesDir)
		if err != nil {
			log.Printf("qingyu folder delete failed uid=%d: %v", uid, err)
			writeJSON(w, http.StatusBadGateway, map[string]string{"error": "qingyu_folder_delete_failed"})
			return
		}
	}

	if err := s.Store.DeleteUserByID(ctx, uid); err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "db_failed"})
		return
	}
	writeJSON(w, http.StatusOK, map[string]bool{"ok": true})
}
