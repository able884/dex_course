-- ============================================================
-- Solana Limit Order Deployment Data - Devnet
-- Generated from deployment-devnet.json and deploy-config-devnet.json
-- Deployment Timestamp: 2025-12-25T10:52:27.696Z
-- ============================================================

-- ============================================================
-- 1. Insert Program Configuration
-- ============================================================
INSERT INTO `limit_order_config` (
  `program_id`,
  `config_pda`,
  `admin`,
  `treasury`,
  `matcher`,
  `whitelist_root`,
  `whitelist_version`,
  `pyth_confidence_tol_bps`,
  `pyth_max_staleness_slots`,
  `fee_bps_limit`,
  `paused`,
  `network`,
  `created_at`,
  `updated_at`
) VALUES (
  'C4Xn6ACR3XmTwpWm2fN82QSAx23TQkwVW8wro9Hdpqef',  -- program_id
  'HmxecGh6cn3QQ88bSonfJDyEnQWbuXLSmh6bLmpNNUM',  -- config_pda
  '6QC9MjYEBQ8XXxgs4ipJbJ3LFSK4zNvQgqg2SyaWYHW5',  -- admin
  '6QC9MjYEBQ8XXxgs4ipJbJ3LFSK4zNvQgqg2SyaWYHW5',  -- treasury
  '6QC9MjYEBQ8XXxgs4ipJbJ3LFSK4zNvQgqg2SyaWYHW5',  -- matcher
  NULL,                                              -- whitelist_root (无白名单)
  0,                                                 -- whitelist_version
  500,                                               -- pyth_confidence_tol_bps (5%)
  100,                                               -- pyth_max_staleness_slots
  5000,                                              -- fee_bps_limit (50%)
  0,                                                 -- paused (0=正常运行)
  'devnet',                                          -- network
  '2025-12-25 10:52:27',                            -- created_at
  '2025-12-25 10:52:27'                             -- updated_at
);

-- ============================================================
-- 2. Insert Market Configuration - SOL/USDC
-- ============================================================
INSERT INTO `limit_order_market` (
  `market_pda`,
  `base_mint`,
  `quote_mint`,
  `base_symbol`,
  `quote_symbol`,
  `base_decimals`,
  `quote_decimals`,
  `base_vault`,
  `quote_vault`,
  `tick_size`,
  `min_base_lot`,
  `min_quote_lot`,
  `maker_fee_bps`,
  `taker_fee_bps`,
  `fee_accumulator`,
  `seq_num`,
  `pyth_price_feed`,
  `paused`,
  `network`,
  `created_at`,
  `updated_at`
) VALUES (
  '99ER6YgkqtCPemSrqVNByB9juCiNgFUormPx9dSyLSkt',  -- market_pda
  'So11111111111111111111111111111111111111112',   -- base_mint (SOL)
  'USDCoctVLVnvTXBEuP9s8hntucdJokbo17RwHuNXemT',   -- quote_mint (USDC devnet)
  'SOL',                                             -- base_symbol
  'USDC',                                            -- quote_symbol
  9,                                                 -- base_decimals (SOL = 9)
  6,                                                 -- quote_decimals (USDC = 6)
  '8QQSeUWYXwzBNubpq7Kg2is9YBDtADT58dREHwPkE32z',  -- base_vault
  '8ZentPPfACnnATgxV5c78br41161gg2XkM5xTDPqL9pT',  -- quote_vault
  100,                                               -- tick_size (最小价格变动单位)
  1000000,                                           -- min_base_lot (最小0.001 SOL)
  1000000,                                           -- min_quote_lot (最小1 USDC)
  0,                                                 -- maker_fee_bps (0% maker费用)
  30,                                                -- taker_fee_bps (0.3% taker费用)
  0,                                                 -- fee_accumulator (初始累积费用为0)
  0,                                                 -- seq_num (初始订单序号为0)
  NULL,                                              -- pyth_price_feed (未配置价格源)
  0,                                                 -- paused (0=正常运行)
  'devnet',                                          -- network
  '2025-12-25 10:52:27',                            -- created_at
  '2025-12-25 10:52:27'                             -- updated_at
);

-- ============================================================
-- 验证插入的数据
-- ============================================================

-- 查询配置信息
SELECT
  id,
  program_id,
  config_pda,
  admin,
  network,
  CASE WHEN paused = 0 THEN 'Active' ELSE 'Paused' END as status,
  created_at
FROM limit_order_config
WHERE network = 'devnet';

-- 查询市场信息
SELECT
  id,
  CONCAT(base_symbol, '/', quote_symbol) as market_pair,
  market_pda,
  CONCAT(maker_fee_bps / 100, '%') as maker_fee,
  CONCAT(taker_fee_bps / 100, '%') as taker_fee,
  tick_size,
  min_base_lot,
  min_quote_lot,
  CASE WHEN paused = 0 THEN 'Active' ELSE 'Paused' END as status,
  network,
  created_at
FROM limit_order_market
WHERE network = 'devnet';

-- ============================================================
-- 注意事项
-- ============================================================
-- 1. 执行前请确保数据库 rc_dex_study 已创建
-- 2. 执行前请确保已运行 limit_order.sql 创建表结构
-- 3. 如果需要重新插入，请先删除旧数据：
--    DELETE FROM limit_order_market WHERE network = 'devnet';
--    DELETE FROM limit_order_config WHERE network = 'devnet';
-- 4. program_id 和 config_pda 的组合在数据库中是唯一的
-- 5. market_pda 在数据库中是唯一的
-- 6. base_mint + quote_mint + network 的组合是唯一的
