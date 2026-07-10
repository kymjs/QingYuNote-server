package release

import (
	"testing"
	"time"
)

func TestNormalizePlatform(t *testing.T) {
	cases := map[string]string{
		"android":  "android",
		"Android":  "android",
		"ios":      "ios",
		"iOS":      "ios",
		"ipad":     "ios",
		"iPad":     "ios",
		"iphone":   "ios",
		"harmony":  "harmony",
		"windows":  "windows",
		"linux":    "linux",
		"macos":    "macos",
		"mac":      "macos",
		"web":      "",
		"":         "",
	}
	for in, want := range cases {
		if got := NormalizePlatform(in); got != want {
			t.Fatalf("NormalizePlatform(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestManifestURLForPlatform(t *testing.T) {
	if manifestURLForPlatform("ios") != cdnManifestBase+"/ios.json" {
		t.Fatal("ios url mismatch")
	}
	if manifestURLForPlatform("macos") != cdnManifestBase+"/mac.json" {
		t.Fatal("macos url mismatch")
	}
}

func TestShouldSkipCDNFetchWithinTTL(t *testing.T) {
	p := &Provider{cache: make(map[string]cachedManifest)}
	p.cache["android"] = cachedManifest{
		info:          GateInfo{MinVersion: "1.0.0"},
		fetchedAt:     time.Now(),
		lastAttemptAt: time.Now(),
	}
	if !p.shouldSkipCDNFetch("android") {
		t.Fatal("expected skip within 24h TTL")
	}
	p.cache["android"] = cachedManifest{
		info:          GateInfo{MinVersion: "1.0.0"},
		fetchedAt:     time.Now().Add(-25 * time.Hour),
		lastAttemptAt: time.Now().Add(-25 * time.Hour),
	}
	if p.shouldSkipCDNFetch("android") {
		t.Fatal("expected refresh after 24h TTL")
	}
}
