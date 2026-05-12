-- 会籍充值/核销审计：每笔成功延长订阅时落库，便于与微信、苹果或兑换码渠道核验。

CREATE TABLE IF NOT EXISTS membership_recharge_records (
  id BIGINT PRIMARY KEY AUTO_INCREMENT,
  user_id BIGINT NOT NULL,
  channel VARCHAR(16) NOT NULL COMMENT 'wechat | apple | redeem',
  order_id BIGINT NULL COMMENT '服务端 orders.id（支付类）',
  out_trade_no VARCHAR(64) NULL COMMENT '商户订单号（与 orders.out_trade_no 一致）',
  gateway_transaction_id VARCHAR(128) NULL COMMENT '微信/苹果网关交易号',
  redemption_code_hash CHAR(64) NULL COMMENT '兑换码 SHA256 hex（与 redemption_codes.code_hash 一致）',
  plan_id VARCHAR(32) NOT NULL,
  created_at DATETIME(3) NOT NULL,
  KEY idx_mrr_user_created (user_id, created_at),
  KEY idx_mrr_out_trade (out_trade_no),
  KEY idx_mrr_gateway_tx (gateway_transaction_id),
  KEY idx_mrr_code_hash (redemption_code_hash),
  CONSTRAINT fk_mrr_user FOREIGN KEY (user_id) REFERENCES users (id) ON DELETE CASCADE,
  CONSTRAINT fk_mrr_order FOREIGN KEY (order_id) REFERENCES orders (id) ON DELETE SET NULL
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;
