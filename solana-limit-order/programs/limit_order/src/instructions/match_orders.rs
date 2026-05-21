use anchor_lang::prelude::*;
use crate::state::config::ProgramConfig;
use crate::state::margin::UserMargin;
use crate::state::order::*;
use crate::error::LimitOrderError;
use crate::events::TradeEvent;
use crate::state::market::*;
use crate::constants::*;

#[derive(Accounts)]
pub struct MatchOrders<'info> {
    pub config: Account<'info, ProgramConfig>,
    #[account(mut, seeds = [b"LimitOrderMarket", market.base_mint.as_ref(), market.quote_mint.as_ref()], bump = market.bump)]
    pub market: Account<'info, Market>,
    #[account(mut, seeds = [b"margin", market.key().as_ref(), maker_order.owner.as_ref()], bump = maker_margin.bump)]
    pub maker_margin: Account<'info, UserMargin>,
    #[account(mut, seeds = [b"margin", market.key().as_ref(), taker_order.owner.as_ref()], bump = taker_margin.bump)]
    pub taker_margin: Account<'info, UserMargin>,
    #[account(
        mut,
        constraint = maker_order.market == market.key() @ LimitOrderError::InvalidAmount,
        constraint = maker_order.status == OrderStatus::Active @ LimitOrderError::OrderNotActive
    )]
    pub maker_order: Account<'info, Order>,
    #[account(
        mut,
        constraint = taker_order.market == market.key() @ LimitOrderError::InvalidAmount,
        constraint = taker_order.status == OrderStatus::Active @ LimitOrderError::OrderNotActive
    )]
    pub taker_order: Account<'info, Order>,
    /// CHECK: Pyth price feed account，键值校验
    pub pyth_price_feed: UncheckedAccount<'info>,
    pub caller: Signer<'info>,
}

#[derive(AnchorSerialize, AnchorDeserialize)]
pub struct MatchParams {
    pub match_qty_lots: u64,
}

pub fn match_orders(ctx: Context<MatchOrders>, params: MatchParams) -> Result<()> {
    let config = &ctx.accounts.config;
    require!(!config.paused, LimitOrderError::ProgramPaused);
    require!(!ctx.accounts.market.paused, LimitOrderError::MarketPaused);

    if config.matcher != Pubkey::default() {
        require!(
            ctx.accounts.caller.key() == config.matcher,
            LimitOrderError::Unauthorized
        );
    }

    require!(params.match_qty_lots > 0, LimitOrderError::InvalidAmount);
    require!(params.match_qty_lots <= ctx.accounts.maker_order.remaining_lots, LimitOrderError::InvalidAmount);
    require!(params.match_qty_lots <= ctx.accounts.taker_order.remaining_lots, LimitOrderError::InvalidAmount);

    // Handle self-trading
    if ctx.accounts.maker_order.owner == ctx.accounts.taker_order.owner {
        match ctx.accounts.taker_order.self_trade_behavior {
            SelfTradeBehavior::DecrementTake => {
                // Unlock the decremented quantity from taker's margin
                let taker_margin = &mut ctx.accounts.taker_margin;
                let taker_order = &mut ctx.accounts.taker_order;

                match taker_order.locked_side {
                    LockedSide::Base => {
                        taker_margin.base_locked = taker_margin.base_locked
                            .checked_sub(params.match_qty_lots)
                            .ok_or(LimitOrderError::MathOverflow)?;
                        taker_margin.base_free = taker_margin.base_free
                            .checked_add(params.match_qty_lots)
                            .ok_or(LimitOrderError::MathOverflow)?;
                    }
                    LockedSide::Quote => {
                        let quote_amount = taker_order.price_lots
                            .checked_mul(params.match_qty_lots)
                            .ok_or(LimitOrderError::MathOverflow)?;
                        taker_margin.quote_locked = taker_margin.quote_locked
                            .checked_sub(quote_amount)
                            .ok_or(LimitOrderError::MathOverflow)?;
                        taker_margin.quote_free = taker_margin.quote_free
                            .checked_add(quote_amount)
                            .ok_or(LimitOrderError::MathOverflow)?;
                    }
                }

                taker_order.remaining_lots = taker_order.remaining_lots
                    .checked_sub(params.match_qty_lots)
                    .ok_or(LimitOrderError::MathOverflow)?;

                if taker_order.remaining_lots == 0 {
                    taker_order.status = OrderStatus::Filled;
                }

                msg!("Self-trade handled: DecrementTake, unlocked {} lots", params.match_qty_lots);
                return Ok(());
            }
            SelfTradeBehavior::CancelNew => {
                // Unlock all funds from taker order
                unlock_order_funds(&mut ctx.accounts.taker_margin, &ctx.accounts.taker_order)?;
                ctx.accounts.taker_order.status = OrderStatus::Cancelled;

                msg!("Self-trade handled: CancelNew, order cancelled");
                return Ok(());
            }
        }
    }
    require!(
        price_crossed(&ctx.accounts.maker_order, &ctx.accounts.taker_order),
        LimitOrderError::PriceNotCrossed
    );
    let slot = Clock::get()?.slot;

    // Check order expiry: expiry_slot == 0 means never expires
    require!(
        ctx.accounts.maker_order.expiry_slot == 0 || slot <= ctx.accounts.maker_order.expiry_slot,
        LimitOrderError::OrderExpired
    );
    require!(
        ctx.accounts.taker_order.expiry_slot == 0 || slot <= ctx.accounts.taker_order.expiry_slot,
        LimitOrderError::OrderExpired
    );

    // Price verification based on oracle type
    match ctx.accounts.market.oracle_type {
        PriceOracleType::None => {
            msg!("Market has no price verification - skipping price check");
        }
        PriceOracleType::Pyth => {
            verify_pyth_price(
                config,
                &ctx.accounts.pyth_price_feed.to_account_info(),
                &ctx.accounts.market.pyth_price_feed_id,
                ctx.accounts.maker_order.price_lots,
                slot,
            )?;
            msg!("Pyth price verification passed");
        }
        PriceOracleType::Custom => {
            verify_custom_price(
                config,
                &ctx.accounts.market,
                ctx.accounts.maker_order.price_lots,
                slot,
            )?;
            msg!("Custom price verification passed");
        }
    }
    if let Some(min_fill) = ctx.accounts.taker_order.min_fill_bps {
        let filled_bps = params
            .match_qty_lots
            .checked_mul(BPS_DENOM)
            .ok_or(LimitOrderError::MathOverflow)?
            / ctx.accounts.taker_order.qty_lots;
        require!(filled_bps >= min_fill as u64, LimitOrderError::BelowMinFill);
    }
    if let Some(min_fill) = ctx.accounts.maker_order.min_fill_bps {
        let filled_bps = params
            .match_qty_lots
            .checked_mul(BPS_DENOM)
            .ok_or(LimitOrderError::MathOverflow)?
            / ctx.accounts.maker_order.qty_lots;
        require!(filled_bps >= min_fill as u64, LimitOrderError::BelowMinFill);
    }
    settle_match(
        &mut ctx.accounts.market,
        &mut ctx.accounts.maker_margin,
        &mut ctx.accounts.taker_margin,
        &mut ctx.accounts.maker_order,
        &mut ctx.accounts.taker_order,
        params.match_qty_lots,
    )?;

    // Calculate fee for event (same logic as settle_match)
    let quote_amount = params.match_qty_lots
        .checked_mul(ctx.accounts.maker_order.price_lots)
        .ok_or(LimitOrderError::MathOverflow)?;
    let fee_numerator = quote_amount
        .checked_mul(ctx.accounts.market.taker_fee_bps as u64)
        .ok_or(LimitOrderError::MathOverflow)?;
    let fee = fee_numerator
        .checked_add(BPS_DENOM - 1)
        .ok_or(LimitOrderError::MathOverflow)?
        / BPS_DENOM;

    emit!(TradeEvent {
        market: ctx.accounts.market.key(),
        maker: ctx.accounts.maker_order.owner,
        taker: ctx.accounts.taker_order.owner,
        maker_order_id: ctx.accounts.maker_order.order_id,
        taker_order_id: ctx.accounts.taker_order.order_id,
        qty_lots: params.match_qty_lots,
        price_lots: ctx.accounts.maker_order.price_lots,
        fee,
    });
    Ok(())
}
