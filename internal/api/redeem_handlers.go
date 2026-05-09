package api

import (
	"errors"
	"net/http"
	"strings"
	"time"

	"github.com/kymjs/noteapi/internal/redemption"
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
	writeJSON(w, http.StatusOK, redeemResp{PlanID: plan})
}
