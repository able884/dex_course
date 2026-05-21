-- 为 CLMM 池子和持仓表添加价格、价值和手续费字段
-- 执行日期: 2025-12-11
-- 说明: 在 consumer 服务监听 swap 和相关指令时更新这些字段，查询接口直接从数据库读取

-- ============================================
-- 1. 为 clmm_pool_info_v1 添加当前价格字段
-- ============================================
ALTER TABLE `clmm_pool_info_v1`
ADD COLUMN `current_price` DOUBLE NOT NULL DEFAULT 0 COMMENT '当前价格（Token1 per Token0）' AFTER `trade_fee_rate`,
ADD COLUMN `current_tick` INT NOT NULL DEFAULT 0 COMMENT '当前 Tick 索引' AFTER `current_price`;

-- ============================================
-- 2. 为 clmm_pool_info_v2 添加当前价格字段
-- ============================================
ALTER TABLE `clmm_pool_info_v2`
ADD COLUMN `current_price` DOUBLE NOT NULL DEFAULT 0 COMMENT '当前价格（Token1 per Token0）' AFTER `trade_fee_rate`,
ADD COLUMN `current_tick` INT NOT NULL DEFAULT 0 COMMENT '当前 Tick 索引' AFTER `current_price`;

-- ============================================
-- 3. 为 clmm_position 添加持仓价值和手续费字段
-- ============================================
ALTER TABLE `clmm_position`
ADD COLUMN `position_value_usd` DOUBLE NOT NULL DEFAULT 0 COMMENT '持仓总价值（USD）' AFTER `block_time_stamp`,
ADD COLUMN `unclaimed_fees_0` BIGINT NOT NULL DEFAULT 0 COMMENT '未提取手续费 Token0（原子单位）' AFTER `position_value_usd`,
ADD COLUMN `unclaimed_fees_1` BIGINT NOT NULL DEFAULT 0 COMMENT '未提取手续费 Token1（原子单位）' AFTER `unclaimed_fees_0`,
ADD COLUMN `unclaimed_fees_usd` DOUBLE NOT NULL DEFAULT 0 COMMENT '未提取手续费总价值（USD）' AFTER `unclaimed_fees_1`;

-- ============================================
-- 4. 为 CLMM 持仓表添加代币数量字段
-- ============================================
ALTER TABLE `clmm_position`
ADD COLUMN `token0_amount` DOUBLE NOT NULL DEFAULT 0 COMMENT 'Token0 数量（已考虑 decimals）' AFTER `unclaimed_fees_usd`,
ADD COLUMN `token1_amount` DOUBLE NOT NULL DEFAULT 0 COMMENT 'Token1 数量（已考虑 decimals）' AFTER `token0_amount`;
