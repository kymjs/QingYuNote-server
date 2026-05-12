package api

import (
	"encoding/json"
	"log"
	"net/http"
	"strings"
)

// handleAppleAppSiteAssociation 供 iOS / 微信 Universal Link 校验：须由 https://note.kymjs.com 反代到本服务
//（或同域部署）。依赖 APPLE_APP_SITE_ASSOCIATION_TEAM_ID（Apple 开发者 10 位 Team ID）。
func (s *Server) handleAppleAppSiteAssociation(w http.ResponseWriter, r *http.Request) {
	team := strings.TrimSpace(s.Cfg.AppleAppSiteAssociationTeamID)
	bundle := strings.TrimSpace(s.Cfg.AppleIAPBundleID)
	if team == "" || bundle == "" {
		http.Error(w, `{"error":"apple_app_site_association_not_configured"}`, http.StatusServiceUnavailable)
		return
	}
	appID := team + "." + bundle
	body := map[string]any{
		"applinks": map[string]any{
			"apps": []any{},
			"details": []any{
				map[string]any{
					"appID": appID,
					"paths": []string{"/wx/login", "/wx/login/*"},
				},
			},
		},
	}
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(http.StatusOK)
	if err := json.NewEncoder(w).Encode(body); err != nil {
		log.Printf("apple-app-site-association: encode: %v", err)
	}
}

func (s *Server) handleWXUniversalLinkLanding(w http.ResponseWriter, _ *http.Request) {
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write([]byte("<!DOCTYPE html><html lang=\"zh-Hans\"><head><meta charset=\"utf-8\"><meta name=\"viewport\" content=\"width=device-width\">" +
		"<title>轻羽云笔记</title></head><body><p>请返回 <strong>轻羽云笔记</strong> App 继续操作。</p></body></html>"))
}

func (s *Server) handleWXLoginNoTrailingSlash(w http.ResponseWriter, r *http.Request) {
	http.Redirect(w, r, "/wx/login/", http.StatusFound)
}
