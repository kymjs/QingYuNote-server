package api

import (
	"database/sql"
	"errors"
	"log"
	"net/http"
	"strings"
	"time"

	"github.com/kymjs/noteapi/internal/redemption"
	"github.com/kymjs/noteapi/internal/store"
)

type redeemReq struct {
	Code string `json:"code"`
}

type redeemResp struct {
	PlanID string `json:"plan_id"`
}

func (s *Server) handleRedeem(w http.ResponseWriter, r *http.Request, uid int64) {
	if r.Method != http.MethodPost {
		http.Error(w, `{"error":"method_not_allowed"}`, http.StatusMethodNotAllowed)
		return
	}
	var req redeemReq
	if err := readJSON(r, &req); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid_body"})
		return
	}
	raw := strings.TrimSpace(req.Code)
	if raw == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid_body"})
		return
	}
	norm := redemption.NormalizeCode(raw)
	if len(norm) < 8 || len(norm) > 128 {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid_code"})
		return
	}
	hash := redemption.HashNormalized(norm)
	ctx := r.Context()
	plan, err := redemption.RedeemAtomically(s.Store, ctx, uid, hash, time.Now().UTC())
	if err != nil {
		if errors.Is(err, redemption.ErrInvalid) {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "redemption_invalid"})
			return
		}
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "db_failed"})
		return
	}
	if err := s.Store.InsertMembershipRechargeRecord(ctx, &store.MembershipRechargeRecordParams{
		UserID:             uid,
		Channel:            "redeem",
		RedemptionCodeHash: sql.NullString{String: hash, Valid: true},
		PlanID:             plan,
	}); err != nil {
		log.Printf("membership recharge audit (redeem): %v", err)
	}
	s.qingyuGuard.invalidate(uid)
	writeJSON(w, http.StatusOK, redeemResp{PlanID: plan})
}
