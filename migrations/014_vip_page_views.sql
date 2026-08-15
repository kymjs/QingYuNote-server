-- VIP 充值页打开埋点：按用户聚合查看次数与最后查看时间。

CREATE TABLE IF NOT EXISTS vip_page_views (
  user_id BIGINT PRIMARY KEY,
  view_count BIGINT NOT NULL DEFAULT 0,
  last_viewed_at DATETIME(3) NOT NULL,
  CONSTRAINT fk_vpv_user FOREIGN KEY (user_id) REFERENCES users (id) ON DELETE CASCADE
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;
