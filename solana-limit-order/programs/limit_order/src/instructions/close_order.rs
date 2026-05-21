use anchor_lang::prelude::*;
use crate::state::order::Order;
use crate::state::order::OrderStatus;
use crate::error::LimitOrderError;

#[derive(Accounts)]
pub struct CloseOrder<'info> {
    #[account(
        mut,
        close = owner,
        constraint = order.status != OrderStatus::Active @ LimitOrderError::OrderStillActive,
        constraint = order.owner == owner.key() @ LimitOrderError::Unauthorized
    )]
    pub order: Account<'info, Order>,
    #[account(mut)]
    pub owner: Signer<'info>,
}

pub fn close_order(ctx: Context<CloseOrder>) -> Result<()> {
    let order = &ctx.accounts.order;

    // Only allow closing non-active orders
    require!(
        order.status != OrderStatus::Active,
        LimitOrderError::OrderStillActive
    );

    // Verify ownership
    require!(
        order.owner == ctx.accounts.owner.key(),
        LimitOrderError::Unauthorized
    );

    msg!("Order {} closed, rent reclaimed", order.order_id);

    Ok(())
}
