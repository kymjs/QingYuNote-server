package api

import (
	"log"
	"net/http"
	"strings"
)

type confirmIdentityRebindReq struct {
	RebindTicket string `json:"rebind_ticket"`
}

// handleConfirmIdentityRebind POST /api/v1/me/rebind/identity/confirm
// 使用 merge 冲突响应中的 rebind_ticket，将第三方身份从其他账号解绑并绑定到当前 JWT 用户。
func (s *Server) handleConfirmIdentityRebind(w http.ResponseWriter, r *http.Request, uid int64) {
	if r.Method != http.MethodPost {
		http.Error(w, `{"error":"method_not_allowed"}`, http.StatusMethodNotAllowed)
		return
	}
	var req confirmIdentityRebindReq
	if err := readJSON(r, &req); err != nil || strings.TrimSpace(req.RebindTicket) == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid_body"})
		return
	}
	prov, sub, surv, err := parseIdentityRebindTicket(s.Cfg.JWTSecret, req.RebindTicket)
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid_rebind_ticket"})
		return
	}
	if surv != uid {
		writeJSON(w, http.StatusForbidden, map[string]string{"error": "rebind_ticket_user_mismatch"})
		return
	}
	if err := s.Store.TransferThirdPartyIdentity(r.Context(), uid, prov, sub); err != nil {
		log.Printf("TransferThirdPartyIdentity uid=%d prov=%s: %v", uid, prov, err)
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "db_failed"})
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"ok": true, "action": "relinked"})
}
