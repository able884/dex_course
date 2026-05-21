use anchor_lang::prelude::*;

#[account]
pub struct UserMargin {
    pub owner: Pubkey,
    pub market: Pubkey,
    pub base_free: u64,
    pub quote_free: u64,
    pub base_locked: u64,
    pub quote_locked: u64,
    pub bump: u8,
}

impl UserMargin {
    pub const SIZE: usize = 32 + 32 + 8 * 4 + 1;
}