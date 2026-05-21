use anchor_lang::prelude::*;
use crate::state::config::ProgramConfig;
use crate::state::market::Market;
use crate::state::margin::UserMargin;
use crate::state::order::{Order, Side, SelfTradeBehavior, LockedSide, OrderStatus};
use crate::error::LimitOrderError;
use crate::state::config::verify_whitelist;
use crate::events::OrderPlaced;

#[derive(Accounts)]
pub struct PlaceOrder<'info> {
    pub config: Account<'info, ProgramConfig>,
    #[account(mut, seeds = [b"LimitOrderMarket", market.base_mint.as_ref(), market.quote_mint.as_ref()], bump = market.bump)]
    pub market: Account<'info, Market>,
    #[account(mut, seeds = [b"margin", market.key().as_ref(), owner.key().as_ref()], bump = margin.bump)]
    pub margin: Account<'info, UserMargin>,
    #[account(
        init,
        seeds = [b"order", market.key().as_ref(), owner.key().as_ref(), &market.seq_num.checked_add(1).unwrap_or(0).to_le_bytes()],
        bump,
        payer = owner,
        space = 8 + Order::SIZE
    )]
    pub order: Account<'info, Order>,
    #[account(mut)]
    pub owner: Signer<'info>,
    pub system_program: Program<'info, System>,
}

#[derive(AnchorSerialize, AnchorDeserialize)]
pub struct PlaceOrderParams {
    pub side: Side,
    pub price_lots: u64,
    pub qty_lots: u64,
    pub expiry_slot: u64,
    pub min_fill_bps: Option<u16>,
    pub self_trade_behavior: SelfTradeBehavior,
}

pub fn place_order(ctx: Context<PlaceOrder>, params: PlaceOrderParams, whitelist_proof: Vec<[u8; 32]>) -> Result<()> {
    let config = &ctx.accounts.config;
    require!(!config.paused, LimitOrderError::ProgramPaused);
    let market = &mut ctx.accounts.market;
    require!(!market.paused, LimitOrderError::MarketPaused);
    require!(params.qty_lots > 0, LimitOrderError::InvalidAmount);
    require!(params.price_lots > 0, LimitOrderError::InvalidAmount);
    require!(params.price_lots % market.tick_size == 0, LimitOrderError::InvalidTick);

    verify_whitelist(config, ctx.accounts.owner.key(), &whitelist_proof)?;
    let margin = &mut ctx.accounts.margin;
    let order = &mut ctx.accounts.order;
    let order_id = market
        .seq_num
        .checked_add(1)
        .ok_or(LimitOrderError::MathOverflow)?;
    market.seq_num = order_id;
    match params.side {
        Side::Bid => {
            let quote_needed = params
                .price_lots
                .checked_mul(params.qty_lots)
                .ok_or(LimitOrderError::MathOverflow)?;
            require!(params.qty_lots >= market.min_base_lot, LimitOrderError::InvalidAmount);
            require!(margin.quote_free >= quote_needed, LimitOrderError::InsufficientFreeBalance);
            margin.quote_free = margin.quote_free.checked_sub(quote_needed).ok_or(LimitOrderError::MathOverflow)?;
            margin.quote_locked =
                margin.quote_locked.checked_add(quote_needed).ok_or(LimitOrderError::MathOverflow)?;
            order.locked_side = LockedSide::Quote;
        }
        Side::Ask => {
            require!(params.qty_lots >= market.min_base_lot, LimitOrderError::InvalidAmount);
            require!(margin.base_free >= params.qty_lots, LimitOrderError::InsufficientFreeBalance);
            margin.base_free = margin.base_free.checked_sub(params.qty_lots).ok_or(LimitOrderError::MathOverflow)?;
            margin.base_locked =
                margin.base_locked.checked_add(params.qty_lots).ok_or(LimitOrderError::MathOverflow)?;
            order.locked_side = LockedSide::Base;
        }
    }
    order.owner = margin.owner;
    order.market = market.key();
    order.side = params.side;
    order.price_lots = params.price_lots;
    order.qty_lots = params.qty_lots;
    order.remaining_lots = params.qty_lots;
    order.expiry_slot = params.expiry_slot;
    order.order_id = order_id;
    order.min_fill_bps = params.min_fill_bps;
    order.self_trade_behavior = params.self_trade_behavior;
    order.status = OrderStatus::Active;
    order.bump = ctx.bumps.order;
    emit!(OrderPlaced {
        market: market.key(),
        owner: margin.owner,
        order_id,
        side: params.side,
        price_lots: params.price_lots,
        qty_lots: params.qty_lots,
        remaining_lots: params.qty_lots,
        expiry_slot: params.expiry_slot,
    });
    Ok(())
}
