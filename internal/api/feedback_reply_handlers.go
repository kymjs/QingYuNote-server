package api

import (
	"database/sql"
	"errors"
	"log"
	"net/http"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/go-sql-driver/mysql"
)

type feedbackReplyCreateReq struct {
	UserID          int64  `json:"user_id"`
	FeedbackContent string `json:"feedback_content"`
	FeedbackType    string `json:"feedback_type"`
	OfficialReply   string `json:"official_reply"`
}

const (
	feedbackReplyContentMaxRunes = 2000
	feedbackReplyTypeMaxRunes    = 64
	feedbackReplyBodyMaxRunes    = 5000
)

// handleCreateFeedbackReply 官方回复写入（无鉴权）；全量落库，客户端只读最新一条。
func (s *Server) handleCreateFeedbackReply(w http.ResponseWriter, r *http.Request) {
	var req feedbackReplyCreateReq
	if err := readJSON(r, &req); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid_body"})
		return
	}
	if req.UserID <= 0 {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid_user_id"})
		return
	}
	content := strings.TrimSpace(req.FeedbackContent)
	if content == "" || utf8.RuneCountInString(content) > feedbackReplyContentMaxRunes {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid_feedback_content"})
		return
	}
	typ := strings.TrimSpace(req.FeedbackType)
	if utf8.RuneCountInString(typ) > feedbackReplyTypeMaxRunes {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid_feedback_type"})
		return
	}
	reply := strings.TrimSpace(req.OfficialReply)
	if reply == "" || utf8.RuneCountInString(reply) > feedbackReplyBodyMaxRunes {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid_official_reply"})
		return
	}
	id, err := s.Store.InsertFeedbackReply(r.Context(), req.UserID, content, typ, reply, time.Now().UTC())
	if err != nil {
		var me *mysql.MySQLError
		if errors.As(err, &me) && me.Number == 1452 {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "user_not_found"})
			return
		}
		log.Printf("error: insert feedback reply user_id=%d: %v", req.UserID, err)
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "db_error"})
		return
	}
	log.Printf("info: feedback reply created id=%d user_id=%d", id, req.UserID)
	writeJSON(w, http.StatusOK, map[string]any{"ok": true, "id": id})
}

// handleGetMyFeedbackReply 当前登录用户最新一条官方回复；无数据时 reply=null。
func (s *Server) handleGetMyFeedbackReply(w http.ResponseWriter, r *http.Request, userID int64) {
	row, err := s.Store.GetLatestFeedbackReplyByUserID(r.Context(), userID)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			writeJSON(w, http.StatusOK, map[string]any{"reply": nil})
			return
		}
		log.Printf("error: get latest feedback reply user_id=%d: %v", userID, err)
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "db_error"})
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"reply": map[string]any{
			"id":               row.ID,
			"feedback_content": row.FeedbackContent,
			"feedback_type":    row.FeedbackType,
			"official_reply":   row.OfficialReply,
			"created_at":       row.CreatedAt.UTC().Format(time.RFC3339Nano),
		},
	})
}
