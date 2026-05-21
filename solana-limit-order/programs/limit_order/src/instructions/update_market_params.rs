use anchor_lang::prelude::*;
use crate::state::config::ProgramConfig;
use crate::state::market::Market;
use crate::error::LimitOrderError;
use crate::events::MarketParamUpdated;

#[derive(Accounts)]
pub struct UpdateMarketParams<'info> {
    #[account(seeds = [b"config"], bump = config.bump)]
    pub config: Account<'info, ProgramConfig>,
    pub admin: Signer<'info>,
    #[account(mut)]
    pub market: Account<'info, Market>,
}

#[derive(AnchorSerialize, AnchorDeserialize)]
pub struct UpdateMarketParamsData {
    pub tick_size: Option<u64>,
    pub min_base_lot: Option<u64>,
    pub min_quote_lot: Option<u64>,
    pub maker_fee_bps: Option<u16>,
    pub taker_fee_bps: Option<u16>,
    pub pyth_price_feed_id: Option<[u8; 32]>,
    pub paused: Option<bool>,
}

pub fn update_market_params(
    ctx: Context<UpdateMarketParams>,
    params: UpdateMarketParamsData,
) -> Result<()> {
    let config = &ctx.accounts.config;
    require!(ctx.accounts.admin.key() == config.admin, LimitOrderError::Unauthorized);
    let market = &mut ctx.accounts.market;
    if let Some(taker_fee_bps) = params.taker_fee_bps {
        require!(taker_fee_bps <= config.fee_bps_limit, LimitOrderError::InvalidFeeBps);
        market.taker_fee_bps = taker_fee_bps;
    }
    if let Some(maker_fee_bps) = params.maker_fee_bps {
        require!(maker_fee_bps <= config.fee_bps_limit, LimitOrderError::InvalidFeeBps);
        market.maker_fee_bps = maker_fee_bps;
    }
    if let Some(tick) = params.tick_size {
        market.tick_size = tick;
    }
    if let Some(min_base_lot) = params.min_base_lot {
        market.min_base_lot = min_base_lot;
    }
    if let Some(min_quote_lot) = params.min_quote_lot {
        market.min_quote_lot = min_quote_lot;
    }
    if let Some(feed_id) = params.pyth_price_feed_id {
        market.pyth_price_feed_id = feed_id;
    }
    if let Some(paused) = params.paused {
        market.paused = paused;
    }
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
