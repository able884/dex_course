package logic

import (
	"context"
	"fmt"
	"math/big"
	"strings"

	ag_solanago "github.com/gagliardetto/solana-go"
	"github.com/shopspring/decimal"
	"github.com/zeromicro/go-zero/core/logx"
	"richcode.cc/dex/pkg/constants"
	"richcode.cc/dex/pkg/pumpfun"
	"richcode.cc/dex/pkg/util"
	"richcode.cc/dex/trade/internal/svc"
	"richcode.cc/dex/trade/trade"
)

// SOL 代币精度常量（9位小数）
const solDecimals uint8 = 9

// QuotePumpTradeLogic 处理 PumpFun 交易报价业务逻辑
type QuotePumpTradeLogic struct {
	ctx    context.Context
	svcCtx *svc.ServiceContext
	logx.Logger
}

// NewQuotePumpTradeLogic 创建 PumpFun 交易报价逻辑处理器
func NewQuotePumpTradeLogic(ctx context.Context, svcCtx *svc.ServiceContext) *QuotePumpTradeLogic {
	return &QuotePumpTradeLogic{
		ctx:    ctx,
		svcCtx: svcCtx,
		Logger: logx.WithContext(ctx),
	}
}

// QuotePumpTrade 获取 PumpFun 代币交易报价
// 该方法负责：
// 1. 参数校验（链ID、代币地址、金额等）
// 2. 转换交易方向和资产类型
// 3. 调用 PumpFun SDK 获取报价
// 4. 格式化返回结果（支付金额、接收金额、价格影响等）
func (l *QuotePumpTradeLogic) QuotePumpTrade(in *trade.QuotePumpTradeRequest) (*trade.QuotePumpTradeResponse, error) {
	// 1. 校验请求不能为空
	if in == nil {
		return nil, fmt.Errorf("empty request")
	}

	// 2. 校验 Solana 交易管理器是否初始化
	if err := l.ensureSolanaEnabled(); err != nil {
		return nil, err
	}

	// 3. 校验链ID是否支持
	if err := l.validateChain(in.ChainId); err != nil {
		return nil, err
	}

	// 4. 校验代币地址
	tokenMint := strings.TrimSpace(in.TokenMint)
	if tokenMint == "" {
		return nil, fmt.Errorf("token_mint is required")
	}

	// 5. 校验金额
	if in.Amount == "" {
		return nil, fmt.Errorf("amount is required")
	}

	// 6. 校验代币精度（最大支持18位小数）
	if in.TokenDecimal > 18 {
		return nil, fmt.Errorf("token decimal %d unsupported", in.TokenDecimal)
	}

	// 7. 解析代币 mint 地址
	mint, err := ag_solanago.PublicKeyFromBase58(tokenMint)
	if err != nil {
		return nil, fmt.Errorf("invalid token mint: %w", err)
	}

	// 8. 转换交易方向（买/卖）
	direction, err := convertDirection(in.SwapType)
	if err != nil {
		return nil, err
	}

	// 9. 转换输入资产类型（SOL/代币）
	inputAsset, err := convertAsset(in.InputAsset)
	if err != nil {
		return nil, err
	}

	// 10. 获取输入资产精度并转换金额
	inputDecimals := decimalsByAsset(inputAsset, uint8(in.TokenDecimal))
	amountIn, err := util.ConverString2Uint64(strings.TrimSpace(in.Amount), inputDecimals)
	if err != nil {
		return nil, fmt.Errorf("invalid amount: %w", err)
	}
	if amountIn == 0 {
		return nil, fmt.Errorf("amount must be greater than zero")
	}

	// 11. 构建报价参数并调用 PumpFun SDK
	params := pumpfun.QuoteParams{
		Direction:   direction,
		InputAsset:  inputAsset,
		Amount:      amountIn,
		SlippageBps: in.SlippageBps,
	}
	quote, err := pumpfun.QuotePumpBondingCurve(l.ctx, l.svcCtx.SolTxMananger.Client, mint, params)
	if err != nil {
		return nil, err
	}

	// 12. 获取支付和接收资产的精度
	payDecimals := decimalsByAsset(quote.PayAsset, uint8(in.TokenDecimal))
	recvDecimals := decimalsByAsset(quote.ReceiveAsset, uint8(in.TokenDecimal))

	// 13. 构建响应对象
	resp := &trade.QuotePumpTradeResponse{
		PayAsset:              revertAsset(quote.PayAsset),              // 支付资产类型
		ReceiveAsset:          revertAsset(quote.ReceiveAsset),          // 接收资产类型
		PayAmount:             formatAmount(quote.PayAmount, payDecimals),             // 支付金额
		PayAmountWithSlippage: formatAmount(quote.PayAmountWithLimit, payDecimals),    // 含滑点的支付金额
		ReceiveAmount:         formatAmount(quote.ReceiveAmount, recvDecimals),         // 接收金额
		MinReceiveAmount:      formatAmount(quote.MinReceiveAmount, recvDecimals),      // 最小接收金额（考虑滑点）
		PriceImpactPct:        formatPriceImpact(quote.PriceImpactBps),  // 价格影响百分比
		TokenDecimal:          in.TokenDecimal,                          // 代币精度
		SolDecimal:            uint32(solDecimals),                      // SOL 精度
	}
	return resp, nil
}

// ensureSolanaEnabled 校验 Solana 交易管理器是否已初始化
func (l *QuotePumpTradeLogic) ensureSolanaEnabled() error {
	if l.svcCtx.SolTxMananger == nil || l.svcCtx.SolTxMananger.Client == nil {
		return fmt.Errorf("sol tx manager is not initialized")
	}
	return nil
}

// validateChain 校验链ID是否支持（仅支持 Solana）
func (l *QuotePumpTradeLogic) validateChain(chainID int32) error {
	if chainID == int32(constants.SolChainIdInt) || chainID == 100000 {
		return nil
	}
	return fmt.Errorf("unsupported chain_id %d", chainID)
}

// convertDirection 将交易方向枚举从 trade.SwapType 转换为 pumpfun.QuoteDirection
func convertDirection(t trade.SwapType) (pumpfun.QuoteDirection, error) {
	switch t {
	case trade.SwapType_Buy:
		return pumpfun.QuoteDirectionBuy, nil
	case trade.SwapType_Sell:
		return pumpfun.QuoteDirectionSell, nil
	default:
		return pumpfun.QuoteDirectionUnknown, fmt.Errorf("swap_type %s not supported", t.String())
	}
}

// convertAsset 将资产类型枚举从 trade.QuoteAsset 转换为 pumpfun.QuoteAsset
func convertAsset(asset trade.QuoteAsset) (pumpfun.QuoteAsset, error) {
	switch asset {
	case trade.QuoteAsset_QuoteAssetSol:
		return pumpfun.QuoteAssetSOL, nil
	case trade.QuoteAsset_QuoteAssetToken:
		return pumpfun.QuoteAssetToken, nil
	default:
		return pumpfun.QuoteAssetUnknown, fmt.Errorf("input_asset %s not supported", asset.String())
	}
}

// revertAsset 将资产类型枚举从 pumpfun.QuoteAsset 转换回 trade.QuoteAsset
func revertAsset(asset pumpfun.QuoteAsset) trade.QuoteAsset {
	switch asset {
	case pumpfun.QuoteAssetSOL:
		return trade.QuoteAsset_QuoteAssetSol
	case pumpfun.QuoteAssetToken:
		return trade.QuoteAsset_QuoteAssetToken
	default:
		return trade.QuoteAsset_QuoteAssetUnknown
	}
}

// decimalsByAsset 根据资产类型获取精度
// SOL 返回9位，代币返回传入的精度值
func decimalsByAsset(asset pumpfun.QuoteAsset, tokenDecimals uint8) uint8 {
	if asset == pumpfun.QuoteAssetSOL {
		return solDecimals
	}
	return tokenDecimals
}

// formatAmount 将原始金额（最小单位）格式化为可读字符串
func formatAmount(value uint64, decimals uint8) string {
	if value == 0 {
		return "0"
	}
	num := decimal.NewFromBigInt(new(big.Int).SetUint64(value), 0)
	denom := decimal.New(1, int32(decimals))
	if denom.Equal(decimal.Zero) {
		return num.String()
	}
	return num.Div(denom).String()
}

// formatPriceImpact 将价格影响从 bps（万分之一）转换为百分比字符串
func formatPriceImpact(bps uint32) string {
	num := decimal.NewFromInt(int64(bps))
	return num.Div(decimal.NewFromInt(100)).String()
}
