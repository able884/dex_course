use anchor_lang::prelude::*;
use crate::state::config::ProgramConfig;
use crate::state::market::{Market, PriceOracleType};
use anchor_spl::token::{Mint, Token, TokenAccount};
use crate::error::LimitOrderError;
use crate::events::MarketParamUpdated;

#[derive(Accounts)]
pub struct InitMarket<'info> {
    #[account(seeds = [b"config"], bump = config.bump)]
    pub config: Account<'info, ProgramConfig>,
    #[account(mut)]
    pub admin: Signer<'info>,
    #[account(
        init,
        seeds = [b"LimitOrderMarket", base_mint.key().as_ref(), quote_mint.key().as_ref()],
        bump,
        payer = admin,
        space = 8 + Market::SIZE
    )]
    pub market: Account<'info, Market>,
    pub base_mint: Account<'info, Mint>,
    pub quote_mint: Account<'info, Mint>,
    #[account(
        init,
        seeds = [b"MarketVault", market.key().as_ref(), base_mint.key().as_ref()],
        bump,
        payer = admin,
        token::mint = base_mint,
        token::authority = market
    )]
    pub base_vault: Account<'info, TokenAccount>,
    #[account(
        init,
        seeds = [b"MarketVault", market.key().as_ref(), quote_mint.key().as_ref()],
        bump,
        payer = admin,
        token::mint = quote_mint,
        token::authority = market
    )]
    pub quote_vault: Account<'info, TokenAccount>,
    pub token_program: Program<'info, Token>,
    pub system_program: Program<'info, System>,
}

#[derive(AnchorSerialize, AnchorDeserialize)]
pub struct InitMarketParams {
    pub tick_size: u64,
    pub min_base_lot: u64,
    pub min_quote_lot: u64,
    pub maker_fee_bps: u16,
    pub taker_fee_bps: u16,
    pub oracle_type: PriceOracleType,
    pub pyth_price_feed_id: [u8; 32],  // Only used if oracle_type == Pyth
}

pub fn init_market(ctx: Context<InitMarket>, params: InitMarketParams) -> Result<()> {
    let config = &ctx.accounts.config;
    require!(ctx.accounts.admin.key() == config.admin, LimitOrderError::Unauthorized);
    require!(params.taker_fee_bps as u16 <= config.fee_bps_limit, LimitOrderError::InvalidFeeBps);
    require!(params.maker_fee_bps <= config.fee_bps_limit, LimitOrderError::InvalidFeeBps);

    // Validate market parameters
    require!(params.tick_size > 0, LimitOrderError::InvalidTick);
    require!(params.min_base_lot > 0, LimitOrderError::InvalidAmount);
    require!(params.min_quote_lot > 0, LimitOrderError::InvalidAmount);
    require!(
        ctx.accounts.base_mint.key() != ctx.accounts.quote_mint.key(),
        LimitOrderError::InvalidAmount
    );

    // Vaults are automatically initialized by init constraint
    // Verify vaults are properly set up
    require!(
        ctx.accounts.base_vault.mint == ctx.accounts.base_mint.key(),
        LimitOrderError::InvalidAmount
    );
    require!(
        ctx.accounts.quote_vault.mint == ctx.accounts.quote_mint.key(),
        LimitOrderError::InvalidAmount
    );
    require!(
        ctx.accounts.base_vault.owner == ctx.accounts.market.key(),
        LimitOrderError::Unauthorized
    );
    require!(
        ctx.accounts.quote_vault.owner == ctx.accounts.market.key(),
        LimitOrderError::Unauthorized
    );

    let market = &mut ctx.accounts.market;
    market.base_mint = ctx.accounts.base_mint.key();
    market.quote_mint = ctx.accounts.quote_mint.key();
    market.base_vault = ctx.accounts.base_vault.key();
    market.quote_vault = ctx.accounts.quote_vault.key();
    market.tick_size = params.tick_size;
    market.min_base_lot = params.min_base_lot;
    market.min_quote_lot = params.min_quote_lot;
    market.maker_fee_bps = params.maker_fee_bps;
    market.taker_fee_bps = params.taker_fee_bps;
    market.fee_accumulator = 0;
    market.seq_num = 0;

    // Set price oracle configuration
    market.oracle_type = params.oracle_type;
    market.pyth_price_feed_id = params.pyth_price_feed_id;

    // Initialize custom price fields (will be set later via update_custom_price if needed)
    market.custom_price = 0;
    market.custom_price_exponent = 0;
    market.custom_price_conf = 0;
    market.custom_price_slot = 0;

    match params.oracle_type {
        PriceOracleType::None => {
            msg!("Market created without price verification");
        }
        PriceOracleType::Pyth => {
            require!(
                params.pyth_price_feed_id != [0u8; 32],
                LimitOrderError::InvalidPythAccount
            );
            msg!("Market created with Pyth price feed ID: {:?}", params.pyth_price_feed_id);
        }
        PriceOracleType::Custom => {
            msg!("Market created with custom price oracle - admin must set initial price");
        }
    }

    market.paused = false;
    market.bump = ctx.bumps.market;
    emit!(MarketParamUpdated {
        market: market.key(),
        maker_fee_bps: market.maker_fee_bps,
        taker_fee_bps: market.taker_fee_bps,
        tick_size: market.tick_size,
        min_base_lot: market.min_base_lot,
        min_quote_lot: market.min_quote_lot,
        paused: market.paused,
    });
    Ok(())
}
