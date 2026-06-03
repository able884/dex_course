use anchor_lang::prelude::*;
use anchor_lang::solana_program::keccak;
use crate::error::LimitOrderError;

#[account]
pub struct ProgramConfig {
    pub admin: Pubkey, // 管理员地址，拥有权限更新程序参数
    pub treasury: Pubkey, // 收益池地址，用于分配交易手续费
    pub whitelist_root: [u8; 32], // 白名单根地址，用于验证用户是否在白名单中
    pub whitelist_version: u64, // 白名单版本，用于验证白名单是否是最新的
    pub pyth_confidence_tol_bps: u16, // Pyth价格可信度容忍度，以百分比表示
    pub pyth_max_staleness_slots: u64, // Pyth最大过时槽数
    pub fee_bps_limit: u16, // 交易手续费上限，以百分比表示
    pub paused: bool, // 是否暂停程序
    pub matcher: Pubkey, // 匹配器地址，用于处理订单匹配
    pub bump: u8, // bump值，用于验证账户的唯一性
}

impl ProgramConfig {
    pub const SIZE: usize = 32 + 32 + 32 + 8 + 2 + 8 + 2 + 1 + 32 + 1;
}

// 验证白名单函数，使用Merkle树验证用户是否在白名单中
// config 是程序配置，user是用户地址，proof是Merkle树的证明
pub fn verify_whitelist(
    config: &ProgramConfig,
    user: Pubkey,
    proof: &[[u8; 32]],
) -> Result<()> {
    if config.whitelist_root == [0u8; 32] {
        return Ok(());
    }

    let mut computed_hash = keccak::hashv(&[user.as_ref()]).0;

    for proof_element in proof.iter() {
        computed_hash = if computed_hash <= *proof_element {
            keccak::hashv(&[&computed_hash, proof_element]).0
        } else {
            keccak::hashv(&[proof_element, &computed_hash]).0
        };
    }

    require!(
        computed_hash == config.whitelist_root,
        LimitOrderError::WhitelistCheckFailed
    );

    Ok(())
}