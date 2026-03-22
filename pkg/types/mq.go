package types

import (
	"time"

	"github.com/blocto/solana-go-sdk/common"
	bin "github.com/gagliardetto/binary"
	"richcode.cc/dex/model/solmodel"
)

const (
	TradeTypeSell                                      = "sell"
	TradeTypeBuy                                       = "buy"
	TradePumpAmmCreatePool                             = "pump_amm_create_pool"
	TradePumpAmmSell                                   = "pump_amm_sell"
	TradePumpAmmBuy                                    = "pump_amm_buy"
	TradeTypeAddPosition                               = "add"
	TradeTypeRemovePosition                            = "remove"
	TradePumpCreate                                    = "create"
	TradePumpLaunch                                    = "launch"
	TradeTokenMint                                     = "token_mint"
	TradeTokenBurn                                     = "token_burn"
	TradeRaydiumConcentratedLiquidityDecreaseLiquidity = "raydium_concentrated_liquidity_decrease"
	TradeRaydiumConcentratedLiquidityIncreaseLiquidity = "raydium_concentrated_liquidity_increase"
	TradeRaydiumCPMMDecreaseLiquidity                  = "raydium_cpmm_decrease"
	TradeRaydiumCPMMIncreaseLiquidity                  = "raydium_cpmm_increase"
)

type AddressInfo struct {
	WalletAddress string `json:"wallet_address"`
	AddressTag    string `json:"address_tag"`
	AddressIcon   string `json:"address_icon"`
	TwitterLink   string `json:"twitter_link"`
}

type InstructionMintTo struct {
	Mint   common.PublicKey
	To     common.PublicKey
	Amount uint64
}

type InstructionBurn struct {
	Mint    common.PublicKey
	Account common.PublicKey
	Amount  uint64
}

type CLMMOpenPositionInfo struct {
	TickLowerIndex           *int32       `json:"tick_lower_index"`
	TickUpperIndex           *int32       `json:"tick_upper_index"`
	TickArrayLowerStartIndex *int32       `json:"tick_array_lower_start_index"`
	TickArrayUpperStartIndex *int32       `json:"tick_array_upper_start_index"`
	Liquidity                *bin.Uint128 `json:"liquidity"`
	Amount0Max               *uint64      `json:"amount0_max"`
	Amount1Max               *uint64      `json:"amount1_max"`
	Payer                    string       `json:"payer"`
	PositionNftOwner         string       `json:"position_nft_owner"`
	PositionNftMint          string       `json:"position_nft_mint"`
	PositionNftAccount       string       `json:"position_nft_account"`
	MetadataAccount          string       `json:"metadata_account"`
	PoolState                string       `json:"pool_state"`
	ProtocolPosition         string       `json:"protocol_position"`
	TickArrayLower           string       `json:"tick_array_lower"`
	TickArrayUpper           string       `json:"tick_array_upper"`
	PersonalPosition         string       `json:"personal_position"`
	TokenAccount0            string       `json:"token_account_0"`
	TokenAccount1            string       `json:"token_account_1"`
	TokenVault0              string       `json:"token_vault_0"`
	TokenVault1              string       `json:"token_vault_1"`
	Rent                     string       `json:"rent"`
	SystemProgram            string       `json:"system_program"`
	TokenProgram             string       `json:"token_program"`
	AssociatedTokenProgram   string       `json:"associated_token_program"`
	MetadataProgram          string       `json:"metadata_program"`
}

type TradeWithPair struct {
	CLMMOpenPositionInfo         *CLMMOpenPositionInfo `json:"clmm_open_position_info"`
	TraderInfo                   AddressInfo           `json:"trader_info"`         // Trader information
	InstructionMintTo            InstructionMintTo     `json:"instruction_mint_to"` // InstructionMintTo
	InstructionBurn              InstructionBurn       `json:"instruction_burn"`    // InstructionMintTo
	Slot                         int64                 `json:"slot"`
	ChainId                      string                `json:"chain_id" tag:"true"`     // Tag for chain ID
	ChainIdInt                   int                   `json:"chain_id_int" tag:"true"` // Tag for chain ID
	PairAddr                     string                `json:"pair_addr" tag:"true"`    // Tag for address
	TxHash                       string                `json:"tx_hash" tag:"true"`      // Tag for transaction hash, may cause memory overflow; needs periodic roll-up and deletion
	HashId                       string                `json:"hash_id"`
	Maker                        string                `json:"maker"`                             // Address
	Type                         string                `json:"type"`                              // Tag: sell/buy/add_position/remove_position
	BaseTokenAmount              float64               `json:"base_token_amount"`                 // Amount of base token changed
	TokenAmount                  float64               `json:"token_amount"`                      // Amount of non-base token changed
	BaseTokenPriceUSD            float64               `json:"base_token_price_usd"`              // Price of the base token in USD
	TotalUSD                     float64               `json:"total_usd"`                         // Total value in USD
	TokenPriceUSD                float64               `json:"token_price_usd"`                   // Price of the non-base token in USD
	To                           string                `json:"to"`                                // Token recipient address
	BlockNum                     int64                 `json:"block_num"`                         // Block height
	BlockTime                    int64                 `json:"block_time"`                        // Block time
	TransactionIndex             int                   `json:"transaction_index"`                 // Transaction index
	LogIndex                     int                   `json:"log_index"`                         // Log index
	SwapName                     string                `json:"swap_name"`                         // Trading pair version
	CurrentTokenInPoolAmount     float64               `json:"current_token_in_pool_amount"`      // Current token amount in pool
	CurrentBaseTokenInPoolAmount float64               `json:"current_base_token_in_pool_amount"` // Current base token amount in pool

	PairInfo Pair `json:"pair_info"`

	KlineUpDown5m  float64 `json:"kline_up_down_5m"`  // 5-minute price change, used for pushing to websocket
	KlineUpDown1h  float64 `json:"kline_up_down_1h"`  // 1-hour price change, used for pushing to websocket
	KlineUpDown4h  float64 `json:"kline_up_down_4h"`  // 4-hour price change, used for pushing to websocket
	KlineUpDown6h  float64 `json:"kline_up_down_6h"`  // 6-hour price change, used for pushing to websocket
	KlineUpDown24h float64 `json:"kline_up_down_24h"` // 24-hour price change, used for pushing to websocket
	Fdv            float64 `json:"fdv"`               // Market cap, used for pushing to websocket
	Mcap           float64 `json:"mcap"`              // Circulating market cap

	TokenAmountInt     int64 `json:"token_amount_int"` // Not divided by decimal
	BaseTokenAmountInt int64 `json:"base_token_amount_int"`
	Clamp              bool  `json:"clamp"` // true: clamped or in a clamp
	Clipper            bool  `json:"-"`     // true: clamp

	// pump
	PumpPoint                    float64   `json:"pump_point"`    // Pump score
	PumpLaunched                 bool      `json:"pump_launched"` // Pump launched
	PumpMarketCap                float64   `json:"pump_market_cap"`
	PumpOwner                    string    `json:"pump_owner"`
	PumpSwapPairAddr             string    `json:"pump_swap_pair_addr"`
	PumpVirtualBaseTokenReserves float64   `json:"pump_virtual_base_token_reserves,omitempty"`
	PumpVirtualTokenReserves     float64   `json:"pump_virtual_token_reserves,omitempty"`
	PumpStatus                   int       `json:"pump_status"`
	PumpPairAddr                 string    `json:"pump_pair_addr"`
	CreateTime                   time.Time `json:"create_time"`

	// sol
	BaseTokenAccountAddress string `json:"-"`
	TokenAccountAddress     string `json:"-"`

	// PumpAmm
	LpMintAddress          string                `json:"lp_mint_address"`
	TokenAmount1           uint64                `json:"token_amount_1"`
	TokenAmount2           uint64                `json:"token_amount_2"`
	PoolBaseTokenReserves  uint64                `json:"pool_base_token_reserves"`
	PoolQuoteTokenReserves uint64                `json:"pool_quote_token_reserves"`
	PumpAmmInfo            *solmodel.PumpAmmInfo `json:"-"`
}
