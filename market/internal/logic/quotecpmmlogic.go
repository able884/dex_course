package logic

import (
	"context"
	"fmt"
	"strconv"

	"github.com/shopspring/decimal"
	"github.com/zeromicro/go-zero/core/logx"
	"richcode.cc/dex/market/internal/svc"
	"richcode.cc/dex/market/market"
	"richcode.cc/dex/model/solmodel"
	"richcode.cc/dex/pkg/constants"
)

// QuoteCpmmLogic 提供 Raydium CPMM 池子的本地报价服务
// 按恒定乘积公式 (x * y = k) + 池子手续费计算交易报价
// 默认支持 exact-in 模式（指定输入金额，计算输出），
// 如果传入 amount_out 则反推所需的 amount_in（exact-out 模式）
type QuoteCpmmLogic struct {
	ctx    context.Context              // 上下文对象，用于请求链路追踪和超时控制
	svcCtx *svc.ServiceContext          // 服务上下文，包含数据库、Redis、Solana RPC等依赖
	logx.Logger                          // 日志记录器
}

// NewQuoteCpmmLogic 创建CPMM报价逻辑处理器
// 参数:
//   ctx - 上下文对象
//   svcCtx - 服务上下文
// 返回: 初始化后的QuoteCpmmLogic指针
func NewQuoteCpmmLogic(ctx context.Context, svcCtx *svc.ServiceContext) *QuoteCpmmLogic {
	return &QuoteCpmmLogic{
		ctx:    ctx,
		svcCtx: svcCtx,
		Logger: logx.WithContext(ctx),
	}
}

// QuoteCpmm CPMM池子交易报价计算
// 根据恒定乘积公式计算交易的输入/输出金额、最小接收金额、价格影响和手续费率
// 参数:
//   in - 请求参数，包含池子状态地址、链ID、输入/输出金额、滑点等
// 返回:
//   *market.QuoteCpmmResponse - 报价响应
//   error - 错误信息
func (l *QuoteCpmmLogic) QuoteCpmm(in *market.QuoteCpmmRequest) (*market.QuoteCpmmResponse, error) {
	// 验证请求参数：池子状态地址不能为空
	if in == nil || in.PoolState == "" {
		return nil, fmt.Errorf("pool_state is required")
	}
	// 设置默认链ID为Solana主网
	if in.ChainId == 0 {
		in.ChainId = constants.SolChainIdInt
	}

	// 查询CPMM池子信息（仅支持CPMM版本3）
	pool, err := solmodel.NewCpmmPoolInfoModel(l.svcCtx.DB).FindOneByPoolState(l.ctx, in.PoolState)
	if err != nil {
		return nil, fmt.Errorf("cpmm pool not found: %w", err)
	}
	if pool == nil {
		return nil, fmt.Errorf("pool not found")
	}

	// 获取池子的输入/输出代币地址
	inputMint := pool.InputTokenMint
	outputMint := pool.OutputTokenMint
	// 如果请求中显式指定了代币方向，使用请求中的值进行兜底校验
	if in.InputMint != "" {
		inputMint = in.InputMint
	}
	if in.OutputMint != "" {
		outputMint = in.OutputMint
	}

	// 使用GetPoolDetailLogic获取金库储备量
	detailLogic := NewGetPoolDetailLogic(l.ctx, l.svcCtx)
	inputReserve, outputReserve, price := detailLogic.fetchVaultReserves(in.ChainId, pool.InputVault, pool.OutputVault)
	// 验证储备量有效性
	if inputReserve <= 0 || outputReserve <= 0 {
		return nil, fmt.Errorf("pool reserve unavailable")
	}

	// 将交易费率转换为小数形式（trade_fee_rate / 1e6）
	feeFraction := float64(pool.TradeFeeRate) / 1_000_000.0
	if feeFraction < 0 {
		feeFraction = 0  // 确保费率非负
	}

	// 声明报价计算相关变量
	var (
		payAmount     decimal.Decimal  // 需要支付的金额
		recvAmount    decimal.Decimal  // 预计收到的金额
		minRecvAmount decimal.Decimal  // 考虑滑点后的最小接收金额
		priceImpact   decimal.Decimal  // 价格影响百分比
		payMint       = inputMint      // 支付代币地址
		recvMint      = outputMint     // 接收代币地址
	)

	// 处理滑点参数，确保非负
	slippageBps := in.SlippageBps
	if slippageBps < 0 {
		slippageBps = 0
	}

	// 将字符串金额转换为decimal类型
	inFloat := toDecimal(in.AmountIn)
	outFloat := toDecimal(in.AmountOut)

	// 将储备量转换为decimal类型用于精确计算
	x := decimal.NewFromFloat(inputReserve)  // 输入代币储备
	y := decimal.NewFromFloat(outputReserve) // 输出代币储备
	spot := y.Div(x)                         // 当前现货价格 (y/x)

	// 根据输入/输出模式计算报价
	if inFloat.GreaterThan(decimal.Zero) {
		// ========== Exact-In 模式：指定输入金额，计算输出 ==========
		payAmount = inFloat
		// 计算扣除手续费后的有效输入：dx_eff = amount_in * (1 - fee)
		dxEff := inFloat.Mul(decimal.NewFromFloat(1 - feeFraction))
		// 计算新的输入储备量：x + dx_eff
		denom := x.Add(dxEff)
		if denom.IsZero() {
			return nil, fmt.Errorf("invalid reserve denominator")
		}
		// 恒定乘积公式计算输出：dy = y * dx_eff / (x + dx_eff)
		recvAmount = y.Mul(dxEff).Div(denom)
		if recvAmount.IsNegative() {
			recvAmount = decimal.Zero
		}
		// 计算价格影响：1 - (实际输出 / (输入 * 现货价格))
		if recvAmount.GreaterThan(decimal.Zero) {
			priceImpact = decimal.NewFromFloat(1).Sub(recvAmount.Div(inFloat.Mul(spot))).Mul(decimal.NewFromInt(100))
		}
	} else if outFloat.GreaterThan(decimal.Zero) {
		// ========== Exact-Out 模式：指定输出金额，反推输入 ==========
		recvAmount = outFloat
		// 反推所需的有效输入：dx_eff = dy * x / (y - dy)
		denom := y.Sub(outFloat)
		if denom.LessThanOrEqual(decimal.Zero) {
			return nil, fmt.Errorf("amount_out too large")
		}
		dxEff := outFloat.Mul(x).Div(denom)
		if dxEff.IsNegative() {
			dxEff = decimal.Zero
		}
		// 计算需要支付的金额（考虑手续费）：amount_in = dx_eff / (1 - fee)
		payAmount = dxEff.Div(decimal.NewFromFloat(1 - feeFraction))
		if payAmount.IsNegative() {
			payAmount = decimal.Zero
		}
		// 计算价格影响
		if recvAmount.GreaterThan(decimal.Zero) {
			priceImpact = decimal.NewFromFloat(1).Sub(recvAmount.Div(payAmount.Mul(spot))).Mul(decimal.NewFromInt(100))
		}
	} else {
		// 必须提供输入或输出金额
		return nil, fmt.Errorf("amount_in or amount_out must be provided")
	}

	// 计算考虑滑点后的最小接收金额
	slippageFraction := decimal.NewFromInt(slippageBps).Div(decimal.NewFromInt(10_000)) // bps -> fraction
	minRecvAmount = recvAmount.Mul(decimal.NewFromInt(1).Sub(slippageFraction))
	// 确保价格影响非负
	if priceImpact.IsNegative() {
		priceImpact = decimal.Zero
	}

	// 构建并返回报价响应
	return &market.QuoteCpmmResponse{
		PayMint:          payMint,                           // 支付代币地址
		ReceiveMint:      recvMint,                          // 接收代币地址
		PayAmount:        payAmount.String(),                 // 需要支付的金额
		ReceiveAmount:    recvAmount.String(),               // 预计收到的金额
		MinReceiveAmount: minRecvAmount.String(),            // 考虑滑点后的最小接收金额
		PriceImpactPct:   priceImpact.String(),              // 价格影响百分比
		FeeRatePct:       decimal.NewFromFloat(feeFraction * 100).String(), // 手续费率（百分比）
		Price:            price,                             // 当前价格
	}, nil
}

// toDecimal 将字符串金额转换为decimal类型
// 用于精确的金融计算，避免浮点数精度问题
// 参数:
//   s - 字符串形式的金额
// 返回: decimal.Decimal 类型的金额（如果转换失败返回Zero）
func toDecimal(s string) decimal.Decimal {
	if s == "" {
		return decimal.Zero
	}
	f, err := strconv.ParseFloat(s, 64)
	if err != nil {
		return decimal.Zero
	}
	return decimal.NewFromFloat(f)
}
