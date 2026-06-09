package api

import (
	"net/http"
	"strings"
)

type aiTagReasoningWire struct {
	Effort string `json:"effort"`
}

type aiTagConfigWire struct {
	TokenPlanKey string                `json:"token_plan_key"`
	Model        string                `json:"model"`
	BaseURL      string                `json:"base_url"`
	Reasoning    *aiTagReasoningWire   `json:"reasoning,omitempty"`
}

func (s *Server) handleGetAiTagConfig(w http.ResponseWriter, r *http.Request, _ int64) {
	if r.Method != http.MethodGet {
		http.Error(w, `{"error":"method_not_allowed"}`, http.StatusMethodNotAllowed)
		return
	}
	if !s.Cfg.MiniMaxConfigured() {
		writeJSON(w, http.StatusServiceUnavailable, map[string]string{"error": "minimax_not_configured"})
		return
	}
	wire := aiTagConfigWire{
		TokenPlanKey: s.Cfg.MiniMaxTokenPlanKey,
		Model:        s.Cfg.MiniMaxModel,
		BaseURL:      strings.TrimRight(s.Cfg.MiniMaxBaseURL, "/"),
	}
	if effort := s.Cfg.MiniMaxReasoningEffortForModel(); effort != "" {
		wire.Reasoning = &aiTagReasoningWire{Effort: effort}
	}
	writeJSON(w, http.StatusOK, wire)
}
