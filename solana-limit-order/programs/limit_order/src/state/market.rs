use anchor_lang::prelude::*;
use crate::state::config::ProgramConfig;
use crate::error::LimitOrderError;
use pyth_solana_receiver_sdk::price_update::PriceUpdateV2;
use anchor_lang::solana_program::clock::Clock;

#[account]
pub struct Market {
    pub base_mint: Pubkey,
    pub quote_mint: Pubkey,
    pub base_vault: Pubkey,
    pub quote_vault: Pubkey,
    pub tick_size: u64,
    pub min_base_lot: u64,
    pub min_quote_lot: u64,
    pub maker_fee_bps: u16,
    pub taker_fee_bps: u16,
    pub fee_accumulator: u64,
    pub seq_num: u64,

    // Price oracle configuration
    pub oracle_type: PriceOracleType,
    pub pyth_price_feed_id: [u8; 32],  // Pyth price feed ID (only used if oracle_type == Pyth)

    // Custom price oracle (only used if oracle_type == Custom)
    pub custom_price: i64,              // Price with exponent
    pub custom_price_exponent: i32,     // Price exponent (e.g., -8 means price * 10^-8)
    pub custom_price_conf: u64,         // Price confidence interval
    pub custom_price_slot: u64,         // Slot when price was last updated

    pub paused: bool,
    pub bump: u8,
}

impl Market {
    // 32*4 (pubkeys) + 8*5 (u64s) + 2+2 (fees) + 1 (oracle_type) + 32 (pyth_feed_id)
    // + 8 (custom_price) + 4 (exponent) + 8 (conf) + 8 (slot) + 1 (paused) + 1 (bump)
    pub const SIZE: usize = 32 * 4 + 8 * 5 + 2 + 2 + 1 + 32 + 8 + 4 + 8 + 8 + 1 + 1;
}

#[derive(AnchorSerialize, AnchorDeserialize, Clone, Copy, PartialEq, Eq)]
pub enum PriceOracleType {
    None,           // No price verification
    Pyth,           // Use Pyth price feed
    Custom,         // Use custom maintained price
}

#[derive(AnchorSerialize, AnchorDeserialize, Clone, Copy, PartialEq, Eq)]
pub enum TokenSide {
    Base,
    Quote,
}

pub fn verify_pyth_price(
    config: &ProgramConfig,
    pyth_price_account: &AccountInfo,
    feed_id: &[u8; 32],
    _match_price_lots: u64,
    _slot: u64,
) -> Result<()> {
    // Load and parse the Pyth price update account
    let price_update = PriceUpdateV2::try_deserialize(&mut &pyth_price_account.data.borrow()[..])
        .map_err(|_| LimitOrderError::InvalidPythAccount)?;

    // Get the price feed data with staleness check
    let price_feed = price_update
        .get_price_no_older_than(&Clock::get()?, config.pyth_max_staleness_slots, feed_id)
        .map_err(|_| LimitOrderError::PythPriceStale)?;

    // Validate price is positive
    require!(price_feed.price > 0, LimitOrderError::InvalidPythPrice);

    // Check confidence interval (confidence should be small relative to price)
    // confidence_bps = (conf * 10000) / price
    let confidence_bps = price_feed
        .conf
        .checked_mul(10000)
        .ok_or(LimitOrderError::MathOverflow)?
        .checked_div(price_feed.price.abs() as u64)
        .ok_or(LimitOrderError::MathOverflow)?;

    require!(
        confidence_bps <= config.pyth_confidence_tol_bps as u64,
        LimitOrderError::PythConfidenceTooWide
    );

    // Optional: Validate match price is within reasonable range of oracle price
    // This is a circuit breaker to prevent trades at manipulated prices
    // For example, match price should be within ±10% of oracle price
    // Note: This requires converting price_lots to the same scale as Pyth price
    // Implementation depends on your lot size and price decimals
    // Uncomment and adjust if needed:
    /*
    let max_deviation_bps = 1000; // 10%
    let oracle_price_scaled = scale_pyth_price_to_lots(price_feed.price, price_feed.exponent);
    let price_diff = if match_price_lots > oracle_price_scaled {
        match_price_lots - oracle_price_scaled
    } else {
        oracle_price_scaled - match_price_lots
    };
    let deviation_bps = price_diff
        .checked_mul(10000)
        .ok_or(LimitOrderError::MathOverflow)?
        / oracle_price_scaled;
    require!(
        deviation_bps <= max_deviation_bps,
        LimitOrderError::PriceOutsideRange
    );
    */

    msg!(
        "Pyth price verified: price={}, conf={}, expo={}, timestamp={}",
        price_feed.price,
        price_feed.conf,
        price_feed.exponent,
        price_feed.publish_time
    );

    Ok(())
}

pub fn verify_custom_price(
    config: &ProgramConfig,
    market: &Market,
    _match_price_lots: u64,
    slot: u64,
) -> Result<()> {
    // Validate custom price has been set and is positive
    require!(
        market.custom_price > 0,
        LimitOrderError::CustomPriceNotSet
    );

    // Check price staleness
    let price_age = slot
        .checked_sub(market.custom_price_slot)
        .ok_or(LimitOrderError::MathOverflow)?;

    require!(
        price_age <= config.pyth_max_staleness_slots,
        LimitOrderError::PythPriceStale
    );

    // Check confidence interval (same logic as Pyth)
    // Safe division: we already checked custom_price > 0
    let price_abs = market.custom_price.abs() as u64;
    require!(price_abs > 0, LimitOrderError::InvalidPythPrice);  // Extra safety check

    let confidence_bps = market
        .custom_price_conf
        .checked_mul(10000)
        .ok_or(LimitOrderError::MathOverflow)?
        .checked_div(price_abs)
        .ok_or(LimitOrderError::MathOverflow)?;

    require!(
        confidence_bps <= config.pyth_confidence_tol_bps as u64,
        LimitOrderError::PythConfidenceTooWide
    );

    msg!(
        "Custom price verified: price={}, exponent={}, conf={}, age={} slots",
        market.custom_price,
        market.custom_price_exponent,
        market.custom_price_conf,
        price_age
    );

    Ok(())
}
