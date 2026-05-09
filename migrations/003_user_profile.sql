-- 用户可编辑资料（均可为空）；password_hash 仅存 bcrypt，不回传明文
-- 本迁移对「已存在的列」跳过 ALTER，可重复执行，便于线上库增量升级

SELECT IF(
  (SELECT COUNT(*) FROM information_schema.COLUMNS
   WHERE TABLE_SCHEMA = DATABASE() AND TABLE_NAME = 'users' AND COLUMN_NAME = 'display_name') = 0,
  'ALTER TABLE users ADD COLUMN display_name VARCHAR(191) NULL',
  'SELECT 1'
) INTO @__note_migrate_sql;
PREPARE __note_migrate_stmt FROM @__note_migrate_sql;
EXECUTE __note_migrate_stmt;
DEALLOCATE PREPARE __note_migrate_stmt;

SELECT IF(
  (SELECT COUNT(*) FROM information_schema.COLUMNS
   WHERE TABLE_SCHEMA = DATABASE() AND TABLE_NAME = 'users' AND COLUMN_NAME = 'avatar_url') = 0,
  'ALTER TABLE users ADD COLUMN avatar_url VARCHAR(512) NULL',
  'SELECT 1'
) INTO @__note_migrate_sql;
PREPARE __note_migrate_stmt FROM @__note_migrate_sql;
EXECUTE __note_migrate_stmt;
DEALLOCATE PREPARE __note_migrate_stmt;

SELECT IF(
  (SELECT COUNT(*) FROM information_schema.COLUMNS
   WHERE TABLE_SCHEMA = DATABASE() AND TABLE_NAME = 'users' AND COLUMN_NAME = 'phone') = 0,
  'ALTER TABLE users ADD COLUMN phone VARCHAR(32) NULL',
  'SELECT 1'
) INTO @__note_migrate_sql;
PREPARE __note_migrate_stmt FROM @__note_migrate_sql;
EXECUTE __note_migrate_stmt;
DEALLOCATE PREPARE __note_migrate_stmt;

SELECT IF(
  (SELECT COUNT(*) FROM information_schema.COLUMNS
   WHERE TABLE_SCHEMA = DATABASE() AND TABLE_NAME = 'users' AND COLUMN_NAME = 'email') = 0,
  'ALTER TABLE users ADD COLUMN email VARCHAR(191) NULL',
  'SELECT 1'
) INTO @__note_migrate_sql;
PREPARE __note_migrate_stmt FROM @__note_migrate_sql;
EXECUTE __note_migrate_stmt;
DEALLOCATE PREPARE __note_migrate_stmt;

SELECT IF(
  (SELECT COUNT(*) FROM information_schema.COLUMNS
   WHERE TABLE_SCHEMA = DATABASE() AND TABLE_NAME = 'users' AND COLUMN_NAME = 'password_hash') = 0,
  'ALTER TABLE users ADD COLUMN password_hash VARCHAR(255) NULL',
  'SELECT 1'
) INTO @__note_migrate_sql;
PREPARE __note_migrate_stmt FROM @__note_migrate_sql;
EXECUTE __note_migrate_stmt;
DEALLOCATE PREPARE __note_migrate_stmt;
