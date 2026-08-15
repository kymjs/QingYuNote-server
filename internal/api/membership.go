package api

import (
	"database/sql"
	"errors"
	"net/http"
	"time"

	"github.com/kymjs/noteapi/internal/subscription"
)

// requireActiveMembership 校验当前用户是否为有效会员（active / lifetime）。
// 未开通或已过期时写入 403 subscription_required 并返回 false。
func (s *Server) requireActiveMembership(w http.ResponseWriter, r *http.Request, uid int64) bool {
	ctx := r.Context()
	sub, err := s.Store.GetSubscription(ctx, uid)
	if err != nil {
		if !errors.Is(err, sql.ErrNoRows) {
			writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "db_failed"})
			return false
		}
		sub = nil
	}
	state, _, _ := subscription.RowToAPIState(sub, time.Now().UTC())
	if state != "active" && state != "lifetime" {
		writeJSON(w, http.StatusForbidden, map[string]string{"error": "subscription_required"})
		return false
	}
	return true
}
