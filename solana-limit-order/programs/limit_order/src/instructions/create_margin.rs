use anchor_lang::prelude::*;
use crate::state::config::ProgramConfig;
use crate::state::market::Market;
use crate::state::margin::UserMargin;
use crate::error::LimitOrderError;
use anchor_spl::token::{Mint, Token, TokenAccount};
use anchor_spl::associated_token::AssociatedToken;
use crate::state::config::verify_whitelist;

#[derive(Accounts)]
pub struct CreateMargin<'info> {
    #[account(seeds = [b"config"], bump = config.bump)]
    pub config: Account<'info, ProgramConfig>,
    #[account(seeds = [b"LimitOrderMarket", market.base_mint.as_ref(), market.quote_mint.as_ref()], bump = market.bump)]
    pub market: Account<'info, Market>,
    #[account(
        init,
        seeds = [b"margin", market.key().as_ref(), owner.key().as_ref()],
        bump,
        payer = owner,
        space = 8 + UserMargin::SIZE
    )]
    pub margin: Account<'info, UserMargin>,
    #[account(mut)]
    pub owner: Signer<'info>,
    #[account(constraint = base_mint.key() == market.base_mint @ LimitOrderError::InvalidAmount)]
    pub base_mint: Account<'info, Mint>,
    #[account(constraint = quote_mint.key() == market.quote_mint @ LimitOrderError::InvalidAmount)]
    pub quote_mint: Account<'info, Mint>,
    #[account(
        init_if_needed,
        payer = owner,
        associated_token::mint = base_mint,
        associated_token::authority = owner
    )]
    pub user_base_token: Account<'info, TokenAccount>,
    #[account(
        init_if_needed,
        payer = owner,
        associated_token::mint = quote_mint,
        associated_token::authority = owner
    )]
    pub user_quote_token: Account<'info, TokenAccount>,
    pub token_program: Program<'info, Token>,
    pub associated_token_program: Program<'info, AssociatedToken>,
    pub system_program: Program<'info, System>,
}

pub fn create_margin(ctx: Context<CreateMargin>, whitelist_proof: Vec<[u8; 32]>) -> Result<()> {
    let config = &ctx.accounts.config;
    require!(!config.paused, LimitOrderError::ProgramPaused);
    require!(!ctx.accounts.market.paused, LimitOrderError::MarketPaused);

    verify_whitelist(config, ctx.accounts.owner.key(), &whitelist_proof)?;

    let margin = &mut ctx.accounts.margin;
    margin.owner = ctx.accounts.owner.key();
    margin.market = ctx.accounts.market.key();
    margin.base_free = 0;
    margin.quote_free = 0;
    margin.base_locked = 0;
    margin.quote_locked = 0;
    margin.bump = ctx.bumps.margin;
    Ok(())
}
