use anchor_lang::prelude::*;
use anchor_lang::solana_program::keccak;
use crate::error::LimitOrderError;

#[account]
pub struct ProgramConfig {
    pub admin: Pubkey,
    pub treasury: Pubkey,
    pub whitelist_root: [u8; 32],
    pub whitelist_version: u64,
    pub pyth_confidence_tol_bps: u16,
    pub pyth_max_staleness_slots: u64,
    pub fee_bps_limit: u16,
    pub paused: bool,
    pub matcher: Pubkey,
    pub bump: u8,
}

impl ProgramConfig {
    pub const SIZE: usize = 32 + 32 + 32 + 8 + 2 + 8 + 2 + 1 + 32 + 1;
}

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