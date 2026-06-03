
Ïx
market.protomarket"Ú
GetPumpTokenListRequest
chain_id (RchainId
pump_status (R
pumpStatus
sorted_type (	R
sortedType'
honeypot_filter (	RhoneypotFilter
page_no (RpageNo
	page_size (RpageSize
	pump_type (	RpumpType"[
GetPumpTokenListResponse)
list (2.market.PumpTokenItemRlist
total (Rtotal"Í
PumpTokenItem
chain_id (RchainId

chain_icon (	R	chainIcon#
token_address (	RtokenAddress

token_icon (	R	tokenIcon

token_name (	R	tokenName
launch_time (R
launchTime
mkt_cap (RmktCap

hold_count (R	holdCount
txs_24h	 (Rtxs24h
vol_24h
 (Rvol24h+
domestic_progress (RdomesticProgress)
twitter_username (	RtwitterUsername
telegram (	Rtelegram
change24 (Rchange24!
pair_address (	RpairAddress"≠
GetClmmPoolListRequest
chain_id (RchainId!
pool_version (RpoolVersion
sorted_type (	R
sortedType
page_no (RpageNo
	page_size (RpageSize"Y
GetClmmPoolListResponse(
list (2.market.ClmmPoolItemRlist
total (Rtotal"ƒ
ClmmPoolItem
chain_id (RchainId

chain_icon (	R	chainIcon

pool_state (	R	poolState(
input_vault_mint (	RinputVaultMint*
output_vault_mint (	RoutputVaultMint,
input_token_symbol (	RinputTokenSymbol.
output_token_symbol (	RoutputTokenSymbol(
input_token_icon (	RinputTokenIcon*
output_token_icon	 (	RoutputTokenIcon$
trade_fee_rate
 (RtradeFeeRate
launch_time (R
launchTime#
liquidity_usd (RliquidityUsd
txs_24h (Rtxs24h
vol_24h (Rvol24h
apr (Rapr!
pool_version (RpoolVersion"´
PushTokenInfoRequest
chain_id (RchainId#
token_address (	RtokenAddress!
pair_address (	RpairAddress
token_price (R
tokenPrice
mkt_cap (RmktCap

token_name (	R	tokenName!
token_symbol (	RtokenSymbol

token_icon (	R	tokenIcon
launch_time	 (R
launchTime

hold_count
 (R	holdCount
	change_24 (Rchange24
txs_24h (Rtxs24h
pump_status (R
pumpStatus"£
GetPoolDetailRequest
chain_id (RchainId

pool_state (	R	poolState!
pool_version (RpoolVersion.
user_wallet_address (	RuserWalletAddress"–

GetPoolDetailResponse
chain_id (RchainId

pool_state (	R	poolState(
input_vault_mint (	RinputVaultMint*
output_vault_mint (	RoutputVaultMint,
input_token_symbol (	RinputTokenSymbol.
output_token_symbol (	RoutputTokenSymbol(
input_token_icon (	RinputTokenIcon*
output_token_icon (	RoutputTokenIcon$
trade_fee_rate	 (RtradeFeeRate
launch_time
 (R
launchTime#
liquidity_usd (RliquidityUsd
txs_24h (Rtxs24h
vol_24h (Rvol24h
apr (Rapr!
pool_version (RpoolVersion%
locked_percent (RlockedPercent!
input_amount (RinputAmount#
output_amount (RoutputAmount!
base_reserve (RbaseReserve#
quote_reserve (RquoteReserve#
input_reserve (RinputReserve%
output_reserve (RoutputReserve
price (Rprice$
quote_per_base (RquotePerBase!
market_price (RmarketPrice*
user_pooled_input (RuserPooledInput,
user_pooled_output (RuserPooledOutput$
user_staked_lp (RuserStakedLp(
user_unstaked_lp (RuserUnstakedLp-
price_range_24h_min (RpriceRange24hMin-
price_range_24h_max (RpriceRange24hMax+
price_range_7d_min  (RpriceRange7dMin+
price_range_7d_max! (RpriceRange7dMax-
price_range_30d_min" (RpriceRange30dMin-
price_range_30d_max# (RpriceRange30dMax"‡
PushTokenInfoResponse
chain_id (RchainId#
token_address (	RtokenAddress
txs_24h (Rtxs24h
vol_24h (	Rvol24h
	change_24 (	Rchange24
token_price (	R
tokenPrice
mkt_cap (	RmktCap"[
GetPairInfoByTokenRequest
chain_id (RchainId#
token_address (	RtokenAddress"Ã
GetPairInfoByTokenResponse
chain_id (RchainId
address (	Raddress
name (	Rname'
factory_address (	RfactoryAddress,
base_token_address (	RbaseTokenAddress#
token_address (	RtokenAddress*
base_token_symbol (	RbaseTokenSymbol!
token_symbol (	RtokenSymbol,
base_token_decimal	 (RbaseTokenDecimal#
token_decimal
 (RtokenDecimal:
base_token_is_native_token (RbaseTokenIsNativeToken/
base_token_is_token0 (RbaseTokenIsToken03
init_base_token_amount (RinitBaseTokenAmount*
init_token_amount (RinitTokenAmount9
current_base_token_amount (RcurrentBaseTokenAmount0
current_token_amount (RcurrentTokenAmount
fdv (Rfdv
mkt_cap (RmktCap
token_price (R
tokenPrice(
base_token_price (RbaseTokenPrice
	block_num (RblockNum

block_time (R	blockTime.
highest_token_price (RhighestTokenPrice*
latest_trade_time (RlatestTradeTime"X
GetNativeTokenPriceRequest
chain_id (RchainId
search_time (	R
searchTime"N
GetNativeTokenPriceResponse/
base_token_price_usd (RbaseTokenPriceUsd"U
GetTokenInfoRequest
chain_id (RchainId#
token_address (	RtokenAddress"∆
GetTokenInfoResponse
chain_id (RchainId
address (	Raddress
name (	Rname
symbol (	Rsymbol
decimals (Rdecimals!
total_supply (RtotalSupply
icon (	Ricon

hold_count (R	holdCount'
is_ca_drop_owner	 (RisCaDropOwner 
is_ca_verify
 (R
isCaVerify"
is_honey_scam (RisHoneyScam$
is_liquid_lock (RisLiquidLock+
is_can_pause_trade (RisCanPauseTrade)
is_can_change_tax (RisCanChangeTax+
is_have_black_list (RisHaveBlackList%
is_can_all_sell (RisCanAllSell"
is_have_proxy (RisHaveProxy/
is_can_external_call (RisCanExternalCall'
is_can_add_token (RisCanAddToken-
is_can_change_token (RisCanChangeToken
sell_tax (RsellTax
buy_tax (RbuyTax)
twitter_username (	RtwitterUsername
website (	Rwebsite
telegram (	Rtelegram
is_check_ca (R	isCheckCa
check_ca_at (R	checkCaAt
program (	Rprogram"Î
QuoteCpmmRequest
chain_id (RchainId

pool_state (	R	poolState

input_mint (	R	inputMint
output_mint (	R
outputMint
	amount_in (	RamountIn

amount_out (	R	amountOut!
slippage_bps (RslippageBps"ß
QuoteCpmmResponse
pay_mint (	RpayMint!
receive_mint (	RreceiveMint

pay_amount (	R	payAmount%
receive_amount (	RreceiveAmount,
min_receive_amount (	RminReceiveAmount(
price_impact_pct (	RpriceImpactPct 
fee_rate_pct (	R
feeRatePct
price (Rprice"Î
QuoteClmmRequest
chain_id (RchainId

pool_state (	R	poolState

input_mint (	R	inputMint
output_mint (	R
outputMint
	amount_in (	RamountIn

amount_out (	R	amountOut!
slippage_bps (RslippageBps"ß
QuoteClmmResponse
pay_mint (	RpayMint!
receive_mint (	RreceiveMint

pay_amount (	R	payAmount%
receive_amount (	RreceiveAmount,
min_receive_amount (	RminReceiveAmount(
price_impact_pct (	RpriceImpactPct 
fee_rate_pct (	R
feeRatePct
price (Rprice"W
GetClmmPoolDepthDataRequest
chain_id (RchainId

pool_state (	R	poolState"¨
GetClmmPoolDepthDataResponse
count (Rcount*
line (2.market.DepthDataPointRline$
time_range_min (RtimeRangeMin$
time_range_max (RtimeRangeMax"X
DepthDataPoint
price (Rprice
	liquidity (	R	liquidity
tick (Rtick"∑
GetUserPositionsRequest
chain_id (RchainId.
user_wallet_address (	RuserWalletAddress
	pool_type (	RpoolType
page_no (RpageNo
	page_size (RpageSize"í
GetUserPositionsResponse*
items (2.market.PositionItemRitems
total (Rtotal
page_no (RpageNo
	page_size (RpageSize"Õ

PositionItem
chain_id (RchainId.
user_wallet_address (	RuserWalletAddress

pool_state (	R	poolState!
pool_version (RpoolVersion*
position_nft_mint (	RpositionNftMint0
position_nft_account (	RpositionNftAccount+
personal_position (	RpersonalPosition(
tick_lower_index (RtickLowerIndex(
tick_upper_index	 (RtickUpperIndex
	liquidity
 (	R	liquidity
token0_mint (	R
token0Mint
token1_mint (	R
token1Mint#
token0_symbol (	Rtoken0Symbol#
token1_symbol (	Rtoken1Symbol
token0_name (	R
token0Name
token1_name (	R
token1Name'
token0_decimals (Rtoken0Decimals'
token1_decimals (Rtoken1Decimals
token0_icon (	R
token0Icon
token1_icon (	R
token1Icon%
position_value (RpositionValue#
token0_amount (Rtoken0Amount#
token1_amount (Rtoken1Amount%
unclaimed_fees (RunclaimedFees
	price_min (RpriceMin
	price_max (RpriceMax#
current_price (RcurrentPrice
is_in_range (R	isInRange

lp_balance (R	lpBalance
lp_value (RlpValue,
pool_liquidity_usd (RpoolLiquidityUsd&
pool_volume_24h  (RpoolVolume24h
pool_apr! (RpoolApr
fee_tier" (RfeeTier
tx_hash# (	RtxHash

block_time$ (R	blockTime

created_at% (R	createdAt

updated_at& (R	updatedAt"j
GetLimitOrderBookRequest
chain_id (RchainId

market_pda (	R	marketPda
depth (Rdepth"ﬁ
GetLimitOrderBookResponse*
bids (2.market.OrderBookLevelRbids*
asks (2.market.OrderBookLevelRasks
best_bid (	RbestBid
best_ask (	RbestAsk
spread (	Rspread
	mid_price (	RmidPrice"Ñ
OrderBookLevel
price (	Rprice
quantity (	Rquantity
total_value (	R
totalValue
order_count (R
orderCount"”
GetUserLimitOrdersRequest
chain_id (RchainId.
user_wallet_address (	RuserWalletAddress

market_pda (	R	marketPda
status (Rstatus
page_no (RpageNo
	page_size (RpageSize"ó
GetUserLimitOrdersResponse-
orders (2.market.UserOrderItemRorders
total (Rtotal
page_no (RpageNo
	page_size (RpageSize"ì
UserOrderItem
	order_pda (	RorderPda

market_pda (	R	marketPda
base_symbol (	R
baseSymbol!
quote_symbol (	RquoteSymbol
side (Rside
price (	Rprice
quantity (	Rquantity
	remaining (	R	remaining%
filled_percent	 (	RfilledPercent
status
 (Rstatus

created_at (R	createdAt
expiry_slot (R
expirySlot
total_value (	R
totalValue"T
GetLimitOrderDetailRequest
chain_id (RchainId
	order_pda (	RorderPda"L
GetLimitOrderDetailResponse-
order (2.market.OrderDetailItemRorder"å
OrderDetailItem
	order_pda (	RorderPda
order_id (RorderId

market_pda (	R	marketPda
base_symbol (	R
baseSymbol!
quote_symbol (	RquoteSymbol
owner (	Rowner

margin_pda (	R	marginPda
side (Rside
price	 (	Rprice
quantity
 (	Rquantity
	remaining (	R	remaining
total_value (	R
totalValue%
filled_percent (	RfilledPercent
status (Rstatus
expiry_slot (R
expirySlot.
self_trade_behavior (RselfTradeBehavior$
create_tx_hash (	RcreateTxHash$
cancel_tx_hash (	RcancelTxHash

created_at (R	createdAt

updated_at (R	updatedAt
	filled_at (RfilledAt!
cancelled_at (RcancelledAt

expired_at (R	expiredAt+
fills (2.market.OrderFillItemRfills"n
GetLimitOrderMarketsRequest
chain_id (RchainId
page_no (RpageNo
	page_size (RpageSize"b
GetLimitOrderMarketsResponse,
markets (2.market.MarketItemRmarkets
total (Rtotal"À

MarketItem

market_pda (	R	marketPda
	base_mint (	RbaseMint

quote_mint (	R	quoteMint
base_symbol (	R
baseSymbol!
quote_symbol (	RquoteSymbol#
base_decimals (RbaseDecimals%
quote_decimals (RquoteDecimals
	tick_size (	RtickSize!
min_quantity	 (	RminQuantity"
maker_fee_bps
 (RmakerFeeBps"
taker_fee_bps (RtakerFeeBps
paused (Rpaused#
active_orders (RactiveOrders

volume_24h (	R	volume24h
best_bid (	RbestBid
best_ask (	RbestAsk
status (Rstatus 
init_tx_hash (	R
initTxHash"Ä
GetUserMarginRequest
chain_id (RchainId.
user_wallet_address (	RuserWalletAddress

market_pda (	R	marketPda"J
GetUserMarginResponse1
margin (2.market.MarginAccountItemRmargin"ù
SyncMarginBalanceRequest
chain_id (RchainId

market_pda (	R	marketPda.
user_wallet_address (	RuserWalletAddress
tx_hash (	RtxHash"Ç
SyncMarginBalanceResponse
success (Rsuccess
message (	Rmessage1
margin (2.market.MarginAccountItemRmargin"µ
MarginAccountItem

margin_pda (	R	marginPda

market_pda (	R	marketPda
base_symbol (	R
baseSymbol!
quote_symbol (	RquoteSymbol
	base_free (	RbaseFree
base_locked (	R
baseLocked

quote_free (	R	quoteFree!
quote_locked (	RquoteLocked

base_total	 (	R	baseTotal
quote_total
 (	R
quoteTotal$
last_sync_slot (RlastSyncSlot
status (Rstatus 
init_tx_hash (	R
initTxHash"ÿ
GetLimitOrderFillsRequest
chain_id (RchainId
	order_pda (	RorderPda.
user_wallet_address (	RuserWalletAddress

market_pda (	R	marketPda
page_no (RpageNo
	page_size (RpageSize"_
GetLimitOrderFillsResponse+
fills (2.market.OrderFillItemRfills
total (Rtotal"œ
OrderFillItem

market_pda (	R	marketPda
base_symbol (	R
baseSymbol!
quote_symbol (	RquoteSymbol
maker (	Rmaker
taker (	Rtaker
quantity (	Rquantity
price (	Rprice
fee (	Rfee
total_value	 (	R
totalValue
tx_hash
 (	RtxHash
slot (Rslot

created_at (R	createdAt2î
MarketU
GetPumpTokenList.market.GetPumpTokenListRequest .market.GetPumpTokenListResponseR
GetClmmPoolList.market.GetClmmPoolListRequest.market.GetClmmPoolListResponseL
GetPoolDetail.market.GetPoolDetailRequest.market.GetPoolDetailResponseL
PushTokenInfo.market.PushTokenInfoRequest.market.PushTokenInfoResponse[
GetPairInfoByToken!.market.GetPairInfoByTokenRequest".market.GetPairInfoByTokenResponse^
GetNativeTokenPrice".market.GetNativeTokenPriceRequest#.market.GetNativeTokenPriceResponseI
GetTokenInfo.market.GetTokenInfoRequest.market.GetTokenInfoResponse@
	QuoteCpmm.market.QuoteCpmmRequest.market.QuoteCpmmResponse@
	QuoteClmm.market.QuoteClmmRequest.market.QuoteClmmResponsea
GetClmmPoolDepthData#.market.GetClmmPoolDepthDataRequest$.market.GetClmmPoolDepthDataResponseU
GetUserPositions.market.GetUserPositionsRequest .market.GetUserPositionsResponseX
GetLimitOrderBook .market.GetLimitOrderBookRequest!.market.GetLimitOrderBookResponse[
GetUserLimitOrders!.market.GetUserLimitOrdersRequest".market.GetUserLimitOrdersResponse^
GetLimitOrderDetail".market.GetLimitOrderDetailRequest#.market.GetLimitOrderDetailResponsea
GetLimitOrderMarkets#.market.GetLimitOrderMarketsRequest$.market.GetLimitOrderMarketsResponseL
GetUserMargin.market.GetUserMarginRequest.market.GetUserMarginResponseX
SyncMarginBalance .market.SyncMarginBalanceRequest!.market.SyncMarginBalanceResponse[
GetLimitOrderFills!.market.GetLimitOrderFillsRequest".market.GetLimitOrderFillsResponseB
Z./marketbproto3