use anchor_lang::prelude::*;
use crate::state::config::ProgramConfig;
use crate::state::market::Market;
use crate::state::margin::UserMargin;
use crate::state::order::{Order, OrderStatus};
use crate::error::LimitOrderError;
use crate::events::OrderCancelled;
use crate::state::order::unlock_order_funds;

#[derive(Accounts)]
pub struct CancelOrder<'info> {
    pub config: Account<'info, ProgramConfig>,
    #[account(seeds = [b"LimitOrderMarket", market.base_mint.as_ref(), market.quote_mint.as_ref()], bump = market.bump)]
    pub market: Account<'info, Market>,
    #[account(mut, seeds = [b"margin", market.key().as_ref(), owner.key().as_ref()], bump = margin.bump)]
    pub margin: Account<'info, UserMargin>,
    #[account(
        mut,
        constraint = order.market == market.key() @ LimitOrderError::InvalidAmount,
        constraint = order.owner == owner.key() @ LimitOrderError::Unauthorized
    )]
    pub order: Account<'info, Order>,
    pub owner: Signer<'info>,
}

/// 取消订单指令处理函数
pub fn cancel_order(ctx: Context<CancelOrder>) -> Result<()> {
    let margin = &mut ctx.accounts.margin;
    let order = &mut ctx.accounts.order;
    require!(order.status == OrderStatus::Active, LimitOrderError::OrderNotActive);
    require!(order.owner == margin.owner, LimitOrderError::Unauthorized);
    unlock_order_funds(margin, order)?;
    order.status = OrderStatus::Cancelled;
    emit!(OrderCancelled {
        market: order.market,
        owner: margin.owner,
        order_id: order.order_id,
        remaining_lots: order.remaining_lots,
    });
    Ok(())
}
