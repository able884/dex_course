use anchor_lang::prelude::*;

#[account]
pub struct UserMargin {
    pub owner: Pubkey, // 保证金账户所属用户
    pub market: Pubkey, // 保证金账户所属市场
    pub base_free: u64, // 基础代币的可用余额
    pub quote_free: u64, // 报价代币的可用余额
    pub base_locked: u64, // 基础代币的锁定余额
    pub quote_locked: u64, // 报价代币的锁定余额
    pub bump: u8,
}

impl UserMargin {
    pub const SIZE: usize = 32 + 32 + 8 * 4 + 1;
}