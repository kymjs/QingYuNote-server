-- 官方对用户反馈的回复：同一用户可多条，客户端只展示最新一条。

CREATE TABLE IF NOT EXISTS feedback_replies (
  id BIGINT NOT NULL AUTO_INCREMENT PRIMARY KEY,
  user_id BIGINT NOT NULL,
  feedback_content TEXT NOT NULL,
  feedback_type VARCHAR(64) NOT NULL DEFAULT '',
  official_reply TEXT NOT NULL,
  created_at DATETIME(3) NOT NULL,
  KEY idx_feedback_replies_user_id_id (user_id, id DESC),
  CONSTRAINT fk_feedback_replies_user FOREIGN KEY (user_id) REFERENCES users (id) ON DELETE CASCADE
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;
