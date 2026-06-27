-- 用户云同步方案（轻羽云 / 自建 WebDAV），供管理后台「云服务」列展示

CREATE TABLE IF NOT EXISTS user_sync_settings (
  user_id BIGINT PRIMARY KEY,
  sync_provider VARCHAR(32) NOT NULL DEFAULT '',
  enabled TINYINT(1) NOT NULL DEFAULT 0,
  base_url VARCHAR(512) NULL,
  username VARCHAR(191) NULL,
  password VARCHAR(512) NULL,
  notes_dir VARCHAR(512) NULL,
  updated_at DATETIME(3) NOT NULL,
  CONSTRAINT fk_user_sync_settings_user FOREIGN KEY (user_id) REFERENCES users (id) ON DELETE CASCADE
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;
