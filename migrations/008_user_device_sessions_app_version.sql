-- 用户设备会话：记录客户端版本号，供管理后台「使用端口」列展示
-- 本迁移对「已存在的列」跳过 ALTER，可重复执行，便于 deploy.sh migrate / RUN_MIGRATE_ON_UPDATE

SELECT IF(
  (SELECT COUNT(*) FROM information_schema.COLUMNS
   WHERE TABLE_SCHEMA = DATABASE() AND TABLE_NAME = 'user_device_sessions' AND COLUMN_NAME = 'app_version') = 0,
  'ALTER TABLE user_device_sessions ADD COLUMN app_version VARCHAR(32) NULL COMMENT ''客户端版本号（X-App-Version）'' AFTER device_id',
  'SELECT 1'
) INTO @__note_migrate_sql;
PREPARE __note_migrate_stmt FROM @__note_migrate_sql;
EXECUTE __note_migrate_stmt;
DEALLOCATE PREPARE __note_migrate_stmt;
