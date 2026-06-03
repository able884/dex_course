
Á
limitorder.proto
limitorder"ä
InitializeMarginAccountRequest
chain_id (RchainId.
user_wallet_address (	RuserWalletAddress

market_pda (	R	marketPda"ç
InitializeMarginAccountResponse
success (Rsuccess
message (	Rmessage

margin_pda (	R	marginPda
tx_hash (	RtxHash"ù
SyncMarginAccountRequest
chain_id (RchainId.
user_wallet_address (	RuserWalletAddress

market_pda (	R	marketPda
tx_hash (	RtxHash"Ç
SyncMarginAccountResponse
success (Rsuccess
message (	Rmessage1
margin (2.limitorder.MarginAccountRmargin"£
SyncMarginAccountStatusRequest
chain_id (RchainId.
user_wallet_address (	RuserWalletAddress

market_pda (	R	marketPda
tx_hash (	RtxHash"‹
SyncMarginAccountStatusResponse
success (Rsuccess
message (	Rmessage
status (Rstatus

margin_pda (	R	marginPda
	tx_base64 (	RtxBase641
margin (2.limitorder.MarginAccountRmargin"”
MarginAccount
id (Rid

margin_pda (	R	marginPda

market_pda (	R	marketPda
owner (	Rowner
	base_free (	RbaseFree
base_locked (	R
baseLocked

quote_free (	R	quoteFree!
quote_locked (	RquoteLocked
status	 (Rstatus 
init_tx_hash
 (	R
initTxHash$
last_sync_slot (RlastSyncSlot"¡
CreateMarketRequest
chain_id (RchainId
admin (	Radmin
	base_mint (	RbaseMint

quote_mint (	R	quoteMint
base_symbol (	R
baseSymbol!
quote_symbol (	RquoteSymbol#
base_decimals (RbaseDecimals%
quote_decimals (RquoteDecimals
	tick_size	 (RtickSize 
min_base_lot
 (R
minBaseLot"
maker_fee_bps (RmakerFeeBps"
taker_fee_bps (RtakerFeeBps&
pyth_price_feed (	RpythPriceFeed"•
CreateMarketResponse
success (Rsuccess
message (	Rmessage

market_pda (	R	marketPda
	tx_base64 (	RtxBase64

expires_at (R	expiresAt"Ç
SyncMarketStatusRequest
chain_id (RchainId

market_pda (	R	marketPda
tx_hash (	RtxHash
admin (	Radmin"“
SyncMarketStatusResponse
success (Rsuccess
message (	Rmessage
status (Rstatus

market_pda (	R	marketPda
	tx_base64 (	RtxBase64.
market (2.limitorder.MarketInfoRmarket"¢

MarketInfo
id (Rid

market_pda (	R	marketPda
	base_mint (	RbaseMint

quote_mint (	R	quoteMint
base_symbol (	R
baseSymbol!
quote_symbol (	RquoteSymbol#
base_decimals (RbaseDecimals%
quote_decimals (RquoteDecimals

base_vault	 (	R	baseVault
quote_vault
 (	R
quoteVault
	tick_size (RtickSize 
min_base_lot (R
minBaseLot"
min_quote_lot (RminQuoteLot"
maker_fee_bps (RmakerFeeBps"
taker_fee_bps (RtakerFeeBps
status (Rstatus
paused (Rpaused2¶

LimitOrderr
InitializeMarginAccount*.limitorder.InitializeMarginAccountRequest+.limitorder.InitializeMarginAccountResponser
SyncMarginAccountStatus*.limitorder.SyncMarginAccountStatusRequest+.limitorder.SyncMarginAccountStatusResponseQ
CreateMarket.limitorder.CreateMarketRequest .limitorder.CreateMarketResponse]
SyncMarketStatus#.limitorder.SyncMarketStatusRequest$.limitorder.SyncMarketStatusResponseBZ./limitorderbproto3