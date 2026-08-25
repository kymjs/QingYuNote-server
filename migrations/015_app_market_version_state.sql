-- 应用市场版本水位与跨实例抓取锁。
CREATE TABLE IF NOT EXISTS app_market_version_states (
  platform VARCHAR(16) NOT NULL,
  latest_version VARCHAR(32) NOT NULL DEFAULT '1.1.17',
  fetch_in_progress TINYINT(1) NOT NULL DEFAULT 0,
  fetch_started_at DATETIME(3) NULL,
  updated_at DATETIME(3) NOT NULL,
  PRIMARY KEY (platform)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;

INSERT INTO app_market_version_states
  (platform, latest_version, fetch_in_progress, fetch_started_at, updated_at)
VALUES
  ('harmony', '1.1.17', 0, NULL, UTC_TIMESTAMP(3)),
  ('ios', '1.1.17', 0, NULL, UTC_TIMESTAMP(3))
ON DUPLICATE KEY UPDATE platform = VALUES(platform);
