-- 应用市场好评截图活动：每用户一行，记录审核状态（未上传不建行）。

CREATE TABLE IF NOT EXISTS app_store_review_campaign (
  user_id BIGINT PRIMARY KEY,
  status VARCHAR(16) NOT NULL COMMENT 'reviewing | approved | rejected',
  reject_reason VARCHAR(512) NULL,
  screenshot_filename VARCHAR(128) NULL,
  granted_at DATETIME(3) NULL,
  created_at DATETIME(3) NOT NULL,
  updated_at DATETIME(3) NOT NULL,
  CONSTRAINT fk_asrc_user FOREIGN KEY (user_id) REFERENCES users (id) ON DELETE CASCADE
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;
