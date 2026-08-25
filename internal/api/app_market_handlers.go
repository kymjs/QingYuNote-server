package api

import (
	"context"
	"fmt"
	"log"
	"net/http"
	"strings"
	"time"

	"github.com/kymjs/noteapi/internal/appmarket"
	"github.com/kymjs/noteapi/internal/store"
)

const appMarketFetchLockTTL = 10 * time.Minute

type appMarketVersionReportReq struct {
	Platform string `json:"platform"`
	Version  string `json:"version"`
}

func (s *Server) handleAppMarketVersionReport(w http.ResponseWriter, r *http.Request) {
	if s.appMarketReportGuard == nil || !s.appMarketReportGuard.allow(clientIP(r), time.Now().UTC()) {
		writeTooManyRequests(w)
		return
	}
	var req appMarketVersionReportReq
	if err := readJSON(r, &req); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid_body"})
		return
	}
	platform := strings.TrimSpace(req.Platform)
	version := strings.TrimSpace(req.Version)
	if (platform != "harmony" && platform != "ios") || !store.IsSemanticVersion(version) {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid_app_market_report"})
		return
	}
	claimed, err := s.Store.TryClaimAppMarketVersionFetch(r.Context(), platform, version, time.Now().UTC(), appMarketFetchLockTTL)
	if err != nil {
		log.Printf("app market version claim platform=%s: %v", platform, err)
		writeJSON(w, http.StatusServiceUnavailable, map[string]bool{"accepted": false})
		return
	}
	if claimed {
		go s.refreshAppMarketVersion(platform)
	}
	writeJSON(w, http.StatusAccepted, map[string]bool{"accepted": true})
}

func (s *Server) refreshAppMarketVersion(platform string) {
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()
	defer func() {
		if err := s.Store.ReleaseAppMarketVersionFetch(context.Background(), platform, time.Now().UTC()); err != nil {
			log.Printf("app market version unlock platform=%s: %v", platform, err)
		}
	}()
	var latest string
	var err error
	switch platform {
	case "harmony":
		latest, err = appmarket.FetchHarmonyLatestVersion(ctx)
	case "ios":
		latest, err = appmarket.FetchIOSLatestVersion(ctx)
	default:
		err = fmt.Errorf("unsupported_platform")
	}
	if err != nil {
		log.Printf("app market version fetch platform=%s: %v", platform, err)
		return
	}
	if err := s.Store.UpdateAppMarketVersion(ctx, platform, latest, time.Now().UTC()); err != nil {
		log.Printf("app market version persist platform=%s version=%s: %v", platform, latest, err)
		return
	}
	if err := appmarket.PublishVersion(ctx, s.Cfg, platform, latest); err != nil {
		log.Printf("app market version publish platform=%s version=%s: %v", platform, latest, err)
		return
	}
}
