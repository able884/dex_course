use anchor_lang::prelude::*;
use crate::state::config::ProgramConfig;
use crate::error::LimitOrderError;
use crate::events::ProgramParamUpdated;
use crate::constants::MAX_FEE_BPS;

#[derive(Accounts)]
pub struct UpdateProgramParams<'info> {
    #[account(mut, seeds = [b"config"], bump = config.bump)]
    pub config: Account<'info, ProgramConfig>,
    pub admin: Signer<'info>,
}

#[derive(AnchorSerialize, AnchorDeserialize)]
pub struct UpdateProgramParamsData {
    pub whitelist_root: Option<[u8; 32]>,
    pub pyth_confidence_tol_bps: Option<u16>,
    pub pyth_max_staleness_slots: Option<u64>,
    pub fee_bps_limit: Option<u16>,
    pub paused: Option<bool>,
    pub matcher: Option<Pubkey>,
}

pub fn update_program_params(
    ctx: Context<UpdateProgramParams>,
    params: UpdateProgramParamsData,
) -> Result<()> {
    let config = &mut ctx.accounts.config;
    require!(ctx.accounts.admin.key() == config.admin, LimitOrderError::Unauthorized);
    if let Some(root) = params.whitelist_root {
        config.whitelist_root = root;
        config.whitelist_version = config.whitelist_version.checked_add(1).ok_or(LimitOrderError::MathOverflow)?;
    }
    if let Some(pyth_tol) = params.pyth_confidence_tol_bps {
        config.pyth_confidence_tol_bps = pyth_tol;
    }
    if let Some(pyth_staleness) = params.pyth_max_staleness_slots {
        config.pyth_max_staleness_slots = pyth_staleness;
    }
    if let Some(limit) = params.fee_bps_limit {
        require!(limit <= MAX_FEE_BPS, LimitOrderError::InvalidFeeBps);
        config.fee_bps_limit = limit;
    }
    if let Some(paused) = params.paused {
        config.paused = paused;
    }
    if let Some(matcher) = params.matcher {
        config.matcher = matcher;
    }
    emit!(ProgramParamUpdated {
        admin: config.admin,
        treasury: config.treasury,
        whitelist_root: config.whitelist_root,
        whitelist_version: config.whitelist_version,
        fee_bps_limit: config.fee_bps_limit,
        paused: config.paused,
    });
    Ok(())
}
