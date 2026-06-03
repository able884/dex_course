use crate::constants::BPS_DENOM;
use crate::error::LimitOrderError;
use crate::state::margin::UserMargin;
use crate::state::market::Market;
use anchor_lang::prelude::*;

#[account]
pub struct Order {
    pub owner: Pubkey,                          // 下单用户钱包公钥
    pub market: Pubkey,                         // 归属Market市场账户
    pub side: Side,                             // 订单方向 Bid(买)/Ask(卖)
    pub price_lots: u64,                        // 报价档位 = N * tick_size
    pub qty_lots: u64,                          // 下单原始总数量(base lot)
    pub remaining_lots: u64,                    // 剩余未成交数量(base lot)
    pub expiry_slot: u64,                       // 过期slot高度，超过自动撤单
    pub order_id: u64,                          // 订单唯一ID（关联market.seq_num生成）
    pub min_fill_bps: Option<u16>,              // 最小成交比例(bps)，部分成交规则
    pub self_trade_behavior: SelfTradeBehavior, // 自成交处理策略
    pub locked_side: LockedSide,                // 该订单冻结哪种币种(base/quote)
    pub status: OrderStatus,                    // 订单状态
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
    Active, // 激活中
    Filled, // 成交完成
    Cancelled, // 撤单
    Expired, // 过期
}

#[derive(AnchorSerialize, AnchorDeserialize, Clone, Copy, PartialEq, Eq)]
pub enum SelfTradeBehavior {
    DecrementTake, // 同一个钱包自己买单撞自己卖单=自己成交
    CancelNew,     // 只要新单子会和自己老单子成交，就取消新单子
}

///  判断maker挂单 & taker主动单价格是否交叉，满足返回true才可撮合
pub fn price_crossed(maker: &Order, taker: &Order) -> bool {
    match taker.side {
        // Taker买单(Bid)：愿意高价买，T报价 >= Maker卖价(Ask) → 交叉成交
        Side::Bid => taker.price_lots >= maker.price_lots,
        // Taker卖单(Ask)：愿意低价卖，T报价 <= Maker买价(Bid) → 交叉成交
        Side::Ask => maker.price_lots >= taker.price_lots,
    }
}

/// 解锁订单剩余未成交部分的保证金
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

/// 撮合结算函数，一笔 Maker 挂单 & Taker 吃单部分 / 全量成交后，
/// 双方保证金账户资产划转、扣手续费、更新剩余挂单量、归集平台手续费
pub fn settle_match(
    market: &mut Market,           // 市场全局配置，存费率、手续费归集
    maker_margin: &mut UserMargin, // 挂单方用户保证金账户
    taker_margin: &mut UserMargin, // 吃单方用户保证金账户
    maker_order: &mut Order,       // 盘口原有挂单（被动成交Maker）
    taker_order: &mut Order,       // 主动吃单（主动成交Taker）
    qty: u64,                      // 本次实际撮合成交base数量(lot)
) -> Result<()> {
    // 报价挡位tick数
    let price = maker_order.price_lots;
    // 成交总 quote 金额 = 成交数量 * 挂单价格
    let quote_amount = qty
        .checked_mul(price)
        .ok_or(LimitOrderError::MathOverflow)?;

    // Calculate fee with proper rounding (round up to ensure protocol doesn't lose fees)
    // 分子 = 成交quote总额 * taker_fee_bps
    let fee_numerator = quote_amount
        .checked_mul(market.taker_fee_bps as u64)
        .ok_or(LimitOrderError::MathOverflow)?;
    // 向上取整：(分子 + 分母 -1) / 分母
    let fee = fee_numerator
        .checked_add(BPS_DENOM - 1) // Add (denominator - 1) for ceiling division
        .ok_or(LimitOrderError::MathOverflow)?
        / BPS_DENOM;

    match taker_order.side {
        // Taker 是买单（想买 base、付出 quote）
        Side::Bid => {
            maker_margin.base_locked = maker_margin.base_locked.checked_sub(qty).ok_or(LimitOrderError::MathOverflow)?;
            taker_margin.quote_locked = taker_margin.quote_locked.checked_sub(quote_amount).ok_or(LimitOrderError::MathOverflow)?;
            taker_margin.base_free = taker_margin.base_free.checked_add(qty).ok_or(LimitOrderError::MathOverflow)?;
            maker_margin.quote_free = maker_margin.quote_free.checked_add(quote_amount.checked_sub(fee).ok_or(LimitOrderError::MathOverflow)?).ok_or(LimitOrderError::MathOverflow)?;
        }
        //  Taker 是卖单（想卖 base、收 quote）
        Side::Ask => {
            taker_margin.base_locked = taker_margin.base_locked.checked_sub(qty).ok_or(LimitOrderError::MathOverflow)?;
            maker_margin.quote_locked = maker_margin.quote_locked.checked_sub(quote_amount).ok_or(LimitOrderError::MathOverflow)?;
            maker_margin.base_free = maker_margin.base_free.checked_add(qty).ok_or(LimitOrderError::MathOverflow)?;
            taker_margin.quote_free = taker_margin.quote_free.checked_add(quote_amount.checked_sub(fee).ok_or(LimitOrderError::MathOverflow)?).ok_or(LimitOrderError::MathOverflow)?;
        }
    }
    // 更新订单剩余数量和状态
    maker_order.remaining_lots = maker_order.remaining_lots.checked_sub(qty).ok_or(LimitOrderError::MathOverflow)?;
    taker_order.remaining_lots = taker_order.remaining_lots.checked_sub(qty).ok_or(LimitOrderError::MathOverflow)?;
    if maker_order.remaining_lots == 0 {
        maker_order.status = OrderStatus::Filled;
    }
    if taker_order.remaining_lots == 0 {
        taker_order.status = OrderStatus::Filled;
    }
    // 更新市场手续费归集
    market.fee_accumulator = market.fee_accumulator.checked_add(fee).ok_or(LimitOrderError::MathOverflow)?;
    Ok(())
}
