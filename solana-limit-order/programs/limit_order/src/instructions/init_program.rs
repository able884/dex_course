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
    pub admin: Pubkey, // 管理员地址，拥有权限更新程序参数和暂停/恢复程序
    pub treasury: Pubkey, // 金库地址，接收交易手续费
    pub whitelist_root: [u8; 32], // 白名单根地址，用于验证白名单地址
    pub whitelist_version: u64, // 白名单版本号，用于验证白名单地址
    pub pyth_confidence_tol_bps: u16, // Pyth价格容忍度，以百分比表示
    pub pyth_max_staleness_slots: u64, // Pyth最大过时槽数
    pub fee_bps_limit: u16, // 交易手续费百分比限制
    pub matcher: Pubkey, // 匹配器地址，用于处理订单
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
