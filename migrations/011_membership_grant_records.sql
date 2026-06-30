-- 会籍赠送审计：欢迎礼、邀请奖励等非支付类会员延长，供管理后台「充值记录」展示来源。

CREATE TABLE IF NOT EXISTS membership_grant_records (
  id BIGINT PRIMARY KEY AUTO_INCREMENT,
  user_id BIGINT NOT NULL,
  source VARCHAR(32) NOT NULL COMMENT 'welcome | invited | invite_friend | invite_recharge',
  grant_days INT NULL,
  grant_months INT NULL,
  related_user_id BIGINT NULL,
  order_id BIGINT NULL,
  created_at DATETIME(3) NOT NULL,
  KEY idx_mgr_user_created (user_id, created_at),
  CONSTRAINT fk_mgr_user FOREIGN KEY (user_id) REFERENCES users (id) ON DELETE CASCADE
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;
