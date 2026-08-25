package store

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strconv"
	"strings"
	"time"
)

type AppMarketVersionState struct {
	Platform      string
	LatestVersion string
}

func CompareSemanticVersions(a, b string) int {
	parse := func(raw string) []int {
		core := strings.TrimSpace(strings.Split(raw, "+")[0])
		if core == "" {
			return nil
		}
		parts := strings.Split(core, ".")
		out := make([]int, len(parts))
		for i, part := range parts {
			n, err := strconv.Atoi(strings.TrimSpace(part))
			if err != nil || n < 0 {
				return nil
			}
			out[i] = n
		}
		return out
	}
	pa, pb := parse(a), parse(b)
	max := len(pa)
	if len(pb) > max {
		max = len(pb)
	}
	for i := 0; i < max; i++ {
		var va, vb int
		if i < len(pa) {
			va = pa[i]
		}
		if i < len(pb) {
			vb = pb[i]
		}
		if va < vb {
			return -1
		}
		if va > vb {
			return 1
		}
	}
	return 0
}

func IsSemanticVersion(v string) bool {
	v = strings.TrimSpace(v)
	if v == "" || len(v) > 32 {
		return false
	}
	parts := strings.Split(strings.Split(v, "+")[0], ".")
	if len(parts) == 0 || len(parts) > 4 {
		return false
	}
	for _, part := range parts {
		if part == "" {
			return false
		}
		n, err := strconv.Atoi(part)
		if err != nil || n < 0 {
			return false
		}
	}
	return true
}

// TryClaimAppMarketVersionFetch 原子领取平台任务锁。锁超时后可接管，防止崩溃遗留锁永久阻塞。
func (s *Store) TryClaimAppMarketVersionFetch(
	ctx context.Context, platform, reportedVersion string, now time.Time, staleAfter time.Duration,
) (bool, error) {
	if !IsSemanticVersion(reportedVersion) {
		return false, errors.New("invalid_reported_version")
	}
	tx, err := s.DB.BeginTx(ctx, &sql.TxOptions{})
	if err != nil {
		return false, err
	}
	defer func() { _ = tx.Rollback() }()

	var current string
	if err := tx.QueryRowContext(ctx,
		`SELECT latest_version FROM app_market_version_states WHERE platform = ? FOR UPDATE`, platform,
	).Scan(&current); err != nil {
		return false, err
	}
	if CompareSemanticVersions(reportedVersion, current) < 0 {
		return false, nil
	}
	cutoff := now.Add(-staleAfter)
	result, err := tx.ExecContext(ctx, `
		UPDATE app_market_version_states
		SET fetch_in_progress = 1, fetch_started_at = ?, updated_at = ?
		WHERE platform = ?
		  AND (fetch_in_progress = 0 OR fetch_started_at IS NULL OR fetch_started_at < ?)`,
		now.UTC(), now.UTC(), platform, cutoff.UTC(),
	)
	if err != nil {
		return false, err
	}
	affected, err := result.RowsAffected()
	if err != nil {
		return false, err
	}
	if affected == 0 {
		return false, tx.Commit()
	}
	return true, tx.Commit()
}

// CompleteAppMarketVersionFetch 仅在配置文件成功发布后推进版本水位。
func (s *Store) CompleteAppMarketVersionFetch(ctx context.Context, platform, latestVersion string, now time.Time) error {
	if !IsSemanticVersion(latestVersion) {
		return fmt.Errorf("invalid_market_version: %q", latestVersion)
	}
	_, err := s.DB.ExecContext(ctx, `
		UPDATE app_market_version_states
		SET latest_version = ?, fetch_in_progress = 0, fetch_started_at = NULL, updated_at = ?
		WHERE platform = ?`,
		latestVersion, now.UTC(), platform,
	)
	return err
}

// UpdateAppMarketVersion 在抓取成功后先持久化市场版本；调用方随后发布 WebDAV 配置并释放任务锁。
func (s *Store) UpdateAppMarketVersion(ctx context.Context, platform, latestVersion string, now time.Time) error {
	if !IsSemanticVersion(latestVersion) {
		return fmt.Errorf("invalid_market_version: %q", latestVersion)
	}
	_, err := s.DB.ExecContext(ctx, `
		UPDATE app_market_version_states
		SET latest_version = ?, updated_at = ?
		WHERE platform = ?`,
		latestVersion, now.UTC(), platform,
	)
	return err
}

func (s *Store) ReleaseAppMarketVersionFetch(ctx context.Context, platform string, now time.Time) error {
	_, err := s.DB.ExecContext(ctx, `
		UPDATE app_market_version_states
		SET fetch_in_progress = 0, fetch_started_at = NULL, updated_at = ?
		WHERE platform = ?`,
		now.UTC(), platform,
	)
	return err
}
