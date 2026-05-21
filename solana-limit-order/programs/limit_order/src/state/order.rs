use anchor_lang::prelude::*;
use crate::state::margin::UserMargin;
use crate::state::market::Market;
use crate::error::LimitOrderError;
use crate::constants::BPS_DENOM;

#[account]
pub struct Order {
    pub owner: Pubkey,
    pub market: Pubkey,
    pub side: Side,
    pub price_lots: u64,
    pub qty_lots: u64,
    pub remaining_lots: u64,
    pub expiry_slot: u64,
    pub order_id: u64,
    pub min_fill_bps: Option<u16>,
    pub self_trade_behavior: SelfTradeBehavior,
    pub locked_side: LockedSide,
    pub status: OrderStatus,
    pub bump: u8,
}

impl Order {
    pub const SIZE: usize = 32 * 2 + 1 + 8 * 5 + 3 + 4;
}
#[derive(AnchorSerialize, AnchorDeserialize, Clone, Copy, PartialEq, Eq)]
pub enum Side {
    Bid,
    Ask,
}

#[derive(AnchorSerialize, AnchorDeserialize, Clone, Copy, PartialEq, Eq)]
pub enum LockedSide {
    Base,
    Quote,
}

#[derive(AnchorSerialize, AnchorDeserialize, Clone, Copy, PartialEq, Eq)]
pub enum OrderStatus {
    Active,
    Filled,
    Cancelled,
    Expired,
}

#[derive(AnchorSerialize, AnchorDeserialize, Clone, Copy, PartialEq, Eq)]
pub enum SelfTradeBehavior {
    DecrementTake,
    CancelNew,
}

pub fn price_crossed(maker: &Order, taker: &Order) -> bool {
    match taker.side {
        Side::Bid => taker.price_lots >= maker.price_lots,
        Side::Ask => maker.price_lots >= taker.price_lots,
    }
}

pub fn unlock_order_funds(margin: &mut UserMargin, order: &Order) -> Result<()> {
    match order.locked_side {
        LockedSide::Base => {
            margin.base_locked = margin.base_locked.checked_sub(order.remaining_lots).ok_or(LimitOrderError::MathOverflow)?;
            margin.base_free = margin.base_free.checked_add(order.remaining_lots).ok_or(LimitOrderError::MathOverflow)?;
        }
        LockedSide::Quote => {
            let quote_locked = order
                .price_lots
                .checked_mul(order.remaining_lots)
                .ok_or(LimitOrderError::MathOverflow)?;
            margin.quote_locked = margin.quote_locked.checked_sub(quote_locked).ok_or(LimitOrderError::MathOverflow)?;
            margin.quote_free = margin.quote_free.checked_add(quote_locked).ok_or(LimitOrderError::MathOverflow)?;
        }
    }
    Ok(())
}

pub fn settle_match(
    market: &mut Market,
    maker_margin: &mut UserMargin,
    taker_margin: &mut UserMargin,
    maker_order: &mut Order,
    taker_order: &mut Order,
    qty: u64,
) -> Result<()> {
    let price = maker_order.price_lots;
    let quote_amount = qty.checked_mul(price).ok_or(LimitOrderError::MathOverflow)?;

    // Calculate fee with proper rounding (round up to ensure protocol doesn't lose fees)
    let fee_numerator = quote_amount
        .checked_mul(market.taker_fee_bps as u64)
        .ok_or(LimitOrderError::MathOverflow)?;
    let fee = fee_numerator
        .checked_add(BPS_DENOM - 1)  // Add (denominator - 1) for ceiling division
        .ok_or(LimitOrderError::MathOverflow)?
        / BPS_DENOM;

    match taker_order.side {
        Side::Bid => {
            maker_margin.base_locked = maker_margin.base_locked.checked_sub(qty).ok_or(LimitOrderError::MathOverflow)?;
            taker_margin.quote_locked = taker_margin.quote_locked.checked_sub(quote_amount).ok_or(LimitOrderError::MathOverflow)?;
            taker_margin.base_free = taker_margin.base_free.checked_add(qty).ok_or(LimitOrderError::MathOverflow)?;
            maker_margin.quote_free = maker_margin.quote_free.checked_add(quote_amount.checked_sub(fee).ok_or(LimitOrderError::MathOverflow)?).ok_or(LimitOrderError::MathOverflow)?;
        }
        Side::Ask => {
            taker_margin.base_locked = taker_margin.base_locked.checked_sub(qty).ok_or(LimitOrderError::MathOverflow)?;
            maker_margin.quote_locked = maker_margin.quote_locked.checked_sub(quote_amount).ok_or(LimitOrderError::MathOverflow)?;
            maker_margin.base_free = maker_margin.base_free.checked_add(qty).ok_or(LimitOrderError::MathOverflow)?;
            taker_margin.quote_free = taker_margin.quote_free.checked_add(quote_amount.checked_sub(fee).ok_or(LimitOrderError::MathOverflow)?).ok_or(LimitOrderError::MathOverflow)?;
        }
    }
    maker_order.remaining_lots = maker_order.remaining_lots.checked_sub(qty).ok_or(LimitOrderError::MathOverflow)?;
    taker_order.remaining_lots = taker_order.remaining_lots.checked_sub(qty).ok_or(LimitOrderError::MathOverflow)?;
    if maker_order.remaining_lots == 0 {
        maker_order.status = OrderStatus::Filled;
    }
    if taker_order.remaining_lots == 0 {
        taker_order.status = OrderStatus::Filled;
    }
    market.fee_accumulator = market.fee_accumulator.checked_add(fee).ok_or(LimitOrderError::MathOverflow)?;
    Ok(())
}
