package api

import (
	"database/sql"
	"errors"
	"net/http"
	"time"
)

type vipPageViewReq struct {
	UserID int64 `json:"user_id"`
}

type vipPageViewAdminItem struct {
	UserID       int64  `json:"user_id"`
	ViewCount    int64  `json:"view_count"`
	LastViewedAt string `json:"last_viewed_at"`
}

type vipPageViewsAdminWire struct {
	Items []vipPageViewAdminItem `json:"items"`
}

// handleVipPageView 充值页打开埋点（H5 自报 user_id，无需登录）。
func (s *Server) handleVipPageView(w http.ResponseWriter, r *http.Request) {
	var req vipPageViewReq
	if err := readJSON(r, &req); err != nil || req.UserID <= 0 {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid_user_id"})
		return
	}
	ctx := r.Context()
	if _, err := s.Store.GetUserByID(ctx, req.UserID); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			writeJSON(w, http.StatusNotFound, map[string]string{"error": "user_not_found"})
			return
		}
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "db_failed"})
		return
	}
	if err := s.Store.UpsertVipPageView(ctx, req.UserID, time.Now().UTC()); err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "db_failed"})
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"ok": true})
}

func (s *Server) handleAdminVipPageViews(w http.ResponseWriter, r *http.Request) {
	rows, err := s.Store.ListVipPageViews(r.Context())
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "db_failed"})
		return
	}
	items := make([]vipPageViewAdminItem, 0, len(rows))
	for _, row := range rows {
		items = append(items, vipPageViewAdminItem{
			UserID:       row.UserID,
			ViewCount:    row.ViewCount,
			LastViewedAt: row.LastViewedAt.UTC().Format(time.RFC3339Nano),
		})
	}
	writeJSON(w, http.StatusOK, vipPageViewsAdminWire{Items: items})
}
