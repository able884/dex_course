use anchor_lang::prelude::*;
use crate::state::config::ProgramConfig;
use crate::state::market::Market;
use crate::state::margin::UserMargin;
use crate::error::LimitOrderError;
use anchor_spl::token::{Mint, Token, TokenAccount};
use anchor_spl::token::{self, Transfer};
use crate::state::config::verify_whitelist;
use crate::state::market::TokenSide;
use crate::events::DepositEvent;

#[derive(Accounts)]
pub struct Deposit<'info> {
    #[account(seeds = [b"config"], bump = config.bump)]
    pub config: Account<'info, ProgramConfig>,
    #[account(seeds = [b"LimitOrderMarket", market.base_mint.as_ref(), market.quote_mint.as_ref()], bump = market.bump)]
    pub market: Account<'info, Market>,
    #[account(mut, seeds = [b"margin", market.key().as_ref(), owner.key().as_ref()], bump = margin.bump)]
    pub margin: Account<'info, UserMargin>,
    #[account(mut)]
    pub owner: Signer<'info>,
    #[account(constraint = base_mint.key() == market.base_mint @ LimitOrderError::InvalidAmount)]
    pub base_mint: Account<'info, Mint>,
    #[account(constraint = quote_mint.key() == market.quote_mint @ LimitOrderError::InvalidAmount)]
    pub quote_mint: Account<'info, Mint>,
    #[account(mut, constraint = user_base_token.mint == base_mint.key())]
    pub user_base_token: Account<'info, TokenAccount>,
    #[account(mut, constraint = user_quote_token.mint == quote_mint.key())]
    pub user_quote_token: Account<'info, TokenAccount>,
    #[account(mut, seeds = [b"MarketVault", market.key().as_ref(), base_mint.key().as_ref()], bump)]
    pub base_vault: Account<'info, TokenAccount>,
    #[account(mut, seeds = [b"MarketVault", market.key().as_ref(), quote_mint.key().as_ref()], bump)]
    pub quote_vault: Account<'info, TokenAccount>,
    pub token_program: Program<'info, Token>,
}

/// 用户充值，用户ATA账户 -> 向市场金库转移代币，并更新保证金账户余额
pub fn deposit(ctx: Context<Deposit>, amount: u64, side: TokenSide, whitelist_proof: Vec<[u8; 32]>) -> Result<()> {
    let config = &ctx.accounts.config;
    require!(!config.paused, LimitOrderError::ProgramPaused);
    require!(!ctx.accounts.market.paused, LimitOrderError::MarketPaused);
    require!(amount > 0, LimitOrderError::InvalidAmount);

    verify_whitelist(config, ctx.accounts.owner.key(), &whitelist_proof)?;

    // 根据side判断如果side是base，则从用户基础代币ATA账户 -> 市场金库
    // 如果side是quote，则从用户报价代币ATA账户 -> 市场金库
    let (source, dest) = match side {
        TokenSide::Base => (&ctx.accounts.user_base_token, &ctx.accounts.base_vault),
        TokenSide::Quote => (&ctx.accounts.user_quote_token, &ctx.accounts.quote_vault),
    };

    let cpi_accounts = Transfer {
        from: source.to_account_info(),
        to: dest.to_account_info(),
        authority: ctx.accounts.owner.to_account_info(),
    };
    let cpi_ctx = CpiContext::new(ctx.accounts.token_program.to_account_info(), cpi_accounts);
    token::transfer(cpi_ctx, amount)?;

    let margin = &mut ctx.accounts.margin;
    match side {
        TokenSide::Base => {
            // 充值基础代币，增加保证金账户的基础可用余额
            margin.base_free = margin.base_free.checked_add(amount).ok_or(LimitOrderError::MathOverflow)?;
        }
        TokenSide::Quote => {
            // 充值报价代币，增加保证金账户的报价可用余额
            margin.quote_free = margin.quote_free.checked_add(amount).ok_or(LimitOrderError::MathOverflow)?;
        }
    }
    // 发出充值事件
    emit!(DepositEvent {
        market: ctx.accounts.market.key(),
        owner: margin.owner,
        side,
        amount,
    });
    Ok(())
}
