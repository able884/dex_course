use anchor_lang::prelude::*;
use crate::state::config::ProgramConfig;
use crate::error::LimitOrderError;
use crate::events::ProgramParamUpdated;
use crate::constants::MAX_FEE_BPS;

#[derive(Accounts)]
pub struct InitProgram<'info> {
    #[account(
        init,
        seeds = [b"config"],
        bump,
        payer = payer,
        space = 8 + ProgramConfig::SIZE
    )]
    pub config: Account<'info, ProgramConfig>,
    #[account(mut)]
    pub payer: Signer<'info>,
    pub system_program: Program<'info, System>,
}

#[derive(AnchorSerialize, AnchorDeserialize)]
pub struct InitProgramParams {
    pub admin: Pubkey,
    pub treasury: Pubkey,
    pub whitelist_root: [u8; 32],
    pub whitelist_version: u64,
    pub pyth_confidence_tol_bps: u16,
    pub pyth_max_staleness_slots: u64,
    pub fee_bps_limit: u16,
    pub matcher: Pubkey,
}

pub fn init_program(ctx: Context<InitProgram>, params: InitProgramParams) -> Result<()> {
    let config = &mut ctx.accounts.config;
    config.admin = params.admin;
    config.treasury = params.treasury;
    config.whitelist_root = params.whitelist_root;
    config.whitelist_version = params.whitelist_version;
    config.pyth_confidence_tol_bps = params.pyth_confidence_tol_bps;
    config.pyth_max_staleness_slots = params.pyth_max_staleness_slots;
    require!(params.fee_bps_limit <= MAX_FEE_BPS, LimitOrderError::InvalidFeeBps);
    config.fee_bps_limit = params.fee_bps_limit;
    config.paused = false;
    config.matcher = params.matcher;
    config.bump = ctx.bumps.config;
    emit!(ProgramParamUpdated {
        admin: params.admin,
        treasury: params.treasury,
        whitelist_root: params.whitelist_root,
        whitelist_version: params.whitelist_version,
        fee_bps_limit: params.fee_bps_limit,
        paused: false,
    });
    Ok(())
}
