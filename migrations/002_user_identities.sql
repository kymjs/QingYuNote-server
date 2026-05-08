-- 统一身份：同一 users 行可绑定微信 / 华为 / Apple 等多个 provider+subject

CREATE TABLE IF NOT EXISTS user_identities (
  id BIGINT PRIMARY KEY AUTO_INCREMENT,
  user_id BIGINT NOT NULL,
  provider VARCHAR(16) NOT NULL,
  subject VARCHAR(191) NOT NULL,
  created_at DATETIME(3) NOT NULL,
  UNIQUE KEY uk_identity_provider_subject (provider, subject),
  KEY idx_identity_user (user_id),
  CONSTRAINT fk_identity_user FOREIGN KEY (user_id) REFERENCES users (id) ON DELETE CASCADE
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;

-- 历史数据：已有微信用户写入 identities
INSERT IGNORE INTO user_identities (user_id, provider, subject, created_at)
SELECT id, 'wechat', wechat_openid, created_at FROM users WHERE wechat_openid IS NOT NULL AND TRIM(wechat_openid) <> '';
