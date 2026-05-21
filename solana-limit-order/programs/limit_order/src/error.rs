use anchor_lang::prelude::*;

#[error_code]
pub enum LimitOrderError {
    #[msg("Unauthorized")]
    Unauthorized,
    #[msg("Program paused")]
    ProgramPaused,
    #[msg("Market paused")]
    MarketPaused,
    #[msg("Invalid fee bps")]
    InvalidFeeBps,
    #[msg("Missing bump")]
    MissingBump,
    #[msg("Invalid amount")]
    InvalidAmount,
    #[msg("Invalid tick")]
    InvalidTick,
    #[msg("Insufficient free balance")]
    InsufficientFreeBalance,
    #[msg("Math overflow")]
    MathOverflow,
    #[msg("Order not active")]
    OrderNotActive,
    #[msg("Order expired")]
    OrderExpired,
    #[msg("Order not expired")]
    OrderNotExpired,
    #[msg("Price not crossed")]
    PriceNotCrossed,
    #[msg("Invalid Pyth account")]
    InvalidPythAccount,
    #[msg("Emergency only")]
    EmergencyOnly,
    #[msg("Below minimum fill")]
    BelowMinFill,
    #[msg("Whitelist check failed")]
    WhitelistCheckFailed,
    #[msg("Invalid Pyth price")]
    InvalidPythPrice,
    #[msg("Pyth price stale")]
    PythPriceStale,
    #[msg("Pyth price confidence too wide")]
    PythConfidenceTooWide,
    #[msg("Price outside allowed range")]
    PriceOutsideRange,
    #[msg("Invalid oracle type")]
    InvalidOracleType,
    #[msg("Custom price not set")]
    CustomPriceNotSet,
    #[msg("Invalid price exponent")]
    InvalidPriceExponent,
    #[msg("Order still active")]
    OrderStillActive,
}
