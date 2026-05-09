-- 兑换码：仅存 SHA256(规范化明文)，明文仅出现在签发脚本推送到飞书的渠道中。

CREATE TABLE IF NOT EXISTS redemption_codes (
  id BIGINT PRIMARY KEY AUTO_INCREMENT,
  code_hash CHAR(64) NOT NULL COMMENT 'SHA256 hex（规范化后的明文）',
  plan_id VARCHAR(32) NOT NULL COMMENT 'monthly | half_year | yearly | lifetime_vip',
  created_at DATETIME(3) NOT NULL,
  redeemed_at DATETIME(3) NULL,
  redeemed_by_user_id BIGINT NULL,
  UNIQUE KEY uk_redemption_code_hash (code_hash),
  KEY idx_rc_redeemed (redeemed_at),
  KEY idx_rc_user (redeemed_by_user_id),
  CONSTRAINT fk_rc_user FOREIGN KEY (redeemed_by_user_id) REFERENCES users (id) ON DELETE SET NULL
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;
