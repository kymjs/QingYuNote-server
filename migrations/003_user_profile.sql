-- 用户可编辑资料（均可为空）；password_hash 仅存 bcrypt，不回传明文

ALTER TABLE users ADD COLUMN display_name VARCHAR(191) NULL;
ALTER TABLE users ADD COLUMN avatar_url VARCHAR(512) NULL;
ALTER TABLE users ADD COLUMN phone VARCHAR(32) NULL;
ALTER TABLE users ADD COLUMN email VARCHAR(191) NULL;
ALTER TABLE users ADD COLUMN password_hash VARCHAR(255) NULL;
