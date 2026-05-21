use anchor_lang::prelude::*;

pub mod constants;
pub mod error;
pub mod instructions;
pub mod state;
pub mod events;

pub use constants::*;
pub use error::*;
pub use instructions::*;
pub use state::*;
pub use events::*;

declare_id!("C4Xn6ACR3XmTwpWm2fN82QSAx23TQkwVW8wro9Hdpqef");

#[program]
pub mod limit_order {
    use super::*;

    pub fn init_program(ctx: Context<InitProgram>, params: InitProgramParams) -> Result<()> {
        instructions::init_program(ctx, params)
    }

    pub fn update_program_params(
        ctx: Context<UpdateProgramParams>,
        params: UpdateProgramParamsData,
    ) -> Result<()> {
        instructions::update_program_params(ctx, params)
    }

    pub fn init_market(ctx: Context<InitMarket>, params: InitMarketParams) -> Result<()> {
        instructions::init_market(ctx, params)
    }

    pub fn update_market_params(
        ctx: Context<UpdateMarketParams>,
        params: UpdateMarketParamsData,
    ) -> Result<()> {
        instructions::update_market_params(ctx, params)
    }

    pub fn create_margin(ctx: Context<CreateMargin>, whitelist_proof: Vec<[u8; 32]>) -> Result<()> {
        instructions::create_margin(ctx, whitelist_proof)
    }

    pub fn deposit(ctx: Context<Deposit>, amount: u64, side: TokenSide, whitelist_proof: Vec<[u8; 32]>) -> Result<()> {
        instructions::deposit(ctx, amount, side, whitelist_proof)
    }

    pub fn withdraw(ctx: Context<Withdraw>, amount: u64, side: TokenSide) -> Result<()> {
        instructions::withdraw(ctx, amount, side)
    }

    pub fn place_order(ctx: Context<PlaceOrder>, params: PlaceOrderParams, whitelist_proof: Vec<[u8; 32]>) -> Result<()> {
        instructions::place_order(ctx, params, whitelist_proof)
    }

    pub fn cancel_order(ctx: Context<CancelOrder>) -> Result<()> {
        instructions::cancel_order(ctx)
    }

    pub fn emergency_close_order(ctx: Context<EmergencyCloseOrder>) -> Result<()> {
        instructions::emergency_close_order(ctx)
    }

    pub fn match_orders(ctx: Context<MatchOrders>, params: MatchParams) -> Result<()> {
        instructions::match_orders(ctx, params)
    }

    pub fn sweep_expired(ctx: Context<SweepExpired>) -> Result<()> {
        instructions::sweep_expired(ctx)
    }

    pub fn admin_sweep_fees(ctx: Context<AdminSweepFees>) -> Result<()> {
        instructions::admin_sweep_fees(ctx)
    }

    pub fn update_custom_price(
        ctx: Context<UpdateCustomPrice>,
        price: i64,
        exponent: i32,
        conf: u64,
    ) -> Result<()> {
        instructions::update_custom_price(ctx, price, exponent, conf)
    }

    pub fn close_order(ctx: Context<CloseOrder>) -> Result<()> {
        instructions::close_order(ctx)
    }
}