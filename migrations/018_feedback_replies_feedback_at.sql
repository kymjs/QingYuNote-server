-- 官方反馈回复：补充用户反馈时间（调用方传入）；可重复执行。

SELECT IF(
  (SELECT COUNT(*) FROM information_schema.COLUMNS
   WHERE TABLE_SCHEMA = DATABASE() AND TABLE_NAME = 'feedback_replies' AND COLUMN_NAME = 'feedback_at') = 0,
  'ALTER TABLE feedback_replies ADD COLUMN feedback_at DATETIME(3) NOT NULL DEFAULT ''1970-01-01 00:00:00.000'' COMMENT ''用户反馈时间（调用方传入）'' AFTER feedback_type',
  'SELECT 1'
) INTO @__note_migrate_sql;
PREPARE __note_migrate_stmt FROM @__note_migrate_sql;
EXECUTE __note_migrate_stmt;
DEALLOCATE PREPARE __note_migrate_stmt;
