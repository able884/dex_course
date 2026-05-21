package logic

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/binary"
	"errors"
	"fmt"
	"math"
	"math/big"
	"strings"
	"time"

	"richcode.cc/dex/market/market"
	"richcode.cc/dex/model/trademodel"
	"richcode.cc/dex/pkg/raydium/clmm"
	"richcode.cc/dex/pkg/raydium/clmm/idl/generated/amm_v3"
	"richcode.cc/dex/trade/internal/svc"
	"richcode.cc/dex/trade/trade"

	ag_binary "github.com/gagliardetto/binary"
	aSDK "github.com/gagliardetto/solana-go"
	ag_rpc "github.com/gagliardetto/solana-go/rpc"
	"github.com/shopspring/decimal"
	"github.com/zeromicro/go-zero/core/logx"
	"gorm.io/gorm"
)

type CreateClmmPoolLogic struct {
	ctx    context.Context
	svcCtx *svc.ServiceContext
	logx.Logger
}

const (
	// Raydium program TICK_ARRAY_SIZE=60 (see programs/amm/src/states/tick_array.rs)
	clmmTickArraySize int32 = 60
	clmmMinTickIndex  int32 = -443636
	clmmMaxTickIndex  int32 = 443636
)

func NewCreateClmmPoolLogic(ctx context.Context, svcCtx *svc.ServiceContext) *CreateClmmPoolLogic {
	return &CreateClmmPoolLogic{
		ctx:    ctx,
		svcCtx: svcCtx,
		Logger: logx.WithContext(ctx),
	}
}

func (l *CreateClmmPoolLogic) CreateClmmPool(in *trade.CreateClmmPoolRequest) (*trade.CreateClmmPoolResponse, error) {
	if err := l.validateRequest(in); err != nil {
		return nil, err
	}

	tier, err := l.pickFeeTier(in)
	if err != nil {
		return nil, err
	}

	logx.WithContext(l.ctx).Infof("CLMM fee tier picked: fee_bps=%d config_index=%d amm_config=%s program=%s input_fee_tier_bps=%d input_config_index=%d",
		tier.ValueBps, tier.ConfigIndex, tier.Address, tier.ProgramAddress, in.FeeTierBps, in.ConfigIndex)

	txBase64, err := l.buildCreateClmmPoolTx(in, tier)
	if err != nil {
		return nil, fmt.Errorf("failed to build CLMM create pool tx: %w", err)
	}

	return &trade.CreateClmmPoolResponse{
		TxHash:    "",
		TxType:    "legacy",
		TxBase64:  txBase64,
		ExpiresAt: time.Now().Add(10 * time.Minute).Unix(),
	}, nil
}

func (l *CreateClmmPoolLogic) validateRequest(in *trade.CreateClmmPoolRequest) error {
	if in == nil {
		return errors.New("request is required")
	}
	if strings.TrimSpace(in.BaseTokenMint) == "" || strings.TrimSpace(in.QuoteTokenMint) == "" {
		return errors.New("token mint required")
	}
	if strings.EqualFold(in.BaseTokenMint, in.QuoteTokenMint) {
		return errors.New("base and quote token must differ")
	}
	if strings.TrimSpace(in.UserWalletAddress) == "" {
		return errors.New("user_wallet_address required")
	}
	priceDec, err := decimal.NewFromString(strings.TrimSpace(in.InitialPrice))
	if err != nil {
		return fmt.Errorf("initial_price invalid: %w", err)
	}
	if priceDec.Cmp(decimal.Zero) <= 0 {
		return errors.New("initial_price must be greater than zero")
	}
	amt0 := strings.TrimSpace(in.GetAmount_0())
	amt1 := strings.TrimSpace(in.GetAmount_1())
	if amt0 == "" || amt1 == "" {
		return errors.New("amount_0 and amount_1 are required to initialize liquidity")
	}
	if dec, err := decimal.NewFromString(amt0); err != nil || dec.Cmp(decimal.Zero) <= 0 {
		return fmt.Errorf("amount_0 invalid: %w", err)
	}
	if dec, err := decimal.NewFromString(amt1); err != nil || dec.Cmp(decimal.Zero) <= 0 {
		return fmt.Errorf("amount_1 invalid: %w", err)
	}
	if strings.ToLower(strings.TrimSpace(in.GetRangeMode())) == "custom" {
		minStr := strings.TrimSpace(in.GetPriceMin())
		maxStr := strings.TrimSpace(in.GetPriceMax())
		minDec, err := decimal.NewFromString(minStr)
		if err != nil {
			return fmt.Errorf("price_min invalid: %w", err)
		}
		maxDec, err := decimal.NewFromString(maxStr)
		if err != nil {
			return fmt.Errorf("price_max invalid: %w", err)
		}
		if minDec.Cmp(decimal.Zero) <= 0 || maxDec.Cmp(decimal.Zero) <= 0 {
			return errors.New("price_min and price_max must be greater than zero")
		}
		if minDec.Cmp(maxDec) >= 0 {
			return errors.New("price_min must be lower than price_max")
		}
	}
	return nil
}

func (l *CreateClmmPoolLogic) pickFeeTier(in *trade.CreateClmmPoolRequest) (*trademodel.CpmmFeeTier, error) {
	var tier trademodel.CpmmFeeTier
	query := l.svcCtx.DB.WithContext(l.ctx).Where("pool_type = ?", "CLMM")
	switch {
	case in.ConfigIndex > 0:
		query = query.Where("config_index = ?", in.ConfigIndex)
	case in.FeeTierBps > 0:
		query = query.Where("value_bps = ?", in.FeeTierBps)
	default:
		query = query.Where("config_index = ?", 0)
	}
	if err := query.Order("priority_order asc").Take(&tier).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, fmt.Errorf("clmm fee tier not found (config_index=%d, fee_tier_bps=%d)", in.ConfigIndex, in.FeeTierBps)
		}
		return nil, err
	}
	return &tier, nil
}

func (l *CreateClmmPoolLogic) buildCreateClmmPoolTx(in *trade.CreateClmmPoolRequest, tier *trademodel.CpmmFeeTier) (string, error) {
	// normalize program ID
	clmm.SetDevnetProgramID()

	userWallet, err := aSDK.PublicKeyFromBase58(strings.TrimSpace(in.UserWalletAddress))
	if err != nil {
		return "", fmt.Errorf("invalid user wallet address: %w", err)
	}

	baseMint, err := aSDK.PublicKeyFromBase58(strings.TrimSpace(in.BaseTokenMint))
	if err != nil {
		return "", fmt.Errorf("invalid base token mint: %w", err)
	}
	quoteMint, err := aSDK.PublicKeyFromBase58(strings.TrimSpace(in.QuoteTokenMint))
	if err != nil {
		return "", fmt.Errorf("invalid quote token mint: %w", err)
	}

	price, err := decimal.NewFromString(strings.TrimSpace(in.InitialPrice))
	if err != nil {
		return "", fmt.Errorf("initial_price invalid: %w", err)
	}

	amount0Dec, err := decimal.NewFromString(strings.TrimSpace(in.GetAmount_0()))
	if err != nil {
		return "", fmt.Errorf("amount_0 invalid: %w", err)
	}
	amount1Dec, err := decimal.NewFromString(strings.TrimSpace(in.GetAmount_1()))
	if err != nil {
		return "", fmt.Errorf("amount_1 invalid: %w", err)
	}

	priceMode := strings.ToLower(strings.TrimSpace(in.PriceMode))
	if priceMode == "token0" {
		if price.IsZero() {
			return "", errors.New("initial_price cannot be zero")
		}
		price = decimal.NewFromInt(1).Div(price)
	}

	token0Mint := baseMint
	token1Mint := quoteMint
	priceForCalc := price
	flipped := false
	if token0Mint.String() > token1Mint.String() {
		token0Mint = quoteMint
		token1Mint = baseMint
		if priceForCalc.IsZero() {
			return "", errors.New("initial_price cannot be zero after token ordering")
		}
		priceForCalc = decimal.NewFromInt(1).Div(priceForCalc)
		flipped = true
		// amounts follow token0/token1 order; swap to stay aligned with ordered mints
		amount0Dec, amount1Dec = amount1Dec, amount0Dec
	}

	decimals0, program0, err := l.getTokenMeta(token0Mint.String(), in.ChainId)
	if err != nil {
		return "", fmt.Errorf("failed to get token0 meta: %w", err)
	}
	decimals1, program1, err := l.getTokenMeta(token1Mint.String(), in.ChainId)
	if err != nil {
		return "", fmt.Errorf("failed to get token1 meta: %w", err)
	}

	priceRaw := adjustPriceForDecimals(priceForCalc, decimals0, decimals1)
	sqrtPriceX64, err := toSqrtPriceX64(priceRaw)
	if err != nil {
		return "", fmt.Errorf("invalid price for CLMM: %w", err)
	}

	rangeMode := strings.ToLower(strings.TrimSpace(in.GetRangeMode()))
	if rangeMode == "" {
		rangeMode = "full"
	}

	priceMinDec := priceForCalc
	priceMaxDec := priceForCalc
	if rangeMode == "custom" {
		priceMinDec, err = decimal.NewFromString(strings.TrimSpace(in.GetPriceMin()))
		if err != nil {
			return "", fmt.Errorf("price_min invalid: %w", err)
		}
		priceMaxDec, err = decimal.NewFromString(strings.TrimSpace(in.GetPriceMax()))
		if err != nil {
			return "", fmt.Errorf("price_max invalid: %w", err)
		}
		priceMinDec, err = normalizePriceForOrder(priceMinDec, priceMode, flipped)
		if err != nil {
			return "", fmt.Errorf("normalize price_min failed: %w", err)
		}
		priceMaxDec, err = normalizePriceForOrder(priceMaxDec, priceMode, flipped)
		if err != nil {
			return "", fmt.Errorf("normalize price_max failed: %w", err)
		}
		// 保证区间从小到大，避免因 price_mode 转换后顺序反转导致 tick 计算失败
		if priceMinDec.Cmp(priceMaxDec) > 0 {
			priceMinDec, priceMaxDec = priceMaxDec, priceMinDec
		}
	}

	// 计算实际价格区间的tick的索引位置
	tickLower, tickUpper, err := deriveTicks(priceMinDec, priceMaxDec, tier.TickSpacing, rangeMode)
	if err != nil {
		return "", fmt.Errorf("derive ticks failed: %w", err)
	}

	amount0Atomic, err := toAtomicAmount(amount0Dec, decimals0)
	if err != nil {
		return "", fmt.Errorf("amount_0 invalid: %w", err)
	}
	amount1Atomic, err := toAtomicAmount(amount1Dec, decimals1)
	if err != nil {
		return "", fmt.Errorf("amount_1 invalid: %w", err)
	}

	if tier == nil || strings.TrimSpace(tier.Address) == "" {
		return "", errors.New("amm config address missing for clmm tier")
	}
	ammConfig, err := aSDK.PublicKeyFromBase58(strings.TrimSpace(tier.Address))
	if err != nil {
		return "", fmt.Errorf("invalid amm config address: %w", err)
	}
	if spacing, herr := l.fetchOnchainTickSpacing(ammConfig); herr == nil && spacing > 0 {
		if int64(spacing) != tier.TickSpacing {
			logx.WithContext(l.ctx).Infof("override tick spacing from chain: db=%d chain=%d", tier.TickSpacing, spacing)
		}
		tier.TickSpacing = int64(spacing)
	}

	builder := amm_v3.NewCreatePoolInstructionBuilder().
		SetSqrtPriceX64(sqrtPriceX64).
		SetOpenTime(uint64(startTimeOrNow(in.StartTime))).
		SetPoolCreatorAccount(userWallet).
		SetAmmConfigAccount(ammConfig).
		SetTokenMint0Account(token0Mint).
		SetTokenMint1Account(token1Mint).
		SetTokenProgram0Account(program0).
		SetTokenProgram1Account(program1).
		SetSystemProgramAccount(aSDK.SystemProgramID).
		SetRentAccount(aSDK.SysVarRentPubkey)

	poolState, _, err := builder.FindPoolStateAddress(ammConfig, token0Mint, token1Mint)
	if err != nil {
		return "", fmt.Errorf("failed to derive pool_state: %w", err)
	}
	builder.SetPoolStateAccount(poolState)

	tokenVault0, _, err := builder.FindTokenVault0Address(poolState, token0Mint)
	if err != nil {
		return "", fmt.Errorf("failed to derive token_vault_0: %w", err)
	}
	builder.SetTokenVault0Account(tokenVault0)

	tokenVault1, _, err := builder.FindTokenVault1Address(poolState, token1Mint)
	if err != nil {
		return "", fmt.Errorf("failed to derive token_vault_1: %w", err)
	}
	builder.SetTokenVault1Account(tokenVault1)

	observationState, _, err := builder.FindObservationStateAddress(poolState)
	if err != nil {
		return "", fmt.Errorf("failed to derive observation_state: %w", err)
	}
	builder.SetObservationStateAccount(observationState)

	tickArrayBitmap, _, err := builder.FindTickArrayBitmapAddress(poolState)
	if err != nil {
		return "", fmt.Errorf("failed to derive tick_array_bitmap: %w", err)
	}
	builder.SetTickArrayBitmapAccount(tickArrayBitmap)

	instruction, err := builder.ValidateAndBuild()
	if err != nil {
		return "", fmt.Errorf("build create_pool instruction failed: %w", err)
	}

	instructions := []aSDK.Instruction{instruction}
	var additionalSigners []*aSDK.PrivateKey

	// Step 3: initialize liquidity by opening a position directly after pool creation
	openIx, signer, err := l.buildInitLiquidityInstruction(clmmInitLiquidityParams{
		UserWallet:    userWallet,
		PoolState:     poolState,
		TokenVault0:   tokenVault0,
		TokenVault1:   tokenVault1,
		TokenMint0:    token0Mint,
		TokenMint1:    token1Mint,
		TokenProgram0: program0,
		TokenProgram1: program1,
		Amount0:       amount0Atomic,
		Amount1:       amount1Atomic,
		TickLower:     tickLower,
		TickUpper:     tickUpper,
		RangeMode:     rangeMode,
		TickSpacing:   tier.TickSpacing,
	})
	if err != nil {
		return "", fmt.Errorf("failed to build init liquidity instruction: %w", err)
	}
	if openIx != nil {
		instructions = append(instructions, openIx)
		if signer != nil {
			additionalSigners = append(additionalSigners, signer)
		}
	}

	if l.svcCtx.SolTxMananger == nil || l.svcCtx.SolTxMananger.Client == nil {
		return "", errors.New("solana rpc client not configured")
	}
	recentBlockhash, err := l.svcCtx.SolTxMananger.Client.GetLatestBlockhash(l.ctx, ag_rpc.CommitmentFinalized)
	if err != nil {
		return "", fmt.Errorf("failed to get latest blockhash: %w", err)
	}

	tx, err := aSDK.NewTransaction(instructions, recentBlockhash.Value.Blockhash, aSDK.TransactionPayer(userWallet))
	if err != nil {
		return "", fmt.Errorf("failed to create transaction: %w", err)
	}
	tx.Signatures = make([]aSDK.Signature, int(tx.Message.Header.NumRequiredSignatures))

	if len(additionalSigners) > 0 {
		signerMap := make(map[string]*aSDK.PrivateKey, len(additionalSigners))
		for _, sk := range additionalSigners {
			if sk == nil {
				continue
			}
			// copy to avoid pointer aliasing
			keyCopy := *sk
			signerMap[keyCopy.PublicKey().String()] = &keyCopy
		}
		_, err = tx.PartialSign(func(key aSDK.PublicKey) *aSDK.PrivateKey {
			if pk, ok := signerMap[key.String()]; ok {
				return pk
			}
			return nil
		})
		if err != nil {
			return "", fmt.Errorf("failed to partial sign init liquidity: %w", err)
		}
	}

	txBytes, err := tx.MarshalBinary()
	if err != nil {
		return "", fmt.Errorf("failed to marshal transaction: %w", err)
	}

	logx.WithContext(l.ctx).Infof("Create CLMM pool tx built: pool_state=%s token0=%s token1=%s config_index=%d", poolState.String(), token0Mint.String(), token1Mint.String(), tier.ConfigIndex)

	return base64.StdEncoding.EncodeToString(txBytes), nil
}

func (l *CreateClmmPoolLogic) getTokenMeta(mint string, chainID int32) (int64, aSDK.PublicKey, error) {
	type tokenRow struct {
		Decimals int64  `gorm:"column:decimals"`
		Program  string `gorm:"column:program"`
	}
	var row tokenRow
	err := l.svcCtx.DB.WithContext(l.ctx).
		Table("allowed_tokens").
		Select("decimals, program").
		Where("chain_id = ? AND mint = ?", chainID, mint).
		Take(&row).Error

	var decs int64
	var tokenProgram aSDK.PublicKey
	if err == nil {
		decs = row.Decimals
		if strings.TrimSpace(row.Program) != "" {
			if pk, perr := aSDK.PublicKeyFromBase58(strings.TrimSpace(row.Program)); perr == nil {
				tokenProgram = pk
			}
		}
	}

	if decs == 0 {
		val, derr := l.getTokenDecimals(chainID, mint)
		if derr != nil {
			return 0, aSDK.PublicKey{}, derr
		}
		decs = val
	}
	owner, oerr := l.getMintOwner(mint)
	if oerr != nil {
		// keep best effort owner resolution without failing decimals lookup
		logx.WithContext(l.ctx).Infof("getMintOwner failed for %s: %v", mint, oerr)
	}
	if tokenProgram.IsZero() && owner.IsZero() {
		tokenProgram = aSDK.TokenProgramID
	} else if tokenProgram.IsZero() {
		tokenProgram = owner
	} else if !owner.IsZero() && tokenProgram != owner {
		// prefer on-chain owner to avoid raw constraint violations
		tokenProgram = owner
	}
	return decs, tokenProgram, nil
}

func (l *CreateClmmPoolLogic) getTokenDecimals(chainID int32, tokenMint string) (int64, error) {
	if l.svcCtx.MarketTokenClient == nil {
		return 0, errors.New("market token client not available")
	}
	resp, err := l.svcCtx.MarketTokenClient.GetTokenInfo(l.ctx, &market.GetTokenInfoRequest{
		ChainId:      int64(chainID),
		TokenAddress: tokenMint,
	})
	if err != nil {
		return 0, fmt.Errorf("failed to get token info for %s: %w", tokenMint, err)
	}
	if resp == nil {
		return 0, fmt.Errorf("token info not found for %s", tokenMint)
	}
	return resp.Decimals, nil
}

func startTimeOrNow(start int64) int64 {
	if start > 0 {
		return start
	}
	return time.Now().Unix()
}

func adjustPriceForDecimals(price decimal.Decimal, dec0, dec1 int64) decimal.Decimal {
	diff := dec1 - dec0
	if diff == 0 {
		return price
	}
	scale := decimal.New(1, int32(diff))
	return price.Mul(scale)
}

func toSqrtPriceX64(price decimal.Decimal) (ag_binary.Uint128, error) {
	if price.Cmp(decimal.Zero) <= 0 {
		return ag_binary.Uint128{}, errors.New("price must be positive")
	}
	priceFloat, _ := price.BigFloat().Float64()
	if math.IsNaN(priceFloat) || math.IsInf(priceFloat, 0) || priceFloat <= 0 {
		return ag_binary.Uint128{}, errors.New("price is not a valid number")
	}

	sqrt := math.Sqrt(priceFloat)
	scaled := sqrt * math.Pow(2, 64)
	return floatToUint128(scaled)
}

func floatToUint128(val float64) (ag_binary.Uint128, error) {
	if val < 0 || math.IsNaN(val) || math.IsInf(val, 0) {
		return ag_binary.Uint128{}, errors.New("invalid uint128 value")
	}
	bf := new(big.Float).SetFloat64(val)
	bi, _ := bf.Int(nil)
	if bi.BitLen() > 128 {
		return ag_binary.Uint128{}, errors.New("value exceeds uint128 range")
	}

	mask := new(big.Int).SetUint64(math.MaxUint64)
	lo := new(big.Int).And(bi, mask).Uint64()
	hi := new(big.Int).Rsh(bi, 64).Uint64()

	return ag_binary.Uint128{
		Lo: lo,
		Hi: hi,
	}, nil
}

type clmmInitLiquidityParams struct {
	UserWallet    aSDK.PublicKey
	PoolState     aSDK.PublicKey
	TokenVault0   aSDK.PublicKey
	TokenVault1   aSDK.PublicKey
	TokenMint0    aSDK.PublicKey
	TokenMint1    aSDK.PublicKey
	TokenProgram0 aSDK.PublicKey
	TokenProgram1 aSDK.PublicKey
	Amount0       uint64
	Amount1       uint64
	TickLower     int32
	TickUpper     int32
	RangeMode     string
	TickSpacing   int64
}

// computeRequiredAmounts estimates the minimal token0/token1 needed for a position at current price with given tick bounds.
// Formula follows Uniswap V3/Raydium CLMM: see https://docs.uniswap.org/concepts/protocol/understanding-liquidity#calculating-token-amounts-for-a-price-range
func computeRequiredAmounts(price, lower, upper decimal.Decimal, amt0, amt1 decimal.Decimal) (decimal.Decimal, decimal.Decimal, error) {
	if price.Cmp(decimal.Zero) <= 0 || lower.Cmp(decimal.Zero) <= 0 || upper.Cmp(decimal.Zero) <= 0 {
		return decimal.Zero, decimal.Zero, errors.New("price and bounds must be positive")
	}
	if lower.Cmp(price) >= 0 || upper.Cmp(price) <= 0 {
		return decimal.Zero, decimal.Zero, errors.New("price must be within (lower, upper)")
	}
	sqrtP := decimal.NewFromFloat(math.Sqrt(price.InexactFloat64()))
	sqrtL := decimal.NewFromFloat(math.Sqrt(lower.InexactFloat64()))
	sqrtU := decimal.NewFromFloat(math.Sqrt(upper.InexactFloat64()))
	if sqrtU.Cmp(sqrtP) == 0 || sqrtP.Cmp(sqrtL) == 0 {
		return decimal.Zero, decimal.Zero, errors.New("degenerate sqrt bounds")
	}

	// Liquidity derivation from available amounts
	// L0 = amount0 * sqrtP * sqrtU / (sqrtU - sqrtP)
	L0 := amt0.Mul(sqrtP).Mul(sqrtU).Div(sqrtU.Sub(sqrtP))
	// L1 = amount1 / (sqrtP - sqrtL)
	L1 := amt1.Div(sqrtP.Sub(sqrtL))
	L := L0
	if L1.Cmp(L0) < 0 {
		L = L1
	}
	if L.Cmp(decimal.Zero) <= 0 {
		return decimal.Zero, decimal.Zero, errors.New("liquidity computed non-positive")
	}

	// Required amounts for that liquidity
	req0 := L.Mul(sqrtU.Sub(sqrtP)).Div(sqrtU.Mul(sqrtP))
	req1 := L.Mul(sqrtP.Sub(sqrtL))
	return req0, req1, nil
}

func applySlippageDecimal(val decimal.Decimal, slippageBps uint32) decimal.Decimal {
	if slippageBps == 0 {
		return val
	}
	factor := decimal.NewFromInt(int64(10000 + slippageBps)).Div(decimal.NewFromInt(10000))
	return val.Mul(factor)
}

func (l *CreateClmmPoolLogic) buildInitLiquidityInstruction(p clmmInitLiquidityParams) (aSDK.Instruction, *aSDK.PrivateKey, error) {
	// simple guard: amounts must be positive
	if p.Amount0 == 0 || p.Amount1 == 0 {
		return nil, nil, nil
	}

	tickSpacing := int32(p.TickSpacing)
	if tickSpacing <= 0 {
		tickSpacing = 1
	}

	// 根据 tick 和 tick spacing 计算对应的 tick array 的 start tick 索引
	tickArrayLowerStart := calcTickArrayStartIndex(p.TickLower, tickSpacing)
	tickArrayUpperStart := calcTickArrayStartIndex(p.TickUpper, tickSpacing)

	// amount0Max 和 amount1Max 的作用：
	// 1. 当 liquidity=0 且 base_flag=true 时，Raydium 会根据 amount0Max 计算流动性
	// 2. 然后根据流动性计算实际需要的金额，并检查是否 <= amount0Max/amount1Max
	//
	// 问题：如果使用应用滑点的值（如 100000 * 1.005 = 100500），Raydium 会根据 100500 计算流动性
	// 导致实际扣除的金额接近 100500，而不是用户期望的 100000
	//
	// 解决方案：使用原始金额，让 Raydium 根据原始金额计算流动性
	// 滑点通过设置更大的 amount0Max/amount1Max 来保证（但这里我们直接使用原始金额，因为实际金额应该接近原始金额）
	amount0Max := p.Amount0
	amount1Max := p.Amount1

	positionMint, err := aSDK.NewRandomPrivateKey()
	if err != nil {
		return nil, nil, fmt.Errorf("failed to generate position mint key: %w", err)
	}

	builder := amm_v3.NewOpenPositionWithToken22NftInstructionBuilder().
		SetTickLowerIndex(p.TickLower).
		SetTickUpperIndex(p.TickUpper).
		SetTickArrayLowerStartIndex(tickArrayLowerStart).
		SetTickArrayUpperStartIndex(tickArrayUpperStart).
		SetLiquidity(ag_binary.Uint128{}).
		SetAmount0Max(amount0Max).
		SetAmount1Max(amount1Max).
		SetWithMetadata(false).
		SetBaseFlag(true).
		SetPayerAccount(p.UserWallet).
		SetPositionNftOwnerAccount(p.UserWallet).
		SetPositionNftMintAccount(positionMint.PublicKey())

	positionNftAccount, err := findTokenAccountForProgram(p.UserWallet, positionMint.PublicKey(), aSDK.Token2022ProgramID)
	if err != nil {
		return nil, nil, fmt.Errorf("derive position nft account failed: %w", err)
	}
	builder.SetPositionNftAccountAccount(positionNftAccount)

	builder.SetPoolStateAccount(p.PoolState)
	// 根据 pool state 和 tick 范围推导出 protocol position PDA地址
	protocolPos, err := findProtocolPositionAddress(p.PoolState, p.TickLower, p.TickUpper)
	if err != nil {
		return nil, nil, fmt.Errorf("derive protocol position failed: %w", err)
	}
	builder.SetProtocolPositionAccount(protocolPos)

	// 根据前面计算的lower_tick索引，计算对应的价格账户地址
	tickArrayLower, err := findTickArrayAddress(p.PoolState, tickArrayLowerStart)
	if err != nil {
		return nil, nil, fmt.Errorf("derive tick array lower failed: %w", err)
	}
	// 根据前面计算的upper_tick索引，计算对应的价格账户地址
	tickArrayUpper, err := findTickArrayAddress(p.PoolState, tickArrayUpperStart)
	if err != nil {
		return nil, nil, fmt.Errorf("derive tick array upper failed: %w", err)
	}
	builder.SetTickArrayLowerAccount(tickArrayLower).
		SetTickArrayUpperAccount(tickArrayUpper)

	// 日志验证 tick 对齐及 PDA
	block := int32(p.TickSpacing) * clmmTickArraySize
	fmt.Println("CLMM init tick debug",
		"lower", p.TickLower,
		"upper", p.TickUpper,
		"spacing", p.TickSpacing,
		"array_size", clmmTickArraySize,
		"block", block,
		"startLower", tickArrayLowerStart,
		"startUpper", tickArrayUpperStart,
		"amount0Max", amount0Max,
		"amount1Max", amount1Max,
		"tickArrayLower", tickArrayLower.String(),
		"tickArrayUpper", tickArrayUpper.String())

	// 根据 position mint 推导出个人持仓 PDA地址
	personalPos, err := findPersonalPositionPDA(positionMint.PublicKey())
	if err != nil {
		return nil, nil, fmt.Errorf("derive personal position failed: %w", err)
	}
	builder.SetPersonalPositionAccount(personalPos)

	tokenAccount0, _, err := aSDK.FindAssociatedTokenAddress(p.UserWallet, p.TokenMint0)
	if err != nil {
		return nil, nil, fmt.Errorf("derive token account0 failed: %w", err)
	}
	tokenAccount1, _, err := aSDK.FindAssociatedTokenAddress(p.UserWallet, p.TokenMint1)
	if err != nil {
		return nil, nil, fmt.Errorf("derive token account1 failed: %w", err)
	}

	builder.SetTokenAccount0Account(tokenAccount0).
		SetTokenAccount1Account(tokenAccount1).
		SetTokenVault0Account(p.TokenVault0).
		SetTokenVault1Account(p.TokenVault1).
		SetRentAccount(aSDK.SysVarRentPubkey).
		SetSystemProgramAccount(aSDK.SystemProgramID).
		SetAssociatedTokenProgramAccount(amm_v3.Addresses["ATokenGPvbdGVxr1b2hvZbsiqW5xWH25efTNsLJA8knL"])

	tokenProgram := aSDK.TokenProgramID
	if !p.TokenProgram0.IsZero() {
		tokenProgram = p.TokenProgram0
	}
	// open_position_with_token22_nft 要求 token_program_2022 固定为 Token-2022
	tokenProgram2022 := aSDK.Token2022ProgramID

	builder.SetTokenProgramAccount(tokenProgram).
		SetTokenProgram2022Account(tokenProgram2022).
		SetVault0MintAccount(p.TokenMint0).
		SetVault1MintAccount(p.TokenMint1)

	instruction, err := builder.ValidateAndBuild()
	if err != nil {
		return nil, nil, err
	}

	return instruction, &positionMint, nil
}

func normalizePriceForOrder(p decimal.Decimal, priceMode string, flipped bool) (decimal.Decimal, error) {
	if p.Cmp(decimal.Zero) <= 0 {
		return decimal.Zero, errors.New("price must be positive")
	}
	price := p
	if strings.ToLower(strings.TrimSpace(priceMode)) == "token0" {
		if price.IsZero() {
			return decimal.Zero, errors.New("price cannot be zero for token0 mode")
		}
		price = decimal.NewFromInt(1).Div(price)
	}
	if flipped {
		if price.IsZero() {
			return decimal.Zero, errors.New("price cannot be zero after flip")
		}
		price = decimal.NewFromInt(1).Div(price)
	}
	return price, nil
}

func deriveTicks(priceMin, priceMax decimal.Decimal, tickSpacing int64, rangeMode string) (int32, int32, error) {
	if strings.ToLower(strings.TrimSpace(rangeMode)) != "custom" {
		return clmmMinTickIndex, clmmMaxTickIndex, nil
	}
	// 根据价格下限计算实际的下限 tick = ln(price) / ln(1.0001)
	minTick, err := priceToTick(priceMin)
	if err != nil {
		return 0, 0, err
	}
	// 根据价格上限计算实际的下限 tick = ln(price) / ln(1.0001)
	maxTick, err := priceToTick(priceMax)
	if err != nil {
		return 0, 0, err
	}
	// 将tick计算在区间的开始或者结束索引，如：tick=2，spc=10，应该lower=0，
	lower := alignTickToSpacing(minTick, tickSpacing, false)
	// 将tick计算在区间的开始或者结束索引，如：tick=6，spc=10，应该lower=10
	upper := alignTickToSpacing(maxTick, tickSpacing, true)
	if lower < clmmMinTickIndex {
		lower = clmmMinTickIndex
	}
	if upper > clmmMaxTickIndex {
		upper = clmmMaxTickIndex
	}
	if lower >= upper {
		return 0, 0, errors.New("invalid tick range after alignment")
	}
	return lower, upper, nil
}

func alignTickToSpacing(tick int32, spacing int64, upward bool) int32 {
	sp := int32(spacing)
	if sp <= 0 {
		return tick
	}
	rem := tick % sp
	if rem == 0 {
		return tick
	}
	if upward {
		if tick >= 0 {
			return tick + (sp - rem)
		}
		return tick - rem
	}
	if tick >= 0 {
		return tick - rem
	}
	return tick - rem - sp
}

func priceToTick(price decimal.Decimal) (int32, error) {
	if price.Cmp(decimal.Zero) <= 0 {
		return 0, errors.New("price must be positive")
	}
	f, _ := price.BigFloat().Float64()
	if f <= 0 || math.IsNaN(f) || math.IsInf(f, 0) {
		return 0, errors.New("price not finite")
	}
	tick := math.Log(f) / math.Log(1.0001)
	return int32(math.Floor(tick)), nil
}

func calcTickArrayStartIndex(tick int32, spacing int32) int32 {
	if spacing <= 0 {
		spacing = 1
	}
	block := spacing * clmmTickArraySize
	quot := tick / block
	if tick < 0 && tick%block != 0 {
		quot--
	}
	// start_index uses the tick array start tick (array index * block size)
	return quot * block
}

func toAtomicAmount(amount decimal.Decimal, decimals int64) (uint64, error) {
	if amount.Cmp(decimal.Zero) <= 0 {
		return 0, errors.New("amount must be positive")
	}
	scale := decimal.NewFromFloat(math.Pow10(int(decimals)))
	val := amount.Mul(scale)
	bi := val.BigInt()
	if !bi.IsUint64() {
		return 0, errors.New("amount exceeds uint64")
	}
	return bi.Uint64(), nil
}

// 根据 poolState 和 tick 范围计算 protocol position 的 PDA 地址，供开仓指令使用
func findProtocolPositionAddress(poolState aSDK.PublicKey, tickLower, tickUpper int32) (aSDK.PublicKey, error) {
	buf := new(bytes.Buffer)
	if err := binary.Write(buf, binary.LittleEndian, tickLower); err != nil {
		return aSDK.PublicKey{}, err
	}
	if err := binary.Write(buf, binary.LittleEndian, tickUpper); err != nil {
		return aSDK.PublicKey{}, err
	}
	seeds := [][]byte{
		[]byte("protocol_position"),
		poolState.Bytes(),
		buf.Bytes(),
	}
	pda, _, err := aSDK.FindProgramAddress(seeds, amm_v3.ProgramID)
	return pda, err
}

// findTickArrayAddress derives tick_array PDA using big-endian start_index (to_be_bytes in on-chain code).
func findTickArrayAddress(poolState aSDK.PublicKey, startIndex int32) (aSDK.PublicKey, error) {
	buf := new(bytes.Buffer)
	if err := binary.Write(buf, binary.BigEndian, startIndex); err != nil {
		return aSDK.PublicKey{}, err
	}
	seeds := [][]byte{
		[]byte("tick_array"),
		poolState.Bytes(),
		buf.Bytes(),
	}
	pda, _, err := aSDK.FindProgramAddress(seeds, amm_v3.ProgramID)
	return pda, err
}

func findPersonalPositionPDA(positionMint aSDK.PublicKey) (aSDK.PublicKey, error) {
	seeds := [][]byte{
		[]byte("position"),
		positionMint.Bytes(),
	}
	pda, _, err := aSDK.FindProgramAddress(seeds, amm_v3.ProgramID)
	return pda, err
}

// findTokenAccountForProgram derives ATA for given token program (SPL or Token-2022).
func findTokenAccountForProgram(owner, mint, tokenProgram aSDK.PublicKey) (aSDK.PublicKey, error) {
	seeds := [][]byte{
		owner.Bytes(),
		tokenProgram.Bytes(),
		mint.Bytes(),
	}
	pda, _, err := aSDK.FindProgramAddress(seeds, amm_v3.Addresses["ATokenGPvbdGVxr1b2hvZbsiqW5xWH25efTNsLJA8knL"])
	return pda, err
}

// applySlippage 将用户填写的金额放大 slippage_bps，返回向上取整后的最大可用额度。
func applySlippage(amount uint64, slippageBps uint32) (uint64, error) {
	if slippageBps == 0 {
		return amount, nil
	}
	mul := new(big.Int).SetUint64(amount)
	mul = mul.Mul(mul, big.NewInt(int64(10000+slippageBps)))
	div := mul.Div(mul, big.NewInt(10000))
	if !div.IsUint64() {
		return 0, errors.New("amount exceeds uint64 after slippage")
	}
	return div.Uint64(), nil
}

// getMintOwner fetches the program owner for a mint account to choose the correct token program (SPL vs token-2022).
func (l *CreateClmmPoolLogic) getMintOwner(mint string) (aSDK.PublicKey, error) {
	if l.svcCtx == nil || l.svcCtx.SolTxMananger == nil || l.svcCtx.SolTxMananger.Client == nil {
		return aSDK.PublicKey{}, errors.New("solana rpc client not configured")
	}
	mintPK, err := aSDK.PublicKeyFromBase58(strings.TrimSpace(mint))
	if err != nil {
		return aSDK.PublicKey{}, fmt.Errorf("invalid mint: %w", err)
	}
	resp, err := l.svcCtx.SolTxMananger.Client.GetAccountInfoWithOpts(l.ctx, mintPK, &ag_rpc.GetAccountInfoOpts{
		Commitment: ag_rpc.CommitmentFinalized,
	})
	if err != nil {
		return aSDK.PublicKey{}, err
	}
	if resp == nil || resp.Value == nil {
		return aSDK.PublicKey{}, fmt.Errorf("mint account not found: %s", mint)
	}
	return resp.Value.Owner, nil
}

// fetchOnchainTickSpacing 读取链上 amm_config 的 tick_spacing，优先使用链上配置避免本地 DB 过期。
func (l *CreateClmmPoolLogic) fetchOnchainTickSpacing(ammConfig aSDK.PublicKey) (uint16, error) {
	if l.svcCtx == nil || l.svcCtx.SolTxMananger == nil || l.svcCtx.SolTxMananger.Client == nil {
		return 0, errors.New("solana rpc client not configured")
	}
	info, err := l.svcCtx.SolTxMananger.Client.GetAccountInfoWithOpts(l.ctx, ammConfig, &ag_rpc.GetAccountInfoOpts{
		Commitment: ag_rpc.CommitmentFinalized,
	})
	if err != nil {
		return 0, err
	}
	if info == nil || info.Value == nil {
		return 0, fmt.Errorf("amm_config not found: %s", ammConfig.String())
	}
	data := info.Value.Data.GetBinary()
	decoder := ag_binary.NewBorshDecoder(data)
	var acct amm_v3.AmmConfigAccount
	if err := acct.UnmarshalWithDecoder(decoder); err != nil {
		return 0, err
	}
	return acct.TickSpacing, nil
}
