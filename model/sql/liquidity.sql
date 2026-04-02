-- CPMM 手续费层级定义表
CREATE TABLE `cpmm_fee_tiers` (
  `id` bigint NOT NULL AUTO_INCREMENT COMMENT '主键ID',
  `pool_type` varchar(16) NOT NULL DEFAULT 'CPMM' COMMENT '池子类型',
  `program_address` varchar(64) NOT NULL DEFAULT '' COMMENT 'CPMM 合约地址',
  `value_bps` int NOT NULL COMMENT '手续费值(基点，1 bps = 0.01%)',
  `label` varchar(32) NOT NULL COMMENT '手续费标签',
  `description` varchar(255) DEFAULT '' COMMENT '手续费描述',
  `tick_spacing` int NOT NULL DEFAULT 1 COMMENT '价格刻度间隔',
  `config_index` int NOT NULL DEFAULT 0 COMMENT 'Raydium AMM Config索引',
  `address` varchar(64) NOT NULL DEFAULT '' COMMENT 'AMM Config 链上固定地址',
  `priority_order` int NOT NULL DEFAULT 100 COMMENT '优先级排序(数值越小优先级越高)',
  `version` varchar(32) NOT NULL DEFAULT 'v1' COMMENT '版本号',
  `updated_at` timestamp NOT NULL DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP COMMENT '更新时间',
  `created_at` timestamp NOT NULL DEFAULT CURRENT_TIMESTAMP COMMENT '创建时间',
  PRIMARY KEY (`id`),
  UNIQUE KEY `uniq_pool_bps` (`pool_type`,`value_bps`),
  UNIQUE KEY `uniq_program_index` (`program_address`,`config_index`),
  UNIQUE KEY `uniq_address` (`address`),
  KEY `idx_priority_order` (`priority_order`),
  KEY `idx_updated_at` (`updated_at`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_general_ci COMMENT='CPMM 手续费层级配置表';

-- 插入手续费层级数据 (从 0.25% 到 4%)
-- program_address: CPMM 合约地址 (Devnet)
-- config_index: 对应 Raydium 链上的 AMM Config 索引
-- address: AMM Config 的链上固定地址 (Devnet)
-- value_bps: 手续费值，其中 2500 = 0.25%, 10000 = 1%
INSERT INTO `cpmm_fee_tiers` (`pool_type`, `program_address`, `value_bps`, `label`, `description`, `tick_spacing`, `config_index`, `address`, `priority_order`, `version`) VALUES
('CPMM', 'Db2MGEacxHrshK1UV4k4rBdVVtoET8hTBzh6X939c8AW', 2500, '0.25%', '超低手续费 - 适用于稳定币交易对', 1, 0, '53owGrWT4gbQucpueH81wW984WZeDgd26HKrphz6HJbf', 10, 'v1'),
('CPMM', 'Db2MGEacxHrshK1UV4k4rBdVVtoET8hTBzh6X939c8AW', 3000, '0.30%', '极低手续费 - 适用于稳定币交易对', 1, 1, '3rrSiZmVv9pqNZdhpBXv1eWHsAa65zSTuWryJUyhdinv', 20, 'v1'),
('CPMM', 'Db2MGEacxHrshK1UV4k4rBdVVtoET8hTBzh6X939c8AW', 5000, '0.50%', '低手续费 - 适用于主流币种交易对', 5, 2, '8s3dzSxqBXg763PksvHRT8hRPR8hRXvMejZKPZ8WyJUQ', 30, 'v1'),
('CPMM', 'Db2MGEacxHrshK1UV4k4rBdVVtoET8hTBzh6X939c8AW', 10000, '1.00%', '标准手续费 - 适用于一般交易对', 10, 3, 'AwZ9freR2a5WQE3rkvJ6Sem3cEDFeFkyBV8MGYc1QwTU', 40, 'v1'),
('CPMM', 'Db2MGEacxHrshK1UV4k4rBdVVtoET8hTBzh6X939c8AW', 40000, '4.00%', '高手续费 - 适用于低流动性或高风险交易对', 20, 4, 'Egn9AWQrkstM4NJv2g8rqLfxiYdgFWLr2UxrsrvSRCQZ', 50, 'v1');

-- CPMM 允许的代币列表
CREATE TABLE `cpmm_allowed_tokens` (
  `id` bigint NOT NULL AUTO_INCREMENT COMMENT '主键ID',
  `chain_id` bigint NOT NULL COMMENT '链 ID',
  `mint` varchar(64) NOT NULL COMMENT 'SPL mint 地址',
  `program` varchar(64) NOT NULL DEFAULT '' COMMENT '标准 Token Program',
  `name` varchar(128) NOT NULL COMMENT '代币名称',
  `symbol` varchar(32) NOT NULL COMMENT '代币符号',
  `decimals` int NOT NULL DEFAULT 9 COMMENT '代币精度',
  `logo` varchar(512) NOT NULL DEFAULT '' COMMENT '代币图标',
  `description` text COMMENT '代币描述',
  `status` enum('active','paused','blocked') NOT NULL DEFAULT 'active' COMMENT '代币运营状态',
  `tags` json DEFAULT NULL COMMENT '标签(如：稳定币/主流币)',
  `priority_order` int NOT NULL DEFAULT 100 COMMENT '优先级排序(数值越小越靠前)',
  `created_at` timestamp NOT NULL DEFAULT CURRENT_TIMESTAMP COMMENT '创建时间',
  `updated_at` timestamp NOT NULL DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP COMMENT '更新时间',
  PRIMARY KEY (`id`),
  UNIQUE KEY `uniq_cpmm_token` (`chain_id`,`mint`),
  KEY `idx_cpmm_token_status` (`status`,`priority_order`),
  KEY `idx_cpmm_token_updated_at` (`updated_at`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_general_ci COMMENT='允许添加到 CPMM 的代币';

INSERT INTO `cpmm_allowed_tokens`
(`chain_id`, `mint`, `program`, `name`, `symbol`, `decimals`, `logo`, `description`, `status`, `tags`, `priority_order`, `created_at`, `updated_at`) VALUES
(100000, 'So11111111111111111111111111111111111111112', 'TokenkegQfeZyiNwAJbNbGKPFXCWuBvf9Ss623VQ5DA', 'Wrapped SOL', 'SOL', 9, '', '', 'active', '[]', 20, '2025-11-17 02:41:45', '2025-11-17 06:17:44'),
(100000, '5B8vQ6Nak7DySL7MjpUSxZjv26Lxhynr5VziFaPkkJPH', 'TokenkegQfeZyiNwAJbNbGKPFXCWuBvf9Ss623VQ5DA', 'L BTC', 'LBTC', 6, 'https://gateway.lighthouse.storage/ipfs/bafkreiasjlsp3urir2ifjczadfpgxpb6u6r6oj2ivzscpdujs5njyuqynm', 'LL BTC', 'active', '[]', 100, '2025-11-16 20:54:12', '2025-11-16 20:54:12'),
(100000, 'AHXXR8RHHaiQnEwbVpxTRtTRB6oo8AeEbVsJoBbRLERX', 'TokenkegQfeZyiNwAJbNbGKPFXCWuBvf9Ss623VQ5DA', 'Caution/Warning', 'KRN', 6, 'https://ipfs.io/ipfs/QmNsernFc9T3u6N4NWiLzSQJZqgY87HUkiemEvrwYiuCSd', 'A memecoin that evokes a sense of caution, stability, and reliability, often associated with warning signs or notices to signal attention and vigilance.', 'active', '[]', 10, '2025-11-17 02:41:44', '2025-11-17 02:41:44'),
(100000, 'FL5FJ8CQhR23hMNmLo2tnA9Wobd64H73qy1dap7yazDy', 'TokenkegQfeZyiNwAJbNbGKPFXCWuBvf9Ss623VQ5DA', 'Grey', 'GYX', 6, 'https://ipfs.io/ipfs/QmXo8cZ4QguhuWqdw6T2Pacp9SudrYC4yCgropF7LqwQz5', 'Greycoin: Because who needs excitement, anyway?', 'active', '[]', 30, '2025-11-17 02:42:38', '2025-11-17 02:42:38'),
(100000, '7sbQtxtXYw7wZg8CPTzWX5CBUWvfFrHcByu9DESAmPMu', 'TokenkegQfeZyiNwAJbNbGKPFXCWuBvf9Ss623VQ5DA', 'Neon Green', 'GSOFI', 6, 'https://ipfs.io/ipfs/QmYnpvMcxxsm4YyNvPSdctG3CoXfGU4xNK2DQPhE2gX77G', 'Represents a vibrant, energetic, and attention-grabbing hue that symbolizes innovation, growth, and excitement, often associated with new and forward-thinking ideas or ventures.', 'active', '[]', 40, '2025-11-17 03:45:25', '2025-11-17 03:45:25'),
(100000, '2ANcTVWQu8hcQJT11zem9RgHU8MfLpnMmTJxgETynhx5', 'TokenkegQfeZyiNwAJbNbGKPFXCWuBvf9Ss623VQ5DA', 'LMAO!', 'LMAO!', 6, 'https://ipfs.io/ipfs/QmSZNbtG35W4BUueKVnjLQjr6RKAMm6X3HRrbmJdRz4QUc', 'LMAO!', 'active', '[]', 60, '2025-11-17 04:31:53', '2025-11-17 04:31:53'),
(100000, 'HbtaLcZvz8npPRDdmNTaWZRTWXNRzL9sCF7GWF1uEKUi', 'TokenkegQfeZyiNwAJbNbGKPFXCWuBvf9Ss623VQ5DA', 'NALA', 'NALA', 6, 'https://ipfs.io/ipfs/bafkreif323sdc7uvp5o2nasgrvfegdulhubvsomgqa5kdxvlmxjuxwcrvu', 'Nala is the most famous cat in the world she even holds a Guinness World Record for it! Now, we bring her legend to the Solana blockchain with $NALA, a token made as a tribute to this amazing cat.', 'active', '[]', 70, '2025-11-17 04:32:12', '2025-11-17 04:32:12'),
(100000, '1CsG7sx7qPBP36yceSSErMZmcWEgPC2zobVSqbWsEPY', 'TokenkegQfeZyiNwAJbNbGKPFXCWuBvf9Ss623VQ5DA', 'Neutral Gray', 'GNZ', 6, 'https://ipfs.io/ipfs/QmbDYrFsL2Zut2WrDgTZQ4Wa42D5s4558dUZ4JXnFTrzpq', 'A cryptocurrency representing a calm, balanced, and approachable tone that evokes feelings of stability, serenity, and mediocrity — much like a perfectly ordinary day.', 'active', '[]', 90, '2025-11-17 04:49:25', '2025-11-17 04:49:25'),
(100000, 'EwcmzUM6yy8fxBiBNBHi8FaBZRVDLrjBB3GihBiU2Hvn', 'TokenkegQfeZyiNwAJbNbGKPFXCWuBvf9Ss623VQ5DA', 'shork', 'shork', 6, 'https://cf-ipfs.com/ipfs/QmZ4PJT5qZ1MyMwaYz1dbDD3kpBE4bkskwmampKAQWuNRe', 'just a lil shork', 'active', '[]', 110, '2025-11-17 05:28:12', '2025-11-17 05:28:12'),
(100000, '7nojmxJZCMVqVVzn2dFpUErJtoeW2p8YfCjqW1WQoGnB', 'TokenkegQfeZyiNwAJbNbGKPFXCWuBvf9Ss623VQ5DA', 'Cerulean', 'CYRX', 6, 'https://ipfs.io/ipfs/QmQYhmT4BDWXkcvdod7G3xHULr8NdiJfntyfGUdYgCcG9w', 'The color represents a calm, soothing blue-green hue that evokes feelings of serenity and tranquility, suggesting a sense of stability and balance amidst complexity and uncertainty.', 'active', '[]', 120, '2025-11-17 05:55:21', '2025-11-17 05:55:21');
