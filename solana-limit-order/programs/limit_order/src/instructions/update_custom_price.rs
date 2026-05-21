use anchor_lang::prelude::*;
use crate::state::config::ProgramConfig;
use crate::state::market::Market;
use crate::error::LimitOrderError;
use crate::events::CustomPriceUpdated;
use anchor_lang::solana_program::clock::Clock;
use crate::state::market::PriceOracleType;

#[derive(Accounts)]
pub struct UpdateCustomPrice<'info> {
    #[account(seeds = [b"config"], bump = config.bump)]
    pub config: Account<'info, ProgramConfig>,
    pub admin: Signer<'info>,
    #[account(mut, seeds = [b"LimitOrderMarket", market.base_mint.as_ref(), market.quote_mint.as_ref()], bump = market.bump)]
    pub market: Account<'info, Market>,
}

pub fn update_custom_price(
    ctx: Context<UpdateCustomPrice>,
    price: i64,
    exponent: i32,
    conf: u64,
) -> Result<()> {
    let config = &ctx.accounts.config;
    require!(ctx.accounts.admin.key() == config.admin, LimitOrderError::Unauthorized);

    let market = &mut ctx.accounts.market;
    require!(
        market.oracle_type == PriceOracleType::Custom,
        LimitOrderError::InvalidOracleType
    );

    // Validate price parameters
    require!(price > 0, LimitOrderError::InvalidPythPrice);
    require!(exponent >= -12 && exponent <= 0, LimitOrderError::InvalidPriceExponent);

    // Update custom price
    market.custom_price = price;
    market.custom_price_exponent = exponent;
    market.custom_price_conf = conf;
    market.custom_price_slot = Clock::get()?.slot;

    emit!(CustomPriceUpdated {
        market: market.key(),
        price,
        exponent,
        conf,
        slot: market.custom_price_slot,
    });

    msg!(
        "Custom price updated: price={}, exponent={}, conf={}, slot={}",
        price,
        exponent,
        conf,
        market.custom_price_slot
    );

    Ok(())
}
