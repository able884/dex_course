-- Solana 限价单相关数据库表
-- 数据库: rc_dex_study

-- ============================================================
-- 1. 限价单程序配置表
-- ============================================================
CREATE TABLE IF NOT EXISTS `limit_order_config` (
  `id` int NOT NULL AUTO_INCREMENT COMMENT '主键ID',
  `program_id` varchar(44) NOT NULL COMMENT 'Solana 程序 ID',
  `config_pda` varchar(44) NOT NULL COMMENT '配置账户 PDA',
  `admin` varchar(44) NOT NULL COMMENT '管理员公钥',
  `treasury` varchar(44) NOT NULL COMMENT '金库地址',
  `matcher` varchar(44) DEFAULT NULL COMMENT '撮合器公钥（NULL=开放给所有人）',
  `whitelist_root` varchar(64) DEFAULT NULL COMMENT '白名单 Merkle Root (hex)',
  `whitelist_version` bigint DEFAULT 0 COMMENT '白名单版本号',
  `pyth_confidence_tol_bps` int DEFAULT 500 COMMENT 'Pyth 置信度容忍（bps）',
  `pyth_max_staleness_slots` bigint DEFAULT 100 COMMENT 'Pyth 最大过期 slots',
  `fee_bps_limit` int DEFAULT 5000 COMMENT '费率上限（bps）',
  `paused` tinyint DEFAULT 0 COMMENT '暂停状态: 0=正常 1=暂停',
  `network` varchar(20) NOT NULL COMMENT '网络: devnet/mainnet',
  `created_at` timestamp DEFAULT CURRENT_TIMESTAMP COMMENT '创建时间',
  `updated_at` timestamp DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP COMMENT '更新时间',
  PRIMARY KEY (`id`),
  UNIQUE KEY `uk_program_network` (`program_id`, `network`),
  KEY `idx_network` (`network`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci COMMENT='限价单程序配置';

-- ============================================================
-- 2. 限价单市场配置表
-- ============================================================
CREATE TABLE IF NOT EXISTS `limit_order_market` (
  `id` int NOT NULL AUTO_INCREMENT COMMENT '主键ID',
  `market_pda` varchar(44) NOT NULL COMMENT '市场 PDA',
  `base_mint` varchar(44) NOT NULL COMMENT 'base 代币地址',
  `quote_mint` varchar(44) NOT NULL COMMENT 'quote 代币地址',
  `base_symbol` varchar(20) NOT NULL COMMENT 'base 代币符号',
  `quote_symbol` varchar(20) NOT NULL COMMENT 'quote 代币符号',
  `base_decimals` int NOT NULL COMMENT 'base 代币精度',
  `quote_decimals` int NOT NULL COMMENT 'quote 代币精度',
  `base_vault` varchar(44) NOT NULL COMMENT 'base 金库 PDA',
  `quote_vault` varchar(44) NOT NULL COMMENT 'quote 金库 PDA',
  `tick_size` bigint NOT NULL COMMENT '价格精度（lots）',
  `min_base_lot` bigint NOT NULL COMMENT '最小 base 下单量（lots）',
  `min_quote_lot` bigint NOT NULL COMMENT '最小 quote 下单量（lots）',
  `maker_fee_bps` int NOT NULL COMMENT 'maker 费率（bps）',
  `taker_fee_bps` int NOT NULL COMMENT 'taker 费率（bps）',
  `fee_accumulator` bigint DEFAULT 0 COMMENT '累积费用',
  `seq_num` bigint DEFAULT 0 COMMENT '订单序列号',
  `pyth_price_feed` varchar(44) DEFAULT NULL COMMENT 'Pyth 价格源',
  `paused` tinyint DEFAULT 0 COMMENT '暂停状态',
  `status` tinyint DEFAULT 0 COMMENT '市场状态: 0=创建中 1=正常 2=已关闭',
  `init_tx_hash` varchar(88) DEFAULT NULL COMMENT '初始化交易哈希',
  `network` varchar(20) NOT NULL COMMENT '网络',
  `created_at` timestamp DEFAULT CURRENT_TIMESTAMP COMMENT '创建时间',
  `updated_at` timestamp DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP COMMENT '更新时间',
  PRIMARY KEY (`id`),
  UNIQUE KEY `uk_market_pda` (`market_pda`),
  UNIQUE KEY `uk_mints_network` (`base_mint`, `quote_mint`, `network`),
  KEY `idx_base_mint` (`base_mint`),
  KEY `idx_quote_mint` (`quote_mint`),
  KEY `idx_network` (`network`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci COMMENT='限价单市场配置';

-- ============================================================
-- 3. 限价单订单表
-- ============================================================
CREATE TABLE IF NOT EXISTS `limit_order` (
  `id` bigint NOT NULL AUTO_INCREMENT COMMENT '主键ID',
  `order_pda` varchar(44) NOT NULL COMMENT '订单 PDA',
  `market_id` int NOT NULL COMMENT '市场 ID（外键）',
  `market_pda` varchar(44) NOT NULL COMMENT '市场 PDA',
  `owner` varchar(44) NOT NULL COMMENT '订单所有者',
  `margin_pda` varchar(44) NOT NULL COMMENT '保证金账户 PDA',
  `order_id` bigint NOT NULL COMMENT '链上订单序列号',

  -- 订单参数
  `side` tinyint NOT NULL COMMENT '方向: 1=Bid(买) 2=Ask(卖)',
  `price_lots` bigint NOT NULL COMMENT '价格（lots）',
  `qty_lots` bigint NOT NULL COMMENT '数量（lots）',
  `remaining_lots` bigint NOT NULL COMMENT '剩余数量（lots）',
  `locked_side` tinyint NOT NULL COMMENT '锁定方向: 1=Base 2=Quote',
  `expiry_slot` bigint NOT NULL COMMENT '过期 slot',
  `min_fill_bps` int DEFAULT NULL COMMENT '最小成交比例（bps）',
  `self_trade_behavior` tinyint NOT NULL COMMENT '自成交行为: 1=DecrementTake 2=CancelNew',

  -- 显示用字段（冗余，便于前端查询）
  `price_display` decimal(32,18) NOT NULL COMMENT '显示价格',
  `qty_display` decimal(32,18) NOT NULL COMMENT '显示数量',
  `remaining_display` decimal(32,18) NOT NULL COMMENT '剩余数量显示',
  `total_value_display` decimal(32,18) NOT NULL COMMENT '总价值显示',

  -- 状态
  `status` tinyint NOT NULL COMMENT '状态: 1=Active 2=Filled 3=Cancelled 4=Expired',
  `filled_percent` decimal(5,2) DEFAULT 0.00 COMMENT '成交百分比',

  -- 交易哈希
  `create_tx_hash` varchar(88) DEFAULT NULL COMMENT '创建交易哈希',
  `cancel_tx_hash` varchar(88) DEFAULT NULL COMMENT '取消交易哈希',

  -- 时间戳
  `created_at` timestamp DEFAULT CURRENT_TIMESTAMP COMMENT '创建时间',
  `updated_at` timestamp DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP COMMENT '更新时间',
  `filled_at` timestamp DEFAULT NULL COMMENT '完全成交时间',
  `cancelled_at` timestamp DEFAULT NULL COMMENT '取消时间',
  `expired_at` timestamp DEFAULT NULL COMMENT '过期时间',

  PRIMARY KEY (`id`),
  UNIQUE KEY `uk_order_pda` (`order_pda`),
  KEY `idx_owner_status` (`owner`, `status`),
  KEY `idx_market_status` (`market_id`, `status`),
  KEY `idx_market_side_price` (`market_id`, `side`, `status`, `price_lots`),
  KEY `idx_expiry` (`status`, `expiry_slot`),
  KEY `idx_created_at` (`created_at`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci COMMENT='限价单订单表';

-- ============================================================
-- 4. 限价单成交记录表
-- ============================================================
CREATE TABLE IF NOT EXISTS `limit_order_fill` (
  `id` bigint NOT NULL AUTO_INCREMENT COMMENT '主键ID',
  `maker_order_id` bigint NOT NULL COMMENT 'maker 订单 ID',
  `taker_order_id` bigint NOT NULL COMMENT 'taker 订单 ID',
  `market_id` int NOT NULL COMMENT '市场 ID',
  `market_pda` varchar(44) NOT NULL COMMENT '市场 PDA',
  `maker` varchar(44) NOT NULL COMMENT 'maker 地址',
  `taker` varchar(44) NOT NULL COMMENT 'taker 地址',

  -- 成交信息
  `qty_lots` bigint NOT NULL COMMENT '成交数量（lots）',
  `price_lots` bigint NOT NULL COMMENT '成交价格（lots）',
  `fee` bigint NOT NULL COMMENT 'taker 手续费（quote token, lots）',

  -- 显示字段
  `qty_display` decimal(32,18) NOT NULL COMMENT '成交数量（显示）',
  `price_display` decimal(32,18) NOT NULL COMMENT '成交价格（显示）',
  `fee_display` decimal(32,18) NOT NULL COMMENT '手续费（显示）',
  `total_value_display` decimal(32,18) NOT NULL COMMENT '成交总价值（显示）',

  -- 链上信息
  `tx_hash` varchar(88) NOT NULL COMMENT '撮合交易哈希',
  `slot` bigint NOT NULL COMMENT '成交 slot',
  `created_at` timestamp DEFAULT CURRENT_TIMESTAMP COMMENT '成交时间',

  PRIMARY KEY (`id`),
  KEY `idx_maker_order` (`maker_order_id`),
  KEY `idx_taker_order` (`taker_order_id`),
  KEY `idx_market_time` (`market_id`, `created_at`),
  KEY `idx_maker` (`maker`),
  KEY `idx_taker` (`taker`),
  KEY `idx_tx_hash` (`tx_hash`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci COMMENT='限价单成交记录';

-- ============================================================
-- 5. 用户保证金账户表（可选，用于缓存链上数据）
-- ============================================================
CREATE TABLE IF NOT EXISTS `limit_order_margin` (
  `id` bigint NOT NULL AUTO_INCREMENT COMMENT '主键ID',
  `margin_pda` varchar(44) NOT NULL COMMENT '保证金账户 PDA',
  `market_id` int NOT NULL COMMENT '市场 ID',
  `market_pda` varchar(44) NOT NULL COMMENT '市场 PDA',
  `owner` varchar(44) NOT NULL COMMENT '用户地址',

  -- 余额信息
  `base_free` bigint DEFAULT 0 COMMENT 'base 可用余额（lots）',
  `base_locked` bigint DEFAULT 0 COMMENT 'base 锁定余额（lots）',
  `quote_free` bigint DEFAULT 0 COMMENT 'quote 可用余额（lots）',
  `quote_locked` bigint DEFAULT 0 COMMENT 'quote 锁定余额（lots）',

  -- 显示字段
  `base_free_display` decimal(32,18) DEFAULT 0 COMMENT 'base 可用（显示）',
  `base_locked_display` decimal(32,18) DEFAULT 0 COMMENT 'base 锁定（显示）',
  `quote_free_display` decimal(32,18) DEFAULT 0 COMMENT 'quote 可用（显示）',
  `quote_locked_display` decimal(32,18) DEFAULT 0 COMMENT 'quote 锁定（显示）',

  -- 账户状态
  `status` tinyint DEFAULT 0 COMMENT '状态: 0=创建中 1=正常 2=已关闭',
  `init_tx_hash` varchar(88) DEFAULT NULL COMMENT '初始化交易哈希',

  -- 最后同步时间
  `last_sync_slot` bigint DEFAULT 0 COMMENT '最后同步 slot',
  `created_at` timestamp DEFAULT CURRENT_TIMESTAMP COMMENT '创建时间',
  `updated_at` timestamp DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP COMMENT '更新时间',

  PRIMARY KEY (`id`),
  UNIQUE KEY `uk_margin_pda` (`margin_pda`),
  UNIQUE KEY `uk_owner_market` (`owner`, `market_id`),
  KEY `idx_owner` (`owner`),
  KEY `idx_market` (`market_id`),
  KEY `idx_status` (`status`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci COMMENT='限价单保证金账户';

-- ============================================================
-- 6. 索引视图（用于快速查询订单簿）
-- ============================================================
-- 视图：活跃订单簿
CREATE OR REPLACE VIEW `v_limit_order_book_active` AS
SELECT
  lo.id,
  lo.market_id,
  lo.market_pda,
  lom.base_symbol,
  lom.quote_symbol,
  lo.side,
  lo.price_display,
  lo.qty_display,
  lo.remaining_display,
  lo.order_id,
  lo.owner,
  lo.created_at
FROM
  limit_order lo
  INNER JOIN limit_order_market lom ON lo.market_id = lom.id
WHERE
  lo.status = 1  -- Active
ORDER BY
  lo.market_id,
  lo.side,
  CASE WHEN lo.side = 1 THEN -lo.price_lots ELSE lo.price_lots END,  -- Bid: 价格从高到低, Ask: 价格从低到高
  lo.created_at ASC;  -- 同价格按时间优先

-- 视图：用户订单统计
CREATE OR REPLACE VIEW `v_user_order_stats` AS
SELECT
  owner,
  market_id,
  COUNT(*) as total_orders,
  SUM(CASE WHEN status = 1 THEN 1 ELSE 0 END) as active_orders,
  SUM(CASE WHEN status = 2 THEN 1 ELSE 0 END) as filled_orders,
  SUM(CASE WHEN status = 3 THEN 1 ELSE 0 END) as cancelled_orders,
  SUM(CASE WHEN status = 4 THEN 1 ELSE 0 END) as expired_orders,
  SUM(total_value_display) as total_value
FROM
  limit_order
GROUP BY
  owner, market_id;

-- ============================================================
-- 初始数据
-- ============================================================

-- 示例：插入配置（需要替换为实际值）
-- INSERT INTO limit_order_config (program_id, config_pda, admin, treasury, network)
-- VALUES ('Lo111111111111111111111111111111111111111111', 'ConfigPDA...', 'AdminPubkey...', 'TreasuryPubkey...', 'devnet');

-- 示例：插入市场（需要替换为实际值）
-- INSERT INTO limit_order_market (
--   market_pda, base_mint, quote_mint, base_symbol, quote_symbol,
--   base_decimals, quote_decimals, base_vault, quote_vault,
--   tick_size, min_base_lot, min_quote_lot, maker_fee_bps, taker_fee_bps, network
-- ) VALUES (
--   'MarketPDA...', 'SOL_MINT...', 'USDC_MINT...', 'SOL', 'USDC',
--   9, 6, 'BaseVaultPDA...', 'QuoteVaultPDA...',
--   100, 1000000, 1000000, 0, 30, 'devnet'
-- );
