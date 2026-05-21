package logic

import (
	"context"
	"errors"
	"fmt"

	"github.com/shopspring/decimal"
	"github.com/zeromicro/go-zero/core/logx"
	"gorm.io/gorm"
	"richcode.cc/dex/market/internal/svc"
	"richcode.cc/dex/market/market"
	"richcode.cc/dex/model/solmodel"
	"richcode.cc/dex/pkg/constants"
)

// QuoteClmmLogic 提供 Raydium CLMM 池子的报价计算
// 使用当前价格和储备量进行简化计算（后续可以优化为基于 tick 和流动性的精确计算）
type QuoteClmmLogic struct {
	ctx    context.Context
	svcCtx *svc.ServiceContext
	logx.Logger
}

func NewQuoteClmmLogic(ctx context.Context, svcCtx *svc.ServiceContext) *QuoteClmmLogic {
	return &QuoteClmmLogic{
		ctx:    ctx,
		svcCtx: svcCtx,
		Logger: logx.WithContext(ctx),
	}
}

func (l *QuoteClmmLogic) QuoteClmm(in *market.QuoteClmmRequest) (*market.QuoteClmmResponse, error) {
	if in == nil || in.PoolState == "" {
		return nil, fmt.Errorf("pool_state is required")
	}
	if in.ChainId == 0 {
		in.ChainId = constants.SolChainIdInt
	}

	// Try V2 first, then V1
	poolV2Model := solmodel.NewClmmPoolInfoV2Model(l.svcCtx.DB)
	poolV2, err := poolV2Model.FindOneByPoolState(l.ctx, in.PoolState)
	if err != nil && !errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, fmt.Errorf("failed to query CLMM V2 pool: %w", err)
	}

	var inputVaultMint, outputVaultMint, inputVault, outputVault string
	var tradeFeeRate int64
	var currentPrice float64

	if poolV2 != nil {
		inputVaultMint = poolV2.InputVaultMint
		outputVaultMint = poolV2.OutputVaultMint
		inputVault = poolV2.InputVault
		outputVault = poolV2.OutputVault
		tradeFeeRate = poolV2.TradeFeeRate
		currentPrice = poolV2.CurrentPrice
	} else {
		poolV1Model := solmodel.NewClmmPoolInfoV1Model(l.svcCtx.DB)
		poolV1, err := poolV1Model.FindOneByPoolState(l.ctx, in.PoolState)
		if err != nil {
			return nil, fmt.Errorf("clmm pool not found: %w", err)
		}
		if poolV1 == nil {
			return nil, fmt.Errorf("pool not found")
		}
		inputVaultMint = poolV1.InputVaultMint
		outputVaultMint = poolV1.OutputVaultMint
		inputVault = poolV1.InputVault
		outputVault = poolV1.OutputVault
		tradeFeeRate = poolV1.TradeFeeRate
		currentPrice = poolV1.CurrentPrice
	}

	if inputVaultMint == "" || outputVaultMint == "" {
		return nil, fmt.Errorf("pool info incomplete")
	}

	inputMint := inputVaultMint
	outputMint := outputVaultMint
	// 如果外部显式指定了方向，做一次兜底校验
	if in.InputMint != "" {
		inputMint = in.InputMint
	}
	if in.OutputMint != "" {
		outputMint = in.OutputMint
	}

	// 获取池子储备量
	detailLogic := NewGetPoolDetailLogic(l.ctx, l.svcCtx)
	inputReserve, outputReserve, price := detailLogic.fetchVaultReserves(in.ChainId, inputVault, outputVault)
	if inputReserve <= 0 || outputReserve <= 0 {
		return nil, fmt.Errorf("pool reserve unavailable")
	}

	// 如果数据库中的价格为 0，使用从链上获取的价格
	if currentPrice <= 0 {
		currentPrice = price
	}
	if currentPrice <= 0 {
		currentPrice = 1.0 // Fallback
	}

	// CLMM 手续费率：trade_fee_rate / 1000 (例如 800 = 0.8%)
	feeFraction := float64(tradeFeeRate) / 1000.0 / 100.0 // 转换为小数
	if feeFraction < 0 {
		feeFraction = 0
	}

	var (
		payAmount     decimal.Decimal
		recvAmount    decimal.Decimal
		minRecvAmount decimal.Decimal
		priceImpact   decimal.Decimal
		payMint       = inputMint
		recvMint      = outputMint
	)

	slippageBps := in.SlippageBps
	if slippageBps < 0 {
		slippageBps = 0
	}

	inFloat := toDecimal(in.AmountIn)
	outFloat := toDecimal(in.AmountOut)

	// 使用当前价格进行简化计算
	// 对于 CLMM，更精确的计算需要考虑流动性分布和 tick，这里使用简化版本
	spotPrice := decimal.NewFromFloat(currentPrice)

	if inFloat.GreaterThan(decimal.Zero) {
		// exact-in: 使用当前价格计算输出
		payAmount = inFloat
		// 扣除手续费后的有效输入
		dxEff := inFloat.Mul(decimal.NewFromFloat(1 - feeFraction))
		// 根据当前价格计算输出
		recvAmount = dxEff.Mul(spotPrice)
		if recvAmount.IsNegative() {
			recvAmount = decimal.Zero
		}
		// 计算价格影响（简化：基于储备量）
		if recvAmount.GreaterThan(decimal.Zero) {
			// 价格影响 = (1 - 实际价格/现货价格) * 100
			actualPrice := recvAmount.Div(inFloat)
			if spotPrice.GreaterThan(decimal.Zero) {
				priceImpact = decimal.NewFromFloat(1).Sub(actualPrice.Div(spotPrice)).Mul(decimal.NewFromInt(100))
			}
		}
	} else if outFloat.GreaterThan(decimal.Zero) {
		// exact-out: 反推所需输入
		recvAmount = outFloat
		// 根据当前价格反推输入（加上手续费）
		payAmount = outFloat.Div(spotPrice).Div(decimal.NewFromFloat(1 - feeFraction))
		if payAmount.IsNegative() {
			payAmount = decimal.Zero
		}
		// 计算价格影响
		if payAmount.GreaterThan(decimal.Zero) {
			actualPrice := recvAmount.Div(payAmount)
			if spotPrice.GreaterThan(decimal.Zero) {
				priceImpact = decimal.NewFromFloat(1).Sub(actualPrice.Div(spotPrice)).Mul(decimal.NewFromInt(100))
			}
		}
	} else {
		return nil, fmt.Errorf("amount_in or amount_out must be provided")
	}

	slippageFraction := decimal.NewFromInt(slippageBps).Div(decimal.NewFromInt(10_000)) // bps -> fraction
	minRecvAmount = recvAmount.Mul(decimal.NewFromInt(1).Sub(slippageFraction))
	if priceImpact.IsNegative() {
		priceImpact = decimal.Zero
	}

	return &market.QuoteClmmResponse{
		PayMint:          payMint,
		ReceiveMint:      recvMint,
		PayAmount:        payAmount.String(),
		ReceiveAmount:    recvAmount.String(),
		MinReceiveAmount: minRecvAmount.String(),
		PriceImpactPct:   priceImpact.String(),
		FeeRatePct:       decimal.NewFromFloat(feeFraction * 100).String(), // 转成百分比
		Price:            currentPrice,
	}, nil
}
