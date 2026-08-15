package api

import (
	"net/http"
	"strings"

	"github.com/kymjs/noteapi/internal/auth"
)

// handleCreateH5PayTicket issues a short-lived scope=h5_pay JWT for external-browser VIP checkout.
// Requires a normal access token (h5_pay tickets cannot mint new tickets).
func (s *Server) handleCreateH5PayTicket(w http.ResponseWriter, r *http.Request, uid int64) {
	raw := strings.TrimSpace(strings.TrimPrefix(r.Header.Get("Authorization"), "Bearer "))
	claims, err := auth.ParseTokenClaims(raw, s.Cfg.JWTSecret)
	if err != nil || claims.Scope == auth.ScopeH5Pay {
		writeJSON(w, http.StatusForbidden, map[string]string{"error": "h5_pay_ticket_requires_access_token"})
		return
	}
	ttl := auth.H5PayTicketTTL
	tok, err := auth.SignH5PayTicket(uid, s.Cfg.JWTSecret, ttl)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "ticket_sign_failed"})
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"ticket":     tok,
		"expires_in": int64(ttl.Seconds()),
	})
}
