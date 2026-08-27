package api

import (
	"bytes"
	"context"
	"database/sql"
	"encoding/base64"
	"errors"
	"fmt"
	"io"
	"log"
	"net/http"
	"strings"
	"time"

	"github.com/kymjs/noteapi/internal/avatarwebdav"
	"github.com/kymjs/noteapi/internal/minimax"
	"github.com/kymjs/noteapi/internal/store"
	"github.com/kymjs/noteapi/internal/subscription"
)

const maxAppStoreReviewBytes = 10 << 20

type appStoreReviewStatusWire struct {
	Status              string `json:"status"`
	RejectReason        string `json:"reject_reason,omitempty"`
	MembershipExpiresAt string `json:"membership_expires_at,omitempty"`
}

func (s *Server) handleGetAppStoreReviewCampaign(w http.ResponseWriter, r *http.Request, uid int64) {
	if r.Method != http.MethodGet {
		http.Error(w, `{"error":"method_not_allowed"}`, http.StatusMethodNotAllowed)
		return
	}
	writeJSON(w, http.StatusOK, s.appStoreReviewStatusPayload(r.Context(), uid))
}

func (s *Server) appStoreReviewStatusPayload(ctx context.Context, uid int64) appStoreReviewStatusWire {
	row, err := s.Store.GetAppStoreReviewCampaign(ctx, uid)
	if err != nil {
		log.Printf("app store review get uid=%d: %v", uid, err)
		return appStoreReviewStatusWire{Status: "not_uploaded"}
	}
	out := appStoreReviewStatusWire{Status: "not_uploaded"}
	if row != nil {
		out.Status = row.Status
		if row.Status == store.AppStoreReviewRejected {
			out.RejectReason = row.RejectReason
		}
	}
	sub, err := s.Store.GetSubscription(ctx, uid)
	if err != nil && !errors.Is(err, sql.ErrNoRows) {
		return out
	}
	if errors.Is(err, sql.ErrNoRows) {
		sub = nil
	}
	_, expYmd, _ := subscription.RowToAPIState(sub, time.Now().UTC())
	out.MembershipExpiresAt = expYmd
	return out
}

func (s *Server) handlePostAppStoreReviewScreenshot(w http.ResponseWriter, r *http.Request, uid int64) {
	if r.Method != http.MethodPost {
		http.Error(w, `{"error":"method_not_allowed"}`, http.StatusMethodNotAllowed)
		return
	}
	if !s.Cfg.AvatarWebDAVConfigured() {
		writeJSON(w, http.StatusServiceUnavailable, map[string]string{"error": "avatar_webdav_not_configured"})
		return
	}
	if !s.Cfg.MiniMaxConfigured() {
		writeJSON(w, http.StatusServiceUnavailable, map[string]string{"error": "minimax_not_configured"})
		return
	}
	if err := r.ParseMultipartForm(maxAppStoreReviewBytes + (1 << 20)); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid_multipart"})
		return
	}
	f, _, err := r.FormFile("file")
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "file_required"})
		return
	}
	defer f.Close()
	data, err := io.ReadAll(io.LimitReader(f, maxAppStoreReviewBytes+1))
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "read_failed"})
		return
	}
	if len(data) == 0 {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "empty_file"})
		return
	}
	if len(data) > maxAppStoreReviewBytes {
		writeJSON(w, http.StatusRequestEntityTooLarge, map[string]string{"error": "file_too_large"})
		return
	}
	ct, ext := sniffImage(data)
	if ct == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid_image_type"})
		return
	}
	fname := fmt.Sprintf("%d%s", uid, ext)
	if err := avatarwebdav.PutFile(r.Context(), s.Cfg.AvatarWebDAVBaseURL, s.Cfg.AvatarWebDAVUsername, s.Cfg.AvatarWebDAVPassword,
		fname, bytes.NewReader(data), ct, int64(len(data))); err != nil {
		log.Printf("app store review webdav put uid=%d: %v", uid, err)
		writeJSON(w, http.StatusBadGateway, map[string]string{"error": avatarWebDAVErrorCode(err)})
		return
	}
	if err := s.Store.BeginAppStoreReviewUpload(r.Context(), uid, fname); err != nil {
		if store.IsAppStoreReviewAlreadyApproved(err) {
			writeJSON(w, http.StatusConflict, map[string]string{"error": "already_approved"})
			return
		}
		if store.IsAppStoreReviewAlreadyReviewing(err) {
			writeJSON(w, http.StatusConflict, map[string]string{"error": "already_reviewing"})
			return
		}
		log.Printf("app store review begin uid=%d: %v", uid, err)
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "db_failed"})
		return
	}
	imgCopy := append([]byte(nil), data...)
	go s.processAppStoreReview(uid, fname, imgCopy, ct)
	writeJSON(w, http.StatusOK, s.appStoreReviewStatusPayload(r.Context(), uid))
}

func (s *Server) processAppStoreReview(uid int64, filename string, image []byte, contentType string) {
	ctx, cancel := context.WithTimeout(context.Background(), 90*time.Second)
	defer cancel()
	imgRef := s.appStoreReviewImageRef(filename, contentType, image)
	verdict, err := minimax.ReviewAppStoreScreenshot(ctx, s.Cfg.MiniMaxBaseURL, s.Cfg.MiniMaxTokenPlanKey, s.Cfg.MiniMaxModel, imgRef, "请审核这张应用商店评价截图。")
	if err != nil {
		log.Printf("app store review minimax uid=%d: %v", uid, err)
		_ = s.Store.MarkAppStoreReviewRejected(ctx, uid, "审核服务暂时不可用，请稍后重新提交")
		return
	}
	ok, reason := minimax.ApplyHardRules(verdict)
	if !ok {
		if strings.TrimSpace(reason) == "" {
			reason = "截图未通过审核"
		}
		_ = s.Store.MarkAppStoreReviewRejected(ctx, uid, reason)
		return
	}
	if err := s.grantAppStoreReviewMembership(ctx, uid); err != nil {
		log.Printf("app store review grant uid=%d: %v", uid, err)
		_ = s.Store.MarkAppStoreReviewRejected(ctx, uid, "会员发放失败，请稍后重新提交")
		return
	}
	if err := s.Store.MarkAppStoreReviewApproved(ctx, uid); err != nil {
		log.Printf("app store review approve uid=%d: %v", uid, err)
	}
}

func (s *Server) appStoreReviewImageRef(filename, contentType string, image []byte) string {
	if contentType == "" {
		contentType = "image/jpeg"
	}
	return "data:" + contentType + ";base64," + base64.StdEncoding.EncodeToString(image)
}

func (s *Server) grantAppStoreReviewMembership(ctx context.Context, uid int64) error {
	now := time.Now().UTC()
	sub, err := s.Store.GetSubscription(ctx, uid)
	if err != nil && !errors.Is(err, sql.ErrNoRows) {
		return err
	}
	if errors.Is(err, sql.ErrNoRows) {
		sub = nil
	}
	newExp, lifetime := subscription.ExtendAfterPayment(sub, "monthly", now)
	if lifetime {
		if err := s.Store.UpsertSubscriptionExpiry(ctx, uid, time.Date(2099, 12, 31, 0, 0, 0, 0, time.UTC), true); err != nil {
			return err
		}
	} else {
		if err := s.Store.UpsertSubscriptionExpiry(ctx, uid, newExp, false); err != nil {
			return err
		}
	}
	if err := s.Store.InsertMembershipGrantRecord(ctx, &store.MembershipGrantRecordParams{
		UserID:      uid,
		Source:      store.GrantSourceAppStoreReview,
		GrantMonths: 1,
	}); err != nil {
		log.Printf("app store review grant audit uid=%d: %v", uid, err)
	}
	return nil
}
