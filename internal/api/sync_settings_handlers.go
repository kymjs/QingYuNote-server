package api

import (
	"database/sql"
	"net/http"
	"strings"

	"github.com/kymjs/noteapi/internal/store"
	"github.com/kymjs/noteapi/internal/usersecret"
)

type syncSettingsReq struct {
	SyncProvider string `json:"sync_provider"`
	Enabled      bool   `json:"enabled"`
	BaseURL      string `json:"base_url"`
	NotesDir     string `json:"notes_dir"`
	UsernameEnc  string `json:"username_enc"`
	PasswordEnc  string `json:"password_enc"`
}

func (s *Server) handlePutSyncSettings(w http.ResponseWriter, r *http.Request, uid int64) {
	var req syncSettingsReq
	if err := readJSON(r, &req); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid_body"})
		return
	}
	provider := strings.TrimSpace(req.SyncProvider)
	switch provider {
	case "qingyu_cloud", "webdav":
	default:
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid_sync_provider"})
		return
	}

	row := store.UserSyncSettingsRow{
		UserID:       uid,
		SyncProvider: provider,
		Enabled:      req.Enabled,
	}

	if provider == "qingyu_cloud" {
		row.BaseURL = sql.NullString{}
		row.Username = sql.NullString{}
		row.Password = sql.NullString{}
		row.NotesDir = sql.NullString{}
	} else {
		row.BaseURL = sql.NullString{String: strings.TrimSpace(req.BaseURL), Valid: true}
		row.NotesDir = sql.NullString{String: strings.TrimSpace(req.NotesDir), Valid: true}
		if req.Enabled {
			username, err := usersecret.DecryptWebDAVSyncField(req.UsernameEnc, uid)
			if err != nil {
				writeJSON(w, http.StatusBadRequest, map[string]string{"error": "username_decrypt_failed"})
				return
			}
			password, err := usersecret.DecryptWebDAVSyncField(req.PasswordEnc, uid)
			if err != nil {
				writeJSON(w, http.StatusBadRequest, map[string]string{"error": "password_decrypt_failed"})
				return
			}
			row.Username = sql.NullString{String: username, Valid: username != ""}
			row.Password = sql.NullString{String: password, Valid: password != ""}
		} else {
			row.Username = sql.NullString{}
			row.Password = sql.NullString{}
		}
	}

	if err := s.Store.UpsertUserSyncSettings(r.Context(), row); err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "db_failed"})
		return
	}
	writeJSON(w, http.StatusOK, map[string]bool{"ok": true})
}
