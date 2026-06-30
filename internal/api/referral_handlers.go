package api

import (
	"database/sql"
	"errors"
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/kymjs/noteapi/internal/referral"
	"github.com/kymjs/noteapi/internal/store"
)

type referralClaimReq struct {
	InviterUID   int64  `json:"inviter_user_id"`
	InviteePhone string `json:"invitee_phone"`
}

type referralClaimResp struct {
	ClaimToken string `json:"claim_token"`
	ExpiresAt  string `json:"expires_at"`
}

type referralInviteHistoryItemWire struct {
	MaskedPhone string `json:"masked_phone"`
	Status      string `json:"status"`
	StatusLabel string `json:"status_label"`
	ClaimedAt   string `json:"claimed_at"`
}

func referralStatusLabel(status string) string {
	switch status {
	case store.ReferralClaimStatusSuccess:
		return "邀请成功"
	case store.ReferralClaimStatusFailed:
		return "邀请失败"
	case store.ReferralClaimStatusPending:
		return "邀请进行中"
	default:
		return status
	}
}

func shanghaiDayStartUTC(now time.Time) time.Time {
	loc, err := time.LoadLocation("Asia/Shanghai")
	if err != nil {
		loc = time.UTC
	}
	y, m, d := now.In(loc).Date()
	return time.Date(y, m, d, 0, 0, 0, 0, loc).UTC()
}

// handleReferralClaim POST /api/v1/referral/claim — 被邀请人提交手机号，绑定邀请关系。
func (s *Server) handleReferralClaim(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, `{"error":"method_not_allowed"}`, http.StatusMethodNotAllowed)
		return
	}
	var req referralClaimReq
	if err := readJSON(r, &req); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid_body"})
		return
	}
	if req.InviterUID <= 0 {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "inviter_user_id_required"})
		return
	}
	digits := store.NormalizeLoginPhoneDigits(req.InviteePhone)
	if digits == "" || !validChinaMobileDigits(digits) {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid_phone"})
		return
	}
	ctx := r.Context()
	if _, err := s.Store.GetUserByID(ctx, req.InviterUID); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			writeJSON(w, http.StatusNotFound, map[string]string{"error": "inviter_not_found"})
			return
		}
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "db_failed"})
		return
	}
	member, err := s.Store.UserHasActiveQingyuMembership(ctx, req.InviterUID)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "db_failed"})
		return
	}
	if !member {
		writeJSON(w, http.StatusForbidden, map[string]string{"error": "inviter_not_member"})
		return
	}
	now := time.Now().UTC()
	todayStart := shanghaiDayStartUTC(now)
	cnt, err := s.Store.CountReferralClaimsByInviterSince(ctx, req.InviterUID, todayStart)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "db_failed"})
		return
	}
	if cnt >= store.ReferralDailyInviteLimit {
		writeJSON(w, http.StatusTooManyRequests, map[string]string{"error": "inviter_daily_limit"})
		return
	}
	ever, err := s.Store.PhoneEverRegistered(ctx, digits)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "db_failed"})
		return
	}
	if ever {
		writeJSON(w, http.StatusConflict, map[string]string{"error": "invitee_already_registered"})
		return
	}
	referred, err := s.Store.InviteePhoneAlreadyReferred(ctx, digits, now)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "db_failed"})
		return
	}
	if referred {
		writeJSON(w, http.StatusConflict, map[string]string{"error": "invitee_already_referred"})
		return
	}
	token, err := referral.NewClaimToken()
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "token_failed"})
		return
	}
	if err := s.Store.InsertReferralClaimV2(ctx, token, req.InviterUID, digits, now, store.ReferralClaimStatusPending); err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "db_failed"})
		return
	}
	exp := now.Add(store.ReferralClaimValidWindow)
	writeJSON(w, http.StatusOK, referralClaimResp{
		ClaimToken: token,
		ExpiresAt:  exp.Format(time.RFC3339),
	})
}

// handleReferralHistory GET /api/v1/referral/history?inviter_user_id=
func (s *Server) handleReferralHistory(w http.ResponseWriter, r *http.Request) {
	uidStr := strings.TrimSpace(r.URL.Query().Get("inviter_user_id"))
	if uidStr == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "inviter_user_id_required"})
		return
	}
	inviterID, err := strconv.ParseInt(uidStr, 10, 64)
	if err != nil || inviterID <= 0 {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid_inviter_user_id"})
		return
	}
	ctx := r.Context()
	if _, err := s.Store.GetUserByID(ctx, inviterID); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			writeJSON(w, http.StatusNotFound, map[string]string{"error": "inviter_not_found"})
			return
		}
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "db_failed"})
		return
	}
	now := time.Now().UTC()
	rows, err := s.Store.ListReferralInviteHistory(ctx, inviterID, now)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "db_failed"})
		return
	}
	loc, _ := time.LoadLocation("Asia/Shanghai")
	if loc == nil {
		loc = time.UTC
	}
	out := make([]referralInviteHistoryItemWire, 0, len(rows))
	for _, row := range rows {
		out = append(out, referralInviteHistoryItemWire{
			MaskedPhone: row.MaskedPhone,
			Status:      row.Status,
			StatusLabel: referralStatusLabel(row.Status),
			ClaimedAt:   row.ClaimedAt.In(loc).Format("2006-01-02 15:04"),
		})
	}
	writeJSON(w, http.StatusOK, map[string]any{"invites": out})
}

type referralPopupEventReq struct {
	Variant int `json:"variant"`
}

func (s *Server) handleReferralPopupImpression(w http.ResponseWriter, r *http.Request, uid int64) {
	if r.Method != http.MethodPost {
		http.Error(w, `{"error":"method_not_allowed"}`, http.StatusMethodNotAllowed)
		return
	}
	var req referralPopupEventReq
	if err := readJSON(r, &req); err != nil || (req.Variant != 1 && req.Variant != 2) {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid_body"})
		return
	}
	ctx := r.Context()
	now := time.Now().UTC()
	_ = s.Store.InsertInvitePopupEvent(ctx, req.Variant, "impression", uid, now)
	writeJSON(w, http.StatusOK, map[string]bool{"ok": true})
}

func (s *Server) handleReferralPopupClick(w http.ResponseWriter, r *http.Request, uid int64) {
	if r.Method != http.MethodPost {
		http.Error(w, `{"error":"method_not_allowed"}`, http.StatusMethodNotAllowed)
		return
	}
	var req referralPopupEventReq
	if err := readJSON(r, &req); err != nil || (req.Variant != 1 && req.Variant != 2) {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid_body"})
		return
	}
	ctx := r.Context()
	now := time.Now().UTC()
	_ = s.Store.InsertInvitePopupEvent(ctx, req.Variant, "click", uid, now)
	writeJSON(w, http.StatusOK, map[string]bool{"ok": true})
}

type adminReferralInviteeWire struct {
	UserID            int64   `json:"user_id"`
	Phone             *string `json:"phone"`
	Nickname          *string `json:"nickname"`
	RegisteredAt      string  `json:"registered_at"`
	TotalRechargeYuan float64 `json:"total_recharge_yuan"`
	RechargeCount     int     `json:"recharge_count"`
}

func (s *Server) handleAdminUserReferrals(w http.ResponseWriter, r *http.Request) {
	uidStr := strings.TrimSpace(r.URL.Query().Get("user_id"))
	if uidStr == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid_user_id"})
		return
	}
	var inviterID int64
	if _, err := fmt.Sscan(uidStr, &inviterID); err != nil || inviterID <= 0 {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid_user_id"})
		return
	}
	ctx := r.Context()
	rows, err := s.Store.ListAdminReferralInvitees(ctx, inviterID)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "db_failed"})
		return
	}
	loc, _ := time.LoadLocation("Asia/Shanghai")
	if loc == nil {
		loc = time.UTC
	}
	out := make([]adminReferralInviteeWire, 0, len(rows))
	for _, row := range rows {
		out = append(out, adminReferralInviteeWire{
			UserID:            row.InviteeUserID,
			Phone:             strPtrOrNil(row.InviteePhone),
			Nickname:          strPtrOrNil(row.InviteeNickname),
			RegisteredAt:      row.RegisteredAt.In(loc).Format("2006-01-02 15:04:05"),
			TotalRechargeYuan: float64(row.TotalRechargeFen) / 100.0,
			RechargeCount:     row.RechargeCount,
		})
	}
	writeJSON(w, http.StatusOK, map[string]any{"invitees": out})
}

type invitePopupStatsVariantWire struct {
	Variant     int     `json:"variant"`
	Impressions int64   `json:"impressions"`
	Clicks      int64   `json:"clicks"`
	ClickRate   float64 `json:"click_rate"`
}

type invitePopupStatsWire struct {
	Today   []invitePopupStatsVariantWire `json:"today"`
	Last7d  []invitePopupStatsVariantWire `json:"last_7d"`
	AllTime []invitePopupStatsVariantWire `json:"all_time"`
}

func wireInvitePopupStats(rows []store.InvitePopupStatsRow) []invitePopupStatsVariantWire {
	byVariant := map[int]store.InvitePopupStatsRow{}
	for _, r := range rows {
		byVariant[r.Variant] = r
	}
	out := make([]invitePopupStatsVariantWire, 0, 2)
	for _, v := range []int{1, 2} {
		r := byVariant[v]
		var rate float64
		if r.Impressions > 0 {
			rate = float64(r.Clicks) / float64(r.Impressions)
		}
		out = append(out, invitePopupStatsVariantWire{
			Variant:     v,
			Impressions: r.Impressions,
			Clicks:      r.Clicks,
			ClickRate:   rate,
		})
	}
	return out
}

func (s *Server) handleAdminInvitePopupStats(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	loc, err := time.LoadLocation("Asia/Shanghai")
	if err != nil {
		loc = time.UTC
	}
	now := time.Now().In(loc)
	y, m, d := now.Date()
	todayStart := time.Date(y, m, d, 0, 0, 0, 0, loc).UTC()
	last7Start := todayStart.AddDate(0, 0, -6)

	todayRows, err := s.Store.InvitePopupStatsSince(ctx, todayStart)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "db_failed"})
		return
	}
	last7Rows, err := s.Store.InvitePopupStatsSince(ctx, last7Start)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "db_failed"})
		return
	}
	allRows, err := s.Store.InvitePopupStatsAll(ctx)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "db_failed"})
		return
	}
	writeJSON(w, http.StatusOK, invitePopupStatsWire{
		Today:   wireInvitePopupStats(todayRows),
		Last7d:  wireInvitePopupStats(last7Rows),
		AllTime: wireInvitePopupStats(allRows),
	})
}
