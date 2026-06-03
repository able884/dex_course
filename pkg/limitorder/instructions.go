package limitorder

import (
	"encoding/binary"

	ag_solanago "github.com/gagliardetto/solana-go"
)

// Program ID
var ProgramID = ag_solanago.MustPublicKeyFromBase58("C4Xn6ACR3XmTwpWm2fN82QSAx23TQkwVW8wro9Hdpqef")

// Side represents order side (matches Solana program enum: Bid=0, Ask=1)
type Side uint8

const (
	SideBid Side = 0 // Buy (Bid)
	SideAsk Side = 1 // Sell (Ask)
)

// TokenSide represents which token to deposit/withdraw
type TokenSide uint8

const (
	TokenSideBase  TokenSide = 0 // Base token
	TokenSideQuote TokenSide = 1 // Quote token
)

// SelfTradeBehavior represents self-trade behavior (matches Solana program enum)
type SelfTradeBehavior uint8

const (
	SelfTradeDecrementTake SelfTradeBehavior = 0 // DecrementTake
	SelfTradeCancelNew     SelfTradeBehavior = 1 // CancelNew
)

// PriceOracleType represents the type of price oracle (matches Solana program enum)
type PriceOracleType uint8

const (
	PriceOracleNone   PriceOracleType = 0 // No price verification
	PriceOraclePyth   PriceOracleType = 1 // Use Pyth price feed
	PriceOracleCustom PriceOracleType = 2 // Use custom maintained price
)

// PlaceOrderParams contains parameters for placing an order
type PlaceOrderParams struct {
	Side              Side
	PriceLots         uint64
	QtyLots           uint64
	ExpirySlot        uint64
	MinFillBps        *uint16 // optional
	SelfTradeBehavior SelfTradeBehavior
}

// PlaceOrderAccounts contains accounts for place_order instruction
type PlaceOrderAccounts struct {
	Config        ag_solanago.PublicKey
	Market        ag_solanago.PublicKey
	Margin        ag_solanago.PublicKey
	Order         ag_solanago.PublicKey
	Owner         ag_solanago.PublicKey
	SystemProgram ag_solanago.PublicKey
}

// CancelOrderAccounts contains accounts for cancel_order instruction
type CancelOrderAccounts struct {
	Config ag_solanago.PublicKey
	Market ag_solanago.PublicKey
	Margin ag_solanago.PublicKey
	Order  ag_solanago.PublicKey
	Owner  ag_solanago.PublicKey
}

// DerivePDAs derives all PDAs for limit order operations
func DerivePDAs(
	baseMint ag_solanago.PublicKey,
	quoteMint ag_solanago.PublicKey,
	owner ag_solanago.PublicKey,
	orderSeqNum uint64,
) (
	config ag_solanago.PublicKey,
	market ag_solanago.PublicKey,
	margin ag_solanago.PublicKey,
	order ag_solanago.PublicKey,
	err error,
) {
	// 1. Config PDA: ["config"]
	config, _, err = ag_solanago.FindProgramAddress(
		[][]byte{[]byte("config")},
		ProgramID,
	)
	if err != nil {
		return
	}

	// 2. Market PDA: ["LimitOrderMarket", base_mint, quote_mint]
	market, _, err = ag_solanago.FindProgramAddress(
		[][]byte{
			[]byte("LimitOrderMarket"),
			baseMint[:],
			quoteMint[:],
		},
		ProgramID,
	)
	if err != nil {
		return
	}

	// 3. Margin PDA: ["margin", market, owner]
	margin, _, err = ag_solanago.FindProgramAddress(
		[][]byte{
			[]byte("margin"),
			market[:],
			owner[:],
		},
		ProgramID,
	)
	if err != nil {
		return
	}

	// 4. Order PDA: ["order", market, owner, order_id]
	orderIdBytes := make([]byte, 8)
	binary.LittleEndian.PutUint64(orderIdBytes, orderSeqNum)
	order, _, err = ag_solanago.FindProgramAddress(
		[][]byte{
			[]byte("order"),
			market[:],
			owner[:],
			orderIdBytes,
		},
		ProgramID,
	)
	if err != nil {
		return
	}

	return
}

// NewPlaceOrderInstruction creates a place_order instruction
func NewPlaceOrderInstruction(
	accounts PlaceOrderAccounts,
	params PlaceOrderParams,
	whitelistProof [][32]byte,
) (ag_solanago.Instruction, error) {
	// Discriminator for place_order
	discriminator := []byte{51, 194, 155, 175, 109, 130, 96, 106}

	// Serialize parameters
	data := make([]byte, 0, 256)
	data = append(data, discriminator...)

	// Serialize PlaceOrderParams
	data = append(data, byte(params.Side))

	priceLots := make([]byte, 8)
	binary.LittleEndian.PutUint64(priceLots, params.PriceLots)
	data = append(data, priceLots...)

	qtyLots := make([]byte, 8)
	binary.LittleEndian.PutUint64(qtyLots, params.QtyLots)
	data = append(data, qtyLots...)

	expirySlot := make([]byte, 8)
	binary.LittleEndian.PutUint64(expirySlot, params.ExpirySlot)
	data = append(data, expirySlot...)

	// Option<u16> for min_fill_bps
	if params.MinFillBps != nil {
		data = append(data, 1) // Some
		minFillBps := make([]byte, 2)
		binary.LittleEndian.PutUint16(minFillBps, *params.MinFillBps)
		data = append(data, minFillBps...)
	} else {
		data = append(data, 0) // None
	}

	data = append(data, byte(params.SelfTradeBehavior))

	// Serialize whitelist_proof (Vec<[u8; 32]>)
	// Length prefix
	proofLenBytes := make([]byte, 4)
	binary.LittleEndian.PutUint32(proofLenBytes, uint32(len(whitelistProof)))
	data = append(data, proofLenBytes...)

	// Proof hashes
	for _, hash := range whitelistProof {
		data = append(data, hash[:]...)
	}

	// Build instruction
	accounts_list := []*ag_solanago.AccountMeta{
		{PublicKey: accounts.Config, IsWritable: false, IsSigner: false},
		{PublicKey: accounts.Market, IsWritable: true, IsSigner: false},
		{PublicKey: accounts.Margin, IsWritable: true, IsSigner: false},
		{PublicKey: accounts.Order, IsWritable: true, IsSigner: false},
		{PublicKey: accounts.Owner, IsWritable: true, IsSigner: true},
		{PublicKey: accounts.SystemProgram, IsWritable: false, IsSigner: false},
	}

	return ag_solanago.NewInstruction(
		ProgramID,
		accounts_list,
		data,
	), nil
}

// NewCancelOrderInstruction creates a cancel_order instruction
func NewCancelOrderInstruction(
	accounts CancelOrderAccounts,
) (ag_solanago.Instruction, error) {
	// Discriminator for cancel_order
	discriminator := []byte{95, 129, 237, 240, 8, 49, 223, 132}

	// Build instruction (no parameters needed)
	accounts_list := []*ag_solanago.AccountMeta{
		{PublicKey: accounts.Config, IsWritable: false, IsSigner: false},
		{PublicKey: accounts.Market, IsWritable: false, IsSigner: false},
		{PublicKey: accounts.Margin, IsWritable: true, IsSigner: false},
		{PublicKey: accounts.Order, IsWritable: true, IsSigner: false},
		{PublicKey: accounts.Owner, IsWritable: false, IsSigner: true},
	}

	return ag_solanago.NewInstruction(
		ProgramID,
		accounts_list,
		discriminator,
	), nil
}

// ConvertPriceToLots converts display price to lots (rounds to nearest tick)
func ConvertPriceToLots(priceDisplay float64, tickSize uint64) uint64 {
	if tickSize == 0 {
		return 0
	}
	// Round to nearest tick
	lots := priceDisplay * float64(tickSize)
	return uint64(lots + 0.5) // Round to nearest integer
}

// ConvertQtyToLots converts display quantity to lots (rounds down)
func ConvertQtyToLots(qtyDisplay float64, minBaseLot uint64) uint64 {
	if minBaseLot == 0 {
		return 0
	}
	// Round down to ensure we don't exceed available balance
	return uint64(qtyDisplay * float64(minBaseLot))
}

// ConvertLotsToPrice converts lots to display price
func ConvertLotsToPrice(priceLots uint64, tickSize uint64) float64 {
	return float64(priceLots) / float64(tickSize)
}

// ConvertLotsToQty converts lots to display quantity
func ConvertLotsToQty(qtyLots uint64, minBaseLot uint64) float64 {
	return float64(qtyLots) / float64(minBaseLot)
}

// DeriveVaultPDAs derives vault PDAs for a market
func DeriveVaultPDAs(
	market ag_solanago.PublicKey,
	baseMint ag_solanago.PublicKey,
	quoteMint ag_solanago.PublicKey,
) (
	baseVault ag_solanago.PublicKey,
	quoteVault ag_solanago.PublicKey,
	err error,
) {
	// Base Vault PDA: ["MarketVault", market, base_mint]
	baseVault, _, err = ag_solanago.FindProgramAddress(
		[][]byte{
			[]byte("MarketVault"),
			market[:],
			baseMint[:],
		},
		ProgramID,
	)
	if err != nil {
		return
	}

	// Quote Vault PDA: ["MarketVault", market, quote_mint]
	quoteVault, _, err = ag_solanago.FindProgramAddress(
		[][]byte{
			[]byte("MarketVault"),
			market[:],
			quoteMint[:],
		},
		ProgramID,
	)
	if err != nil {
		return
	}

	return
}

// InitVaultsAccounts contains accounts for init_vaults instruction
type InitVaultsAccounts struct {
	Market        ag_solanago.PublicKey
	Payer         ag_solanago.PublicKey
	BaseMint      ag_solanago.PublicKey
	QuoteMint     ag_solanago.PublicKey
	BaseVault     ag_solanago.PublicKey
	QuoteVault    ag_solanago.PublicKey
	TokenProgram  ag_solanago.PublicKey
	SystemProgram ag_solanago.PublicKey
	Rent          ag_solanago.PublicKey
}

// NewInitVaultsInstruction creates an init_vaults instruction
func NewInitVaultsInstruction(
	accounts InitVaultsAccounts,
) (ag_solanago.Instruction, error) {
	// Discriminator for init_vaults
	discriminator := []byte{250, 62, 242, 50, 163, 133, 108, 93}

	// Build instruction (no parameters needed)
	accounts_list := []*ag_solanago.AccountMeta{
		{PublicKey: accounts.Market, IsWritable: false, IsSigner: false},
		{PublicKey: accounts.Payer, IsWritable: true, IsSigner: true},
		{PublicKey: accounts.BaseMint, IsWritable: false, IsSigner: false},
		{PublicKey: accounts.QuoteMint, IsWritable: false, IsSigner: false},
		{PublicKey: accounts.BaseVault, IsWritable: true, IsSigner: false},
		{PublicKey: accounts.QuoteVault, IsWritable: true, IsSigner: false},
		{PublicKey: accounts.TokenProgram, IsWritable: false, IsSigner: false},
		{PublicKey: accounts.SystemProgram, IsWritable: false, IsSigner: false},
		{PublicKey: accounts.Rent, IsWritable: false, IsSigner: false},
	}

	return ag_solanago.NewInstruction(
		ProgramID,
		accounts_list,
		discriminator,
	), nil
}

// DepositAccounts contains accounts for deposit instruction
type DepositAccounts struct {
	Config         ag_solanago.PublicKey
	Market         ag_solanago.PublicKey
	Margin         ag_solanago.PublicKey
	Owner          ag_solanago.PublicKey
	BaseMint       ag_solanago.PublicKey
	QuoteMint      ag_solanago.PublicKey
	UserBaseToken  ag_solanago.PublicKey
	UserQuoteToken ag_solanago.PublicKey
	BaseVault      ag_solanago.PublicKey
	QuoteVault     ag_solanago.PublicKey
	TokenProgram   ag_solanago.PublicKey
}

// WithdrawAccounts contains accounts for withdraw instruction
type WithdrawAccounts struct {
	Config         ag_solanago.PublicKey
	Market         ag_solanago.PublicKey
	Margin         ag_solanago.PublicKey
	Owner          ag_solanago.PublicKey
	UserBaseToken  ag_solanago.PublicKey
	UserQuoteToken ag_solanago.PublicKey
	BaseVault      ag_solanago.PublicKey
	QuoteVault     ag_solanago.PublicKey
	TokenProgram   ag_solanago.PublicKey
}

// NewDepositInstruction creates a deposit instruction
func NewDepositInstruction(
	accounts DepositAccounts,
	amount uint64,
	side TokenSide,
	whitelistProof [][32]byte,
) (ag_solanago.Instruction, error) {
	// Discriminator for deposit
	discriminator := []byte{242, 35, 198, 137, 82, 225, 242, 182}

	// Serialize parameters
	data := make([]byte, 0, 256)
	data = append(data, discriminator...)

	// Amount (u64)
	amountBytes := make([]byte, 8)
	binary.LittleEndian.PutUint64(amountBytes, amount)
	data = append(data, amountBytes...)

	// Side (TokenSide enum)
	data = append(data, byte(side))

	// Serialize whitelist_proof (Vec<[u8; 32]>)
	proofLenBytes := make([]byte, 4)
	binary.LittleEndian.PutUint32(proofLenBytes, uint32(len(whitelistProof)))
	data = append(data, proofLenBytes...)

	for _, hash := range whitelistProof {
		data = append(data, hash[:]...)
	}

	// Build instruction
	accounts_list := []*ag_solanago.AccountMeta{
		{PublicKey: accounts.Config, IsWritable: false, IsSigner: false},
		{PublicKey: accounts.Market, IsWritable: false, IsSigner: false},
		{PublicKey: accounts.Margin, IsWritable: true, IsSigner: false},
		{PublicKey: accounts.Owner, IsWritable: true, IsSigner: true},
		{PublicKey: accounts.BaseMint, IsWritable: false, IsSigner: false},
		{PublicKey: accounts.QuoteMint, IsWritable: false, IsSigner: false},
		{PublicKey: accounts.UserBaseToken, IsWritable: true, IsSigner: false},
		{PublicKey: accounts.UserQuoteToken, IsWritable: true, IsSigner: false},
		{PublicKey: accounts.BaseVault, IsWritable: true, IsSigner: false},
		{PublicKey: accounts.QuoteVault, IsWritable: true, IsSigner: false},
		{PublicKey: accounts.TokenProgram, IsWritable: false, IsSigner: false},
	}

	return ag_solanago.NewInstruction(
		ProgramID,
		accounts_list,
		data,
	), nil
}

// NewWithdrawInstruction creates a withdraw instruction
func NewWithdrawInstruction(
	accounts WithdrawAccounts,
	amount uint64,
	side TokenSide,
) (ag_solanago.Instruction, error) {
	// Discriminator for withdraw
	discriminator := []byte{183, 18, 70, 156, 148, 109, 161, 34}

	// Serialize parameters
	data := make([]byte, 0, 32)
	data = append(data, discriminator...)

	// Amount (u64)
	amountBytes := make([]byte, 8)
	binary.LittleEndian.PutUint64(amountBytes, amount)
	data = append(data, amountBytes...)

	// Side (TokenSide enum)
	data = append(data, byte(side))

	// Build instruction (matching IDL - no base_mint/quote_mint in withdraw accounts)
	accounts_list := []*ag_solanago.AccountMeta{
		{PublicKey: accounts.Config, IsWritable: false, IsSigner: false},
		{PublicKey: accounts.Market, IsWritable: false, IsSigner: false},
		{PublicKey: accounts.Margin, IsWritable: true, IsSigner: false},
		{PublicKey: accounts.Owner, IsWritable: false, IsSigner: true},
		{PublicKey: accounts.UserBaseToken, IsWritable: true, IsSigner: false},
		{PublicKey: accounts.UserQuoteToken, IsWritable: true, IsSigner: false},
		{PublicKey: accounts.BaseVault, IsWritable: true, IsSigner: false},
		{PublicKey: accounts.QuoteVault, IsWritable: true, IsSigner: false},
		{PublicKey: accounts.TokenProgram, IsWritable: false, IsSigner: false},
	}

	return ag_solanago.NewInstruction(
		ProgramID,
		accounts_list,
		data,
	), nil
}

// CloseOrderAccounts contains accounts for close_order instruction
type CloseOrderAccounts struct {
	Order ag_solanago.PublicKey
	Owner ag_solanago.PublicKey
}

// NewCloseOrderInstruction creates a close_order instruction
func NewCloseOrderInstruction(
	accounts CloseOrderAccounts,
) (ag_solanago.Instruction, error) {
	// Discriminator for close_order (from IDL)
	discriminator := []byte{90, 103, 209, 28, 7, 63, 168, 4}

	// Build instruction (no parameters needed)
	accounts_list := []*ag_solanago.AccountMeta{
		{PublicKey: accounts.Order, IsWritable: true, IsSigner: false},
		{PublicKey: accounts.Owner, IsWritable: true, IsSigner: true},
	}

	return ag_solanago.NewInstruction(
		ProgramID,
		accounts_list,
		discriminator,
	), nil
}

// UpdateCustomPriceAccounts contains accounts for update_custom_price instruction
type UpdateCustomPriceAccounts struct {
	Config ag_solanago.PublicKey
	Admin  ag_solanago.PublicKey
	Market ag_solanago.PublicKey
}

// NewUpdateCustomPriceInstruction creates an update_custom_price instruction
func NewUpdateCustomPriceInstruction(
	accounts UpdateCustomPriceAccounts,
	price int64,
	exponent int32,
	conf uint64,
) (ag_solanago.Instruction, error) {
	// Discriminator for update_custom_price (from IDL)
	discriminator := []byte{72, 96, 171, 76, 174, 64, 158, 142}

	// Serialize parameters
	data := make([]byte, 0, 32)
	data = append(data, discriminator...)

	// Price (i64)
	priceBytes := make([]byte, 8)
	binary.LittleEndian.PutUint64(priceBytes, uint64(price))
	data = append(data, priceBytes...)

	// Exponent (i32)
	exponentBytes := make([]byte, 4)
	binary.LittleEndian.PutUint32(exponentBytes, uint32(exponent))
	data = append(data, exponentBytes...)

	// Conf (u64)
	confBytes := make([]byte, 8)
	binary.LittleEndian.PutUint64(confBytes, conf)
	data = append(data, confBytes...)

	// Build instruction
	accounts_list := []*ag_solanago.AccountMeta{
		{PublicKey: accounts.Config, IsWritable: false, IsSigner: false},
		{PublicKey: accounts.Admin, IsWritable: false, IsSigner: true},
		{PublicKey: accounts.Market, IsWritable: true, IsSigner: false},
	}

	return ag_solanago.NewInstruction(
		ProgramID,
		accounts_list,
		data,
	), nil
}

// CreateMarginAccounts contains accounts for create_margin instruction
type CreateMarginAccounts struct {
	Config                 ag_solanago.PublicKey
	Market                 ag_solanago.PublicKey
	Margin                 ag_solanago.PublicKey
	Owner                  ag_solanago.PublicKey
	BaseMint               ag_solanago.PublicKey
	QuoteMint              ag_solanago.PublicKey
	UserBaseToken          ag_solanago.PublicKey
	UserQuoteToken         ag_solanago.PublicKey
	TokenProgram           ag_solanago.PublicKey
	AssociatedTokenProgram ag_solanago.PublicKey
	SystemProgram          ag_solanago.PublicKey
}

// NewCreateMarginInstruction creates a create_margin instruction
func NewCreateMarginInstruction(
	accounts CreateMarginAccounts,
	whitelistProof [][32]byte,
) (ag_solanago.Instruction, error) {
	// Discriminator for create_margin (from IDL)
	discriminator := []byte{19, 155, 72, 104, 164, 192, 3, 68}

	// Serialize parameters
	data := make([]byte, 0, 256)
	data = append(data, discriminator...)

	// Serialize whitelist_proof (Vec<[u8; 32]>)
	proofLenBytes := make([]byte, 4)
	binary.LittleEndian.PutUint32(proofLenBytes, uint32(len(whitelistProof)))
	data = append(data, proofLenBytes...)

	for _, hash := range whitelistProof {
		data = append(data, hash[:]...)
	}

	// Build instruction
	accounts_list := []*ag_solanago.AccountMeta{
		{PublicKey: accounts.Config, IsWritable: false, IsSigner: false},
		{PublicKey: accounts.Market, IsWritable: false, IsSigner: false},
		{PublicKey: accounts.Margin, IsWritable: true, IsSigner: false},
		{PublicKey: accounts.Owner, IsWritable: true, IsSigner: true},
		{PublicKey: accounts.BaseMint, IsWritable: false, IsSigner: false},
		{PublicKey: accounts.QuoteMint, IsWritable: false, IsSigner: false},
		{PublicKey: accounts.UserBaseToken, IsWritable: true, IsSigner: false},
		{PublicKey: accounts.UserQuoteToken, IsWritable: true, IsSigner: false},
		{PublicKey: accounts.TokenProgram, IsWritable: false, IsSigner: false},
		{PublicKey: accounts.AssociatedTokenProgram, IsWritable: false, IsSigner: false},
		{PublicKey: accounts.SystemProgram, IsWritable: false, IsSigner: false},
	}

	return ag_solanago.NewInstruction(
		ProgramID,
		accounts_list,
		data,
	), nil
}
