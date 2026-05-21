use anchor_lang::prelude::*;
use crate::state::order::Side;
use crate::state::market::TokenSide;

#[event]
pub struct OrderPlaced {
    pub market: Pubkey,
    pub owner: Pubkey,
    pub order_id: u64,
    pub side: Side,
    pub price_lots: u64,
    pub qty_lots: u64,
    pub remaining_lots: u64,
    pub expiry_slot: u64,
}

#[event]
pub struct OrderCancelled {
    pub market: Pubkey,
    pub owner: Pubkey,
    pub order_id: u64,
    pub remaining_lots: u64,
}

#[event]
pub struct OrderExpired {
    pub market: Pubkey,
    pub owner: Pubkey,
    pub order_id: u64,
    pub remaining_lots: u64,
}

#[event]
pub struct TradeEvent {
    pub market: Pubkey,
    pub maker: Pubkey,
    pub taker: Pubkey,
    pub maker_order_id: u64,
    pub taker_order_id: u64,
    pub qty_lots: u64,
    pub price_lots: u64,
    pub fee: u64,
}

#[event]
pub struct FeeTransfer {
    pub market: Pubkey,
    pub to: Pubkey,
    pub amount: u64,
}

#[event]
pub struct MarketParamUpdated {
    pub market: Pubkey,
    pub maker_fee_bps: u16,
    pub taker_fee_bps: u16,
    pub tick_size: u64,
    pub min_base_lot: u64,
    pub min_quote_lot: u64,
    pub paused: bool,
}

#[event]
pub struct ProgramParamUpdated {
    pub admin: Pubkey,
    pub treasury: Pubkey,
    pub whitelist_root: [u8; 32],
    pub whitelist_version: u64,
    pub fee_bps_limit: u16,
    pub paused: bool,
}

#[event]
pub struct EmergencyUnlock {
    pub market: Pubkey,
    pub owner: Pubkey,
    pub order_id: u64,
    pub remaining_lots: u64,
}

#[event]
pub struct DepositEvent {
    pub market: Pubkey,
    pub owner: Pubkey,
    pub side: TokenSide,
    pub amount: u64,
}

#[event]
pub struct WithdrawEvent {
    pub market: Pubkey,
    pub owner: Pubkey,
    pub side: TokenSide,
    pub amount: u64,
}

#[event]
pub struct CustomPriceUpdated {
    pub market: Pubkey,
    pub price: i64,
    pub exponent: i32,
    pub conf: u64,
    pub slot: u64,
}
