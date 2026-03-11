CREATE TABLE `block`
 (
     `id`           bigint                                                         NOT NULL AUTO_INCREMENT,
     `slot`         bigint                                                         NOT NULL DEFAULT '0' COMMENT 'slot',
     `block_height` bigint                                                         NOT NULL DEFAULT '0' COMMENT 'block_height',
     `block_time`   timestamp                                                      NOT NULL COMMENT 'block_time',
     `status`       tinyint                                                        NOT NULL DEFAULT '0' COMMENT '1 processed, 2 failed',
     `sol_price`    decimal(64, 18)                                                NOT NULL DEFAULT '0.000000000000000000' COMMENT 'sol price',
     `created_at`   timestamp                                                      NOT NULL DEFAULT CURRENT_TIMESTAMP,
     `updated_at`   timestamp                                                      NOT NULL DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP,
     `deleted_at`   timestamp                                                      NULL     DEFAULT NULL,
     `err_message`  varchar(1000) CHARACTER SET utf8mb4 COLLATE utf8mb4_general_ci NOT NULL DEFAULT '' COMMENT 'error message',
     PRIMARY KEY (`id`),
     UNIQUE KEY `slot_index` (`slot`),
     KEY `block_time_index` (`block_time`)
 ) ENGINE = InnoDB
   DEFAULT CHARSET = utf8mb4
   COLLATE = utf8mb4_general_ci COMMENT ='block';