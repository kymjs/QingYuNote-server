package api

import (
	"fmt"
	"log"
	"net/http"
	"strings"
	"unicode/utf8"
)

type feedbackSubmitReq struct {
	Content string `json:"content"`
	UserID  int64  `json:"user_id"`
	Phone   string `json:"phone"`
}

func feedbackFeishuBody(content string, userID int64, phone string) string {
	return fmt.Sprintf("轻羽云笔记用户反馈：%s用户id:%d，手机号:%s", content, userID, phone)
}

// handleSubmitFeedback 用户反馈 H5 提交：不落库，直接转发飞书 Webhook。
func (s *Server) handleSubmitFeedback(w http.ResponseWriter, r *http.Request) {
	var req feedbackSubmitReq
	if err := readJSON(r, &req); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid_body"})
		return
	}
	content := strings.TrimSpace(req.Content)
	if content == "" || utf8.RuneCountInString(content) > 500 {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid_content"})
		return
	}
	if req.UserID <= 0 {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid_user_id"})
		return
	}
	phone := strings.TrimSpace(req.Phone)
	webhook := strings.TrimSpace(s.Cfg.FeishuFeedbackWebhook)
	if webhook == "" {
		writeJSON(w, http.StatusServiceUnavailable, map[string]string{"error": "feedback_webhook_not_configured"})
		return
	}
	text := feedbackFeishuBody(content, req.UserID, phone)
	if err := postFeishuText(webhook, text); err != nil {
		log.Printf("warning: feedback feishu webhook failed: %v", err)
		writeJSON(w, http.StatusBadGateway, map[string]string{"error": "feishu_webhook_failed"})
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"ok": true})
}
