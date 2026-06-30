-- 按用户查询邀请弹窗曝光历史（profile invite_popup_pending 判定）

SELECT IF(
  (SELECT COUNT(*) FROM information_schema.STATISTICS
   WHERE TABLE_SCHEMA = DATABASE()
     AND TABLE_NAME = 'invite_popup_events'
     AND INDEX_NAME = 'idx_ipe_user_type') = 0,
  'CREATE INDEX idx_ipe_user_type ON invite_popup_events (user_id, event_type)',
  'SELECT 1'
) INTO @__note_migrate_sql;
PREPARE __note_migrate_stmt FROM @__note_migrate_sql;
EXECUTE __note_migrate_stmt;
DEALLOCATE PREPARE __note_migrate_stmt;
