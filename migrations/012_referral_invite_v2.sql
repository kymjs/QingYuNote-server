-- 邀请活动增强：被邀请人手机号、状态、历史注册手机号、身份历史

SELECT IF(
  (SELECT COUNT(*) FROM information_schema.COLUMNS
   WHERE TABLE_SCHEMA = DATABASE() AND TABLE_NAME = 'referral_claims' AND COLUMN_NAME = 'invitee_phone') = 0,
  'ALTER TABLE referral_claims ADD COLUMN invitee_phone VARCHAR(32) NULL',
  'SELECT 1'
) INTO @__note_migrate_sql;
PREPARE __note_migrate_stmt FROM @__note_migrate_sql;
EXECUTE __note_migrate_stmt;
DEALLOCATE PREPARE __note_migrate_stmt;

SELECT IF(
  (SELECT COUNT(*) FROM information_schema.COLUMNS
   WHERE TABLE_SCHEMA = DATABASE() AND TABLE_NAME = 'referral_claims' AND COLUMN_NAME = 'status') = 0,
  'ALTER TABLE referral_claims ADD COLUMN status VARCHAR(16) NOT NULL DEFAULT ''pending''',
  'SELECT 1'
) INTO @__note_migrate_sql;
PREPARE __note_migrate_stmt FROM @__note_migrate_sql;
EXECUTE __note_migrate_stmt;
DEALLOCATE PREPARE __note_migrate_stmt;

CREATE TABLE IF NOT EXISTS registered_phone_history (
  phone VARCHAR(32) PRIMARY KEY,
  first_registered_at DATETIME(3) NOT NULL
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;

CREATE TABLE IF NOT EXISTS registered_identity_history (
  id BIGINT PRIMARY KEY AUTO_INCREMENT,
  provider VARCHAR(16) NOT NULL,
  subject VARCHAR(191) NOT NULL,
  first_registered_at DATETIME(3) NOT NULL,
  UNIQUE KEY uk_rih_provider_subject (provider, subject)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;

-- 将现有用户手机号写入历史表（含已注销前未迁移的数据兜底）
INSERT IGNORE INTO registered_phone_history (phone, first_registered_at)
SELECT phone, created_at FROM users WHERE phone IS NOT NULL AND TRIM(phone) <> '';

INSERT IGNORE INTO registered_identity_history (provider, subject, first_registered_at)
SELECT provider, subject, created_at FROM user_identities;
