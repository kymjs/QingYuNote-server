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
	// Type 可选；H5 也可能把类型写进 content 前缀，未传则多维表格类型列跳过。
	Type string `json:"type"`
}

func feedbackFeishuBody(content string, userID int64, phone string) string {
	return fmt.Sprintf("轻羽云笔记用户反馈：%s用户id:%d，手机号:%s", content, userID, phone)
}

// handleSubmitFeedback 用户反馈 H5 提交：不落库；飞书 Webhook +（可选）多维表格双写。
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
	problemType := strings.TrimSpace(req.Type)
	webhook := strings.TrimSpace(s.Cfg.FeishuFeedbackWebhook)
	bitableOK := s.Cfg.FeishuBitableConfigured()
	if webhook == "" && !bitableOK {
		writeJSON(w, http.StatusServiceUnavailable, map[string]string{"error": "feedback_channel_not_configured"})
		return
	}

	if webhook != "" {
		text := feedbackFeishuBody(content, req.UserID, phone)
		if err := postFeishuText(webhook, text); err != nil {
			log.Printf("warning: feedback feishu webhook failed: %v", err)
			writeJSON(w, http.StatusBadGateway, map[string]string{"error": "feishu_webhook_failed"})
			return
		}
	}
	if bitableOK {
		if err := postFeishuBitableFeedback(s.Cfg, content, req.UserID, phone, problemType); err != nil {
			// 写表失败不阻断已成功的 webhook；仅打日志便于排查。
			log.Printf("warning: feedback feishu bitable failed: %v", err)
		}
	}
	writeJSON(w, http.StatusOK, map[string]any{"ok": true})
}
