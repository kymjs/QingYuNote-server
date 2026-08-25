package appmarket

import (
	"encoding/json"
	"testing"
)

func TestFindVersion(t *testing.T) {
	document := map[string]any{
		"layoutData": []any{
			map[string]any{
				"layoutName": "detailappinfocard",
				"dataList":   []any{map[string]any{"version": "1.1.19"}},
			},
		},
	}
	if got := findVersion(document); got != "1.1.19" {
		t.Fatalf("findVersion() = %q, want 1.1.19", got)
	}
}

func TestRewriteConfigVersionPreservesOtherFields(t *testing.T) {
	raw := []byte(`{"name":"qingyu","redemption_version":"old","nested":{"ok":true}}`)
	updated, err := rewriteConfigVersion(raw, "1.2.3")
	if err != nil {
		t.Fatal(err)
	}
	var value map[string]any
	if err := json.Unmarshal(updated, &value); err != nil {
		t.Fatal(err)
	}
	if value["redemption_version"] != "1.0.0-1.2.3" || value["pay_version"] != "1.0.0-1.2.3" {
		t.Fatalf("unexpected version fields: %#v", value)
	}
	if value["name"] != "qingyu" {
		t.Fatalf("other fields lost: %#v", value)
	}
}

func TestRewriteIOSReleaseManifestPreservesOtherFields(t *testing.T) {
	raw := []byte(`{"latestRelease":"1.1.16","latestVersion":"1.1.0","downloadUrl":"https://example.com"}`)
	updated, err := rewriteIOSReleaseManifest(raw, "1.1.17")
	if err != nil {
		t.Fatal(err)
	}
	var value map[string]any
	if err := json.Unmarshal(updated, &value); err != nil {
		t.Fatal(err)
	}
	if value["latestRelease"] != "1.1.17" {
		t.Fatalf("latestRelease = %#v, want 1.1.17", value["latestRelease"])
	}
	if value["latestVersion"] != "1.1.0" || value["downloadUrl"] != "https://example.com" {
		t.Fatalf("other fields changed: %#v", value)
	}
}

func TestParseIOSVersion(t *testing.T) {
	version, err := parseIOSVersion([]byte(`{"resultCount":1,"results":[{"version":"1.1.17"}]}`))
	if err != nil || version != "1.1.17" {
		t.Fatalf("parseIOSVersion() = %q, %v", version, err)
	}
	for _, payload := range [][]byte{
		[]byte(`{"resultCount":0,"results":[]}`),
		[]byte(`{"resultCount":1,"results":[{"version":"1.2.beta"}]}`),
		[]byte(`not-json`),
	} {
		if _, err := parseIOSVersion(payload); err == nil {
			t.Fatalf("expected parse failure for %q", payload)
		}
	}
}
