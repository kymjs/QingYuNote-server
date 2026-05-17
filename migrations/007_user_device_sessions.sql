-- 用户设备会话表：记录每个设备/平台的使用时间，用于管理后台展示用户使用端口及最后打开时间
CREATE TABLE IF NOT EXISTS user_device_sessions (
  id BIGINT PRIMARY KEY AUTO_INCREMENT,
  user_id BIGINT NOT NULL,
  platform VARCHAR(16) NOT NULL COMMENT 'android/ios/harmony/windows/linux/macos',
  device_id VARCHAR(128) NOT NULL COMMENT '客户端生成的设备唯一ID',
  last_active_at DATETIME(3) NOT NULL DEFAULT CURRENT_TIMESTAMP(3) COMMENT '最后活跃时间',
  created_at DATETIME(3) NOT NULL DEFAULT CURRENT_TIMESTAMP(3),
  INDEX idx_user_platform (user_id, platform),
  INDEX idx_user_last_active (user_id, last_active_at DESC),
  UNIQUE INDEX idx_user_device (user_id, device_id)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;

-- 记录已有用户的设备会话（从 users.created_at 初始化）
-- 这确保新功能不会丢失现有用户的注册平台信息