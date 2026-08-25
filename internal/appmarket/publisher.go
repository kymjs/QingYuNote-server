package appmarket

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/kymjs/noteapi/internal/avatarwebdav"
	"github.com/kymjs/noteapi/internal/config"
)

const maxConfigBytes = 1 << 20

func configFilename(platform string) (string, error) {
	switch platform {
	case "harmony":
		return "config_hw.json", nil
	case "ios":
		return "config.json", nil
	default:
		return "", fmt.Errorf("unsupported_platform: %q", platform)
	}
}

// PublishVersion 更新指定平台的会员配置，其他 JSON 字段保持不变。
func PublishVersion(ctx context.Context, cfg *config.Config, platform, version string) error {
	if !cfg.AppMarketWebDAVConfigured() {
		return fmt.Errorf("app_market_webdav_not_configured")
	}
	filename, err := configFilename(platform)
	if err != nil {
		return err
	}
	raw, err := avatarwebdav.GetFile(ctx, cfg.AppMarketWebDAVBaseURL, cfg.AvatarWebDAVUsername,
		cfg.AvatarWebDAVPassword, filename, maxConfigBytes)
	if err != nil {
		return fmt.Errorf("get_config: %w", err)
	}
	updated, err := rewriteConfigVersion(raw, version)
	if err != nil {
		return err
	}
	if err := avatarwebdav.PutFile(ctx, cfg.AppMarketWebDAVBaseURL, cfg.AvatarWebDAVUsername,
		cfg.AvatarWebDAVPassword, filename, bytes.NewReader(updated), "application/json; charset=utf-8", int64(len(updated))); err != nil {
		return fmt.Errorf("put_config: %w", err)
	}
	if platform == "ios" {
		if err := publishIOSReleaseManifest(ctx, cfg, version); err != nil {
			return err
		}
	}
	return nil
}

// publishIOSReleaseManifest 与 iOS 会员配置同步推进 CDN 更新清单的 latestRelease。
func publishIOSReleaseManifest(ctx context.Context, cfg *config.Config, version string) error {
	raw, err := avatarwebdav.GetFile(ctx, cfg.AppMarketWebDAVBaseURL, cfg.AvatarWebDAVUsername,
		cfg.AvatarWebDAVPassword, "ios.json", maxConfigBytes)
	if err != nil {
		return fmt.Errorf("get_ios_release_manifest: %w", err)
	}
	updated, err := rewriteIOSReleaseManifest(raw, version)
	if err != nil {
		return err
	}
	if err := avatarwebdav.PutFile(ctx, cfg.AppMarketWebDAVBaseURL, cfg.AvatarWebDAVUsername,
		cfg.AvatarWebDAVPassword, "ios.json", bytes.NewReader(updated), "application/json; charset=utf-8", int64(len(updated))); err != nil {
		return fmt.Errorf("put_ios_release_manifest: %w", err)
	}
	return nil
}

func rewriteConfigVersion(raw []byte, version string) ([]byte, error) {
	var document map[string]json.RawMessage
	if err := json.Unmarshal(raw, &document); err != nil {
		return nil, fmt.Errorf("parse_config: %w", err)
	}
	if document == nil {
		return nil, fmt.Errorf("parse_config: empty_object")
	}
	value, err := json.Marshal("1.0.0-" + strings.TrimSpace(version))
	if err != nil {
		return nil, err
	}
	document["redemption_version"] = value
	document["pay_version"] = value
	updated, err := json.Marshal(document)
	if err != nil {
		return nil, fmt.Errorf("encode_config: %w", err)
	}
	return updated, nil
}

func rewriteIOSReleaseManifest(raw []byte, version string) ([]byte, error) {
	var document map[string]json.RawMessage
	if err := json.Unmarshal(raw, &document); err != nil {
		return nil, fmt.Errorf("parse_ios_release_manifest: %w", err)
	}
	if document == nil {
		return nil, fmt.Errorf("parse_ios_release_manifest: empty_object")
	}
	value, err := json.Marshal(strings.TrimSpace(version))
	if err != nil {
		return nil, err
	}
	document["latestRelease"] = value
	updated, err := json.Marshal(document)
	if err != nil {
		return nil, fmt.Errorf("encode_ios_release_manifest: %w", err)
	}
	return updated, nil
}
