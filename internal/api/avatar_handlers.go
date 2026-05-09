package api

import (
	"bytes"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"log"
	"net/http"
	"strings"
	"time"

	"github.com/kymjs/noteapi/internal/avatarwebdav"
)

const maxAvatarBytes = 5 << 20 // 5 MiB

// avatarWebDAVErrorCode 将 WebDAV Put 错误映射为对外 error 码（不含 NAS 响应原文）。
func avatarWebDAVErrorCode(err error) string {
	var ps *avatarwebdav.PutStatusError
	if errors.As(err, &ps) && ps != nil {
		switch ps.StatusCode {
		case http.StatusUnauthorized:
			return "avatar_webdav_unauthorized"
		case http.StatusForbidden:
			return "avatar_webdav_forbidden"
		case http.StatusNotFound:
			return "avatar_webdav_not_found"
		case http.StatusMethodNotAllowed:
			return "avatar_webdav_method_not_allowed"
		case http.StatusInsufficientStorage:
			return "avatar_webdav_insufficient_storage"
		case http.StatusBadGateway, http.StatusServiceUnavailable, http.StatusGatewayTimeout:
			return "avatar_webdav_upstream_error"
		default:
			return "avatar_upload_failed"
		}
	}
	return "avatar_upload_failed"
}

func sniffImage(b []byte) (contentType string, ext string) {
	if len(b) >= 3 && b[0] == 0xFF && b[1] == 0xD8 && b[2] == 0xFF {
		return "image/jpeg", ".jpg"
	}
	if len(b) >= 8 && b[0] == 0x89 && b[1] == 'P' && b[2] == 'N' && b[3] == 'G' {
		return "image/png", ".png"
	}
	if len(b) >= 6 && b[0] == 'G' && b[1] == 'I' && b[2] == 'F' {
		return "image/gif", ".gif"
	}
	if len(b) >= 12 && b[0] == 'R' && b[1] == 'I' && b[2] == 'F' && b[3] == 'F' && string(b[8:12]) == "WEBP" {
		return "image/webp", ".webp"
	}
	return "", ""
}

func randomAvatarSuffix() string {
	buf := make([]byte, 16)
	if _, err := rand.Read(buf); err != nil {
		return fmt.Sprintf("%d", time.Now().UnixNano())
	}
	return hex.EncodeToString(buf)
}

func (s *Server) handlePostAvatar(w http.ResponseWriter, r *http.Request, uid int64) {
	if r.Method != http.MethodPost {
		http.Error(w, `{"error":"method_not_allowed"}`, http.StatusMethodNotAllowed)
		return
	}
	if !s.Cfg.AvatarWebDAVConfigured() {
		writeJSON(w, http.StatusServiceUnavailable, map[string]string{"error": "avatar_webdav_not_configured"})
		return
	}
	if err := r.ParseMultipartForm(maxAvatarBytes + (1 << 20)); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid_multipart"})
		return
	}
	f, _, err := r.FormFile("file")
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "file_required"})
		return
	}
	defer f.Close()
	data, err := io.ReadAll(io.LimitReader(f, maxAvatarBytes+1))
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "read_failed"})
		return
	}
	if len(data) == 0 {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "empty_file"})
		return
	}
	if len(data) > maxAvatarBytes {
		writeJSON(w, http.StatusRequestEntityTooLarge, map[string]string{"error": "file_too_large"})
		return
	}
	ct, ext := sniffImage(data)
	if ct == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid_image_type"})
		return
	}
	fname := fmt.Sprintf("%d_%s%s", uid, randomAvatarSuffix(), ext)
	ctx := r.Context()
	pubBase := strings.TrimRight(strings.TrimSpace(s.Cfg.AvatarPublicBaseURL), "/")
	publicURL := pubBase + "/" + fname

	err = avatarwebdav.PutFile(ctx, s.Cfg.AvatarWebDAVBaseURL, s.Cfg.AvatarWebDAVUsername, s.Cfg.AvatarWebDAVPassword,
		fname, bytes.NewReader(data), ct, int64(len(data)))
	if err != nil {
		log.Printf("avatar webdav put uid=%d: %v", uid, err)
		writeJSON(w, http.StatusBadGateway, map[string]string{"error": avatarWebDAVErrorCode(err)})
		return
	}
	if err := s.Store.PatchUserProfile(ctx, uid, nil, &publicURL, nil, nil); err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "db_failed"})
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"avatar_url": publicURL})
}
