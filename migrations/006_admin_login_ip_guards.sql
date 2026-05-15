-- 管理后台登录：按 IP 记录连续失败次数，达阈值后永久封禁。

CREATE TABLE IF NOT EXISTS admin_login_ip_guards (
  ip VARCHAR(45) NOT NULL PRIMARY KEY COMMENT '客户端 IP（IPv4/IPv6）',
  consecutive_failures INT NOT NULL DEFAULT 0,
  banned_at DATETIME(3) NULL COMMENT '非空表示该 IP 已永久封禁',
  updated_at DATETIME(3) NOT NULL
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;
