-- 裂变运营：邀请关系、待领取邀请、充值返利审计、弹窗曝光点击统计、用户欢迎礼标记

SELECT IF(
  (SELECT COUNT(*) FROM information_schema.COLUMNS
   WHERE TABLE_SCHEMA = DATABASE() AND TABLE_NAME = 'users' AND COLUMN_NAME = 'welcome_bonus_granted_at') = 0,
  'ALTER TABLE users ADD COLUMN welcome_bonus_granted_at DATETIME(3) NULL',
  'SELECT 1'
) INTO @__note_migrate_sql;
PREPARE __note_migrate_stmt FROM @__note_migrate_sql;
EXECUTE __note_migrate_stmt;
DEALLOCATE PREPARE __note_migrate_stmt;

SELECT IF(
  (SELECT COUNT(*) FROM information_schema.COLUMNS
   WHERE TABLE_SCHEMA = DATABASE() AND TABLE_NAME = 'users' AND COLUMN_NAME = 'invite_popup_pending') = 0,
  'ALTER TABLE users ADD COLUMN invite_popup_pending TINYINT(1) NOT NULL DEFAULT 0',
  'SELECT 1'
) INTO @__note_migrate_sql;
PREPARE __note_migrate_stmt FROM @__note_migrate_sql;
EXECUTE __note_migrate_stmt;
DEALLOCATE PREPARE __note_migrate_stmt;

CREATE TABLE IF NOT EXISTS referral_claims (
  id BIGINT PRIMARY KEY AUTO_INCREMENT,
  token CHAR(32) NOT NULL,
  inviter_user_id BIGINT NOT NULL,
  claimed_at DATETIME(3) NOT NULL,
  used_by_user_id BIGINT NULL,
  used_at DATETIME(3) NULL,
  UNIQUE KEY uk_rc_token (token),
  KEY idx_rc_claimed (claimed_at),
  KEY idx_rc_inviter (inviter_user_id),
  CONSTRAINT fk_rc_inviter FOREIGN KEY (inviter_user_id) REFERENCES users (id) ON DELETE CASCADE,
  CONSTRAINT fk_rc_used_by FOREIGN KEY (used_by_user_id) REFERENCES users (id) ON DELETE SET NULL
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;

CREATE TABLE IF NOT EXISTS user_referrals (
  id BIGINT PRIMARY KEY AUTO_INCREMENT,
  inviter_user_id BIGINT NOT NULL,
  invitee_user_id BIGINT NOT NULL,
  created_at DATETIME(3) NOT NULL,
  UNIQUE KEY uk_ur_invitee (invitee_user_id),
  KEY idx_ur_inviter (inviter_user_id),
  CONSTRAINT fk_ur_inviter FOREIGN KEY (inviter_user_id) REFERENCES users (id) ON DELETE CASCADE,
  CONSTRAINT fk_ur_invitee FOREIGN KEY (invitee_user_id) REFERENCES users (id) ON DELETE CASCADE
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;

CREATE TABLE IF NOT EXISTS referral_recharge_rewards (
  id BIGINT PRIMARY KEY AUTO_INCREMENT,
  inviter_user_id BIGINT NOT NULL,
  invitee_user_id BIGINT NOT NULL,
  order_id BIGINT NOT NULL,
  plan_id VARCHAR(32) NOT NULL,
  reward_days INT NOT NULL,
  created_at DATETIME(3) NOT NULL,
  UNIQUE KEY uk_rrr_order (order_id),
  KEY idx_rrr_inviter (inviter_user_id),
  CONSTRAINT fk_rrr_inviter FOREIGN KEY (inviter_user_id) REFERENCES users (id) ON DELETE CASCADE,
  CONSTRAINT fk_rrr_invitee FOREIGN KEY (invitee_user_id) REFERENCES users (id) ON DELETE CASCADE,
  CONSTRAINT fk_rrr_order FOREIGN KEY (order_id) REFERENCES orders (id) ON DELETE CASCADE
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;

CREATE TABLE IF NOT EXISTS invite_popup_events (
  id BIGINT PRIMARY KEY AUTO_INCREMENT,
  variant TINYINT NOT NULL COMMENT '1 | 2',
  event_type VARCHAR(16) NOT NULL COMMENT 'impression | click',
  user_id BIGINT NULL,
  created_at DATETIME(3) NOT NULL,
  KEY idx_ipe_created (created_at),
  KEY idx_ipe_variant_type_created (variant, event_type, created_at),
  CONSTRAINT fk_ipe_user FOREIGN KEY (user_id) REFERENCES users (id) ON DELETE SET NULL
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;
