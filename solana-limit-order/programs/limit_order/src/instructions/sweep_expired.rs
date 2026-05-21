use anchor_lang::prelude::*;
use crate::state::config::ProgramConfig;
use crate::state::market::Market;
use crate::state::margin::UserMargin;
use crate::state::order::Order;
use crate::error::LimitOrderError;
use crate::events::OrderExpired;
use crate::state::order::unlock_order_funds;
use crate::state::order::OrderStatus;

#[derive(Accounts)]
pub struct SweepExpired<'info> {
    pub config: Account<'info, ProgramConfig>,
    #[account(seeds = [b"LimitOrderMarket", market.base_mint.as_ref(), market.quote_mint.as_ref()], bump = market.bump)]
    pub market: Account<'info, Market>,
    #[account(mut, seeds = [b"margin", market.key().as_ref(), order.owner.as_ref()], bump = margin.bump)]
    pub margin: Account<'info, UserMargin>,
    #[account(
        mut,
        constraint = order.market == market.key() @ LimitOrderError::InvalidAmount,
        constraint = order.owner == margin.owner @ LimitOrderError::Unauthorized
    )]
    pub order: Account<'info, Order>,
}

pub fn sweep_expired(ctx: Context<SweepExpired>) -> Result<()> {
    let slot = Clock::get()?.slot;
    let order = &mut ctx.accounts.order;
    require!(order.status == OrderStatus::Active, LimitOrderError::OrderNotActive);

    // Check if order is expired: expiry_slot == 0 means never expires
    require!(
        order.expiry_slot > 0 && slot > order.expiry_slot,
        LimitOrderError::OrderNotExpired
    );

    unlock_order_funds(&mut ctx.accounts.margin, order)?;
    order.status = OrderStatus::Expired;
    emit!(OrderExpired {
        market: order.market,
        owner: order.owner,
        order_id: order.order_id,
        remaining_lots: order.remaining_lots,
    });
    Ok(())
}
