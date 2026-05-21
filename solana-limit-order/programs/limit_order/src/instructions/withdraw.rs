use anchor_lang::prelude::*;
use crate::state::config::ProgramConfig;
use crate::state::market::Market;
use crate::state::margin::UserMargin;
use anchor_spl::token::{Token, TokenAccount};
use anchor_spl::token::{self, Transfer};
use crate::state::market::TokenSide;
use crate::events::WithdrawEvent;
use crate::error::LimitOrderError;

#[derive(Accounts)]
pub struct Withdraw<'info> {
    pub config: Account<'info, ProgramConfig>,
    #[account(seeds = [b"LimitOrderMarket", market.base_mint.as_ref(), market.quote_mint.as_ref()], bump = market.bump)]
    pub market: Account<'info, Market>,
    #[account(mut, seeds = [b"margin", market.key().as_ref(), owner.key().as_ref()], bump = margin.bump)]
    pub margin: Account<'info, UserMargin>,
    pub owner: Signer<'info>,
    #[account(mut, constraint = user_base_token.mint == market.base_mint)]
    pub user_base_token: Account<'info, TokenAccount>,
    #[account(mut, constraint = user_quote_token.mint == market.quote_mint)]
    pub user_quote_token: Account<'info, TokenAccount>,
    #[account(mut, seeds = [b"MarketVault", market.key().as_ref(), market.base_mint.as_ref()], bump)]
    pub base_vault: Account<'info, TokenAccount>,
    #[account(mut, seeds = [b"MarketVault", market.key().as_ref(), market.quote_mint.as_ref()], bump)]
    pub quote_vault: Account<'info, TokenAccount>,
    pub token_program: Program<'info, Token>,
}

pub fn withdraw(ctx: Context<Withdraw>, amount: u64, side: TokenSide) -> Result<()> {
    require!(amount > 0, LimitOrderError::InvalidAmount);
    let margin = &mut ctx.accounts.margin;
    let market = &ctx.accounts.market;
    let bump = market.bump;
    let seeds = &[b"LimitOrderMarket", market.base_mint.as_ref(), market.quote_mint.as_ref(), &[bump]];
    let signer = &[&seeds[..]];
    match side {
        TokenSide::Base => {
            require!(margin.base_free >= amount, LimitOrderError::InsufficientFreeBalance);
            margin.base_free = margin.base_free.checked_sub(amount).ok_or(LimitOrderError::MathOverflow)?;
            let cpi_accounts = Transfer {
                from: ctx.accounts.base_vault.to_account_info(),
                to: ctx.accounts.user_base_token.to_account_info(),
                authority: ctx.accounts.market.to_account_info(),
            };
            let cpi_ctx =
                CpiContext::new_with_signer(ctx.accounts.token_program.to_account_info(), cpi_accounts, signer);
            token::transfer(cpi_ctx, amount)?;
        }
        TokenSide::Quote => {
            require!(margin.quote_free >= amount, LimitOrderError::InsufficientFreeBalance);
            margin.quote_free = margin.quote_free.checked_sub(amount).ok_or(LimitOrderError::MathOverflow)?;
            let cpi_accounts = Transfer {
                from: ctx.accounts.quote_vault.to_account_info(),
                to: ctx.accounts.user_quote_token.to_account_info(),
                authority: ctx.accounts.market.to_account_info(),
            };
            let cpi_ctx =
                CpiContext::new_with_signer(ctx.accounts.token_program.to_account_info(), cpi_accounts, signer);
            token::transfer(cpi_ctx, amount)?;
        }
    }
    emit!(WithdrawEvent {
        market: market.key(),
        owner: margin.owner,
        side,
        amount,
    });
    Ok(())
}
