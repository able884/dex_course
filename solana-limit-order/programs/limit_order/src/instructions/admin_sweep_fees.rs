use anchor_lang::prelude::*;
use crate::state::config::ProgramConfig;
use crate::state::market::Market;
use anchor_spl::token::{Token, TokenAccount};
use crate::events::FeeTransfer;
use crate::error::LimitOrderError;
use anchor_spl::token::{self, Transfer};
use anchor_lang::context::CpiContext;

#[derive(Accounts)]
pub struct AdminSweepFees<'info> {
    pub config: Account<'info, ProgramConfig>,
    pub admin: Signer<'info>,
    #[account(mut, seeds = [b"LimitOrderMarket", market.base_mint.as_ref(), market.quote_mint.as_ref()], bump = market.bump)]
    pub market: Account<'info, Market>,
    #[account(mut, seeds = [b"MarketVault", market.key().as_ref(), market.quote_mint.as_ref()], bump)]
    pub quote_vault: Account<'info, TokenAccount>,
    #[account(mut)]
    pub treasury_token: Account<'info, TokenAccount>,
    pub token_program: Program<'info, Token>,
}

pub fn admin_sweep_fees(ctx: Context<AdminSweepFees>) -> Result<()> {
    let config = &ctx.accounts.config;
    require!(ctx.accounts.admin.key() == config.admin, LimitOrderError::Unauthorized);
    let market = &mut ctx.accounts.market;
    let fee_amount = market.fee_accumulator;
    if fee_amount == 0 {
        return Ok(());
    }
    let bump = market.bump;
    let seeds = &[b"LimitOrderMarket", market.base_mint.as_ref(), market.quote_mint.as_ref(), &[bump]];
    let signer = &[&seeds[..]];
    let cpi_accounts = Transfer {
        from: ctx.accounts.quote_vault.to_account_info(),
        to: ctx.accounts.treasury_token.to_account_info(),
        authority: market.to_account_info(),
    };
    let cpi_ctx =
        CpiContext::new_with_signer(ctx.accounts.token_program.to_account_info(), cpi_accounts, signer);
    token::transfer(cpi_ctx, fee_amount)?;
    market.fee_accumulator = 0;
    emit!(FeeTransfer {
        market: market.key(),
        to: ctx.accounts.treasury_token.key(),
        amount: fee_amount,
    });
    Ok(())
}
