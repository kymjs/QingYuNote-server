package api

import (
	"net/http"
	"strings"

	"github.com/kymjs/noteapi/internal/release"
)

func withMinAppVersion(manifests *release.Provider) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if r.Method == http.MethodOptions {
				next.ServeHTTP(w, r)
				return
			}
			if !strings.HasPrefix(r.URL.Path, "/api/v1/") || shouldSkipAppVersionCheck(r.URL.Path) {
				next.ServeHTTP(w, r)
				return
			}
			platform, _, clientVer := extractDeviceInfo(r)
			gate, ok := manifests.GateInfoForPlatform(platform)
			if !ok {
				next.ServeHTTP(w, r)
				return
			}
			minVer := strings.TrimSpace(gate.MinVersion)
			if minVer == "" {
				next.ServeHTTP(w, r)
				return
			}
			if clientVer == "" || compareSemanticVersions(clientVer, minVer) < 0 {
				payload := map[string]string{
					"error":       "upgrade_required",
					"min_version": minVer,
				}
				if gate.DownloadURL != "" {
					payload["download_url"] = gate.DownloadURL
				}
				writeJSON(w, http.StatusUpgradeRequired, payload)
				return
			}
			next.ServeHTTP(w, r)
		})
	}
}

func shouldSkipAppVersionCheck(path string) bool {
	return strings.HasPrefix(path, "/api/v1/webhooks/") ||
		strings.HasPrefix(path, "/api/v1/admin/")
}
