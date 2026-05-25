package logic

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"

	aSDK "github.com/gagliardetto/solana-go"
	"github.com/shopspring/decimal"
	"github.com/zeromicro/go-zero/core/logx"
	"github.com/zeromicro/go-zero/core/threading"
	"go.opentelemetry.io/otel/trace"

	"richcode.cc/dex/market/marketclient"
	"richcode.cc/dex/model/trademodel"
	"richcode.cc/dex/pkg/constants"
	trade2 "richcode.cc/dex/pkg/trade"
	"richcode.cc/dex/pkg/util"
	"richcode.cc/dex/pkg/xcode"
	"richcode.cc/dex/trade/internal/svc"
	"richcode.cc/dex/trade/trade"
)

// CreateMarketOrderLogic 处理市价订单创建业务逻辑
type CreateMarketOrderLogic struct {
	ctx    context.Context
	svcCtx *svc.ServiceContext
	logx.Logger
}

// 默认滑点设置（100 bps = 1%）
const defaultSlippageBps = 100

// NewCreateMarketOrderLogic 创建市价订单逻辑处理器
func NewCreateMarketOrderLogic(ctx context.Context, svcCtx *svc.ServiceContext) *CreateMarketOrderLogic {
	return &CreateMarketOrderLogic{
		ctx:    ctx,
		svcCtx: svcCtx,
		Logger: logx.WithContext(ctx),
	}
}

// CreateMarketOrder 创建市价订单的主入口方法
// 该方法负责：
// 1. 参数校验和标准化处理
// 2. 查询交易对信息（通过 Market 服务）
// 3. 创建订单记录到数据库
// 4. 根据链类型决定同步/异步处理策略
// 5. 返回交易哈希或未签名交易
func (l *CreateMarketOrderLogic) CreateMarketOrder(in *trade.CreateMarketOrderRequest) (*trade.CreateMarketOrderResponse, error) {
	// 1. 参数校验：解析金额并验证正数
	amountDecimal, err := decimal.NewFromString(in.AmountIn)
	if err != nil {
		return nil, err
	}
	if !amountDecimal.IsPositive() {
		return nil, xcode.AmountErr
	}

	// 2. 标准化滑点设置，默认1%
	slippageBps := in.GetSlippageBps()
	if slippageBps == 0 {
		slippageBps = defaultSlippageBps
	}

	// 3. 查询交易对信息，验证交易对有效性
	pairInfo, err := l.svcCtx.MarketClient.GetPairInfoByToken(l.ctx, &marketclient.GetPairInfoByTokenRequest{
		ChainId:      int64(in.ChainId),
		TokenAddress: in.TokenCa,
	})
	if err != nil {
		l.Errorf("获取交易对信息失败 err: %v", err)
		return nil, fmt.Errorf("获取交易对信息失败: %w", err)
	}
	if pairInfo == nil {
		return nil, fmt.Errorf("交易对信息为空，token: %s", in.TokenCa)
	}

	// 4. 验证交易对是否正常（FDV不能为0）
	if pairInfo.Fdv == 0 {
		return nil, fmt.Errorf("%s 交易对 %s 的FDV为0，无法交易", pairInfo.Name, pairInfo.Address)
	}

	// 5. 准备订单数据
	capDecimal := decimal.NewFromFloat(pairInfo.Fdv)
	model := trademodel.NewTradeOrderModel(l.svcCtx.DB)
	baseTokenPrice := decimal.NewFromFloat(pairInfo.BaseTokenPrice)
	tokenPriceUsdDecimal := decimal.NewFromFloat(pairInfo.TokenPrice)

	// 6. 安全检查：防止零价格导致的除零错误
	if baseTokenPrice.IsZero() {
		l.Errorf("代币 %s 的基础代币价格为零，尝试获取当前价格", in.TokenCa)
		if in.ChainId == constants.SolChainIdInt || in.ChainId == 100000 {
			nativePrice, err := l.svcCtx.MarketClient.GetNativeTokenPrice(l.ctx, &marketclient.GetNativeTokenPriceRequest{
				ChainId: int64(in.ChainId),
			})
			if err == nil && nativePrice.BaseTokenPriceUsd > 0 {
				l.Infof("获取SOL价格成功: %f", nativePrice.BaseTokenPriceUsd)
				baseTokenPrice = decimal.NewFromFloat(nativePrice.BaseTokenPriceUsd)
			} else {
				l.Errorf("获取原生代币价格失败: %v", err)
				return nil, fmt.Errorf("代币 %s 的基础代币价格无效（零），且无法获取当前价格", in.TokenCa)
			}
		} else {
			return nil, fmt.Errorf("代币 %s 的基础代币价格无效（零）", in.TokenCa)
		}
	}

	// 7. 计算订单价值（基础币本位）
	tokenPriceDecimal := tokenPriceUsdDecimal.Div(baseTokenPrice)
	orderValueBase := amountDecimal
	if in.SwapType == trade.SwapType_Sell {
		orderValueBase = amountDecimal.Mul(tokenPriceDecimal)
	}

	// 8. 构建订单实体
	order := &trademodel.TradeOrder{
		TradeType:      int64(trade.TradeType_Market),      // 市价订单
		ChainId:        int64(in.ChainId),                  // 链ID
		TokenCa:        in.TokenCa,                         // 交易代币地址
		SwapType:       int64(in.SwapType),                 // 交易方向（买/卖）
		IsAutoSlippage: 0,                                  // 是否自动滑点
		Slippage:       int64(slippageBps),                 // 滑点设置（bps）
		IsAntiMev:      0,                                  // 是否反MEV
		GasType:        1,                                  // Gas类型
		Status:         int64(trade.OrderStatus_Proc),      // 订单状态：处理中
		OrderCap:       capDecimal.InexactFloat64(),        // FDV
		OrderAmount:    amountDecimal.InexactFloat64(),     // 订单金额
		OrderPriceBase: tokenPriceDecimal.InexactFloat64(), // 基础币价格
		OrderValueBase: orderValueBase.InexactFloat64(),    // 订单价值（基础币）
		OrderBasePrice: baseTokenPrice.InexactFloat64(),    // 基础币价格（USD）
		DoubleOut:      util.BoolToInt64(in.DoubleOut),     // 是否翻倍出本
		DexName:        pairInfo.Name,                      // DEX名称
		PairCa:         pairInfo.Address,                   // 交易对地址
		WalletAddress:  in.UserWalletAddress,               // 用户钱包地址
	}

	// 9. 处理一键买模式
	if in.IsOneClick {
		order.TradeType = int64(trade.TradeType_OneClick)
	}

	// 10. 保存订单到数据库（带日志）
	err = model.InsertWithLog(l.ctx, order)
	if err != nil {
		l.Errorf("InsertWithLog err: %v", err)
		return nil, xcode.ServerErr
	}

	// 11. 路由决策：Solana链或买单采用同步处理，其他采用异步
	l.Infof("路由决策: ChainId=%d, SwapType=%d", order.ChainId, order.SwapType)
	if order.ChainId == constants.SolChainIdInt || order.SwapType == int64(trade.SwapType_Buy) {
		l.Infof("采用同步处理路径")
		txHash, err := l.CreateMarketTx(order, pairInfo)
		if err != nil {
			l.Errorf("CreateMarketTx 失败: %v", err)
			return nil, err
		}
		l.Infof("CreateMarketTx 成功, txHash长度: %d", len(txHash))
		return &trade.CreateMarketOrderResponse{TxHash: txHash}, nil
	}

	// 12. 异步处理路径：立即返回空txHash，后台异步处理
	l.Infof("采用异步处理路径")
	threading.GoSafe(func() {
		asynCtx := trace.ContextWithSpan(context.Background(), trace.SpanFromContext(l.ctx))
		newL := NewCreateMarketOrderLogic(asynCtx, l.svcCtx)
		_, err = newL.CreateMarketTx(order, pairInfo)
		if err != nil {
			newL.Error(err)
		}
	})
	return &trade.CreateMarketOrderResponse{TxHash: ""}, nil
}

// CreateMarketTx 创建市场交易的核心方法
// 该方法负责：
// 1. 获取交易对信息（如果未提供）
// 2. 调用底层交易构建方法
// 3. 处理交易失败时的订单状态更新
func (l *CreateMarketOrderLogic) CreateMarketTx(order *trademodel.TradeOrder, pairInfo *marketclient.GetPairInfoByTokenResponse) (string, error) {
	var err error

	// defer机制：如果订单状态为处理中且发生错误，则更新订单状态为失败
	defer func() {
		if order.Status == int64(trade.OrderStatus_Proc) && err != nil {
			err2 := l.updateDbByTxResult(order, nil, "", err)
			if err2 != nil {
				l.Error(err2)
			}
		}
	}()

	// 如果未提供交易对信息，重新查询
	if pairInfo == nil {
		pairInfo, err = l.svcCtx.MarketClient.GetPairInfoByToken(l.ctx, &marketclient.GetPairInfoByTokenRequest{
			ChainId:      order.ChainId,
			TokenAddress: order.TokenCa,
		})
		if err != nil {
			l.Errorf("CreateMarketTx 获取交易对信息失败: token=%s, err=%v", order.TokenCa, err)
			return "", err
		}
	}

	// 构建并执行交易
	txhash, err := l.createMarketTxWithPairInfo(order, pairInfo)
	l.Infof("createMarketTxWithPairInfo 返回: txhash长度=%d, err=%v", len(txhash), err)
	if err != nil {
		return "", err
	}

	l.Infof("CreateMarketTx 返回: txhash长度=%d", len(txhash))
	return txhash, nil
}

// updateDbByTxResult 根据交易结果更新数据库订单状态
// 参数：
//   - order: 订单实体
//   - param: 交易参数（成功时使用）
//   - txHash: 交易哈希
//   - errReason: 错误原因（失败时使用）
func (l *CreateMarketOrderLogic) updateDbByTxResult(order *trademodel.TradeOrder, param *trade2.CreateMarketTx, txHash string, errReason error) error {
	model := trademodel.NewTradeOrderModel(l.svcCtx.DB)

	// 复制ctx防止取消导致更新失败
	dbCtx := trace.ContextWithSpan(context.Background(), trace.SpanFromContext(l.ctx))
	orderData, _ := json.Marshal(order)
	l.Debugf("updateDbByTxResult 订单: %s", string(orderData))

	// 失败情况：更新状态为失败并记录错误原因
	if errReason != nil {
		selectStr := []string{"status", "fail_reason"}
		if param != nil && order.Slippage != int64(param.Slippage) {
			order.Slippage = int64(param.Slippage)
			selectStr = append(selectStr, "slippage")
		}
		order.Status = int64(trade.OrderStatus_Fail)
		order.FailReason = errReason.Error()
		if err := model.UpdateOrderBySelect(dbCtx, order, selectStr...); err != nil {
			l.Errorf("updateDbByTxResult 更新失败: %v", err)
			return xcode.ServerErr
		}
		return nil
	}

	// 成功情况：更新状态为链上交易
	selectStr := []string{"status"}
	order.Status = int64(trade.OrderStatus_OnChain)
	order.TxHash = txHash
	selectStr = append(selectStr, "tx_hash")
	l.Infof("交易哈希已存储: %s", txHash)

	// 更新其他可能变化的字段
	if order.DexName != param.TradePoolName {
		order.DexName = param.TradePoolName
		selectStr = append(selectStr, "dex_name")
	}
	if order.PairCa != param.PairAddr {
		order.PairCa = param.PairAddr
		selectStr = append(selectStr, "pair_ca")
	}
	if order.Slippage != int64(param.Slippage) {
		order.Slippage = int64(param.Slippage)
		selectStr = append(selectStr, "slippage")
	}

	// 执行更新
	err := model.UpdateOrderBySelect(dbCtx, order, selectStr...)
	if err != nil {
		l.Errorf("CreateMarketOrder 更新订单失败: %v", err)
		return xcode.ServerErr
	}

	orderData, _ = json.Marshal(order)
	l.Debugf("CreateMarketOrder 成功订单: %s", string(orderData))
	return nil
}

// createMarketTxWithPairInfo 使用交易对信息构建市场交易
// 该方法负责：
// 1. 获取代币信息
// 2. 判断是否需要价格限制
// 3. 设置输入/输出代币参数
// 4. 调用交易构建和发送方法
// 5. 处理自动滑点重试逻辑
func (l *CreateMarketOrderLogic) createMarketTxWithPairInfo(order *trademodel.TradeOrder, pairInfo *marketclient.GetPairInfoByTokenResponse) (string, error) {
	// 1. 获取代币详细信息
	tokenInfo, err := l.svcCtx.MarketClient.GetTokenInfo(l.ctx, &marketclient.GetTokenInfoRequest{
		ChainId:      order.ChainId,
		TokenAddress: order.TokenCa,
	})
	if err != nil {
		l.Errorf("获取代币信息失败: token=%s, err=%v", order.TokenCa, err)
		return "", err
	}

	// 2. 判断是否启用价格限制（限价单或CLMM池子）
	usePriceLimit := false
	if order.TradeType == int64(trade.TradeType_Limit) ||
		order.TradeType == int64(trade.TradeType_TokenCapLimit) ||
		order.DexName == constants.RaydiumConcentratedLiquidity {
		usePriceLimit = true
	}

	// 3. 设置输入输出代币地址和精度
	inTokenAddr := pairInfo.BaseTokenAddress
	outTokenAddr := pairInfo.TokenAddress
	inDecimal, outDecimal := uint8(pairInfo.BaseTokenDecimal), uint8(pairInfo.TokenDecimal)
	var inTokenProgram, outTokenProgram string

	// 4. 处理Solana链的代币程序
	if order.ChainId == constants.SolChainIdInt {
		inTokenProgram, outTokenProgram = aSDK.TokenProgramID.String(), aSDK.TokenProgramID.String()
		if tokenInfo.Program != "" {
			outTokenProgramAccount, err := aSDK.PublicKeyFromBase58(tokenInfo.Program)
			if nil != err {
				return "", err
			}
			outTokenProgram = outTokenProgramAccount.String()
		}
	}

	// 5. 卖单时交换输入输出
	if order.SwapType == int64(trade.SwapType_Sell) {
		inTokenAddr, outTokenAddr = outTokenAddr, inTokenAddr
		inDecimal, outDecimal = outDecimal, inDecimal
		inTokenProgram, outTokenProgram = outTokenProgram, inTokenProgram
	}

	// 6. 构建交易参数
	param := &trade2.CreateMarketTx{
		UserId:            uint64(order.Uid),
		ChainId:           uint64(order.ChainId),
		UserWalletId:      uint32(order.WalletIndex),
		UserWalletAddress: order.WalletAddress,
		AmountIn:          decimal.NewFromFloat(order.OrderAmount).String(),
		IsAntiMev:         order.IsAntiMev != 0,
		IsAutoSlippage:    order.IsAutoSlippage != 0,
		Slippage:          uint32(order.Slippage),
		GasType:           int32(order.GasType),
		TradePoolName:     pairInfo.Name,
		InDecimal:         inDecimal,
		OutDecimal:        outDecimal,
		InTokenCa:         inTokenAddr,
		OutTokenCa:        outTokenAddr,
		PairAddr:          pairInfo.Address,
		Price:             decimal.NewFromFloat(order.OrderPriceBase).String(),
		UsePriceLimit:     usePriceLimit,
		InTokenProgram:    inTokenProgram,
		OutTokenProgram:   outTokenProgram,
	}

	// 7. 执行交易（支持自动滑点重试）
	var txHash string
	tryTimes := 0
	for tryTimes == 0 || (param.IsAutoSlippage && errors.Is(err, xcode.SlippageLimit) && tryTimes < 3) {
		tryTimes++
		switch tryTimes {
		case 2:
			param.Slippage = 4500 // 45%
			l.Info("自动滑点重试: 调整滑点至45%")
		case 3:
			param.Slippage = 7000 // 70%
			l.Info("自动滑点重试: 调整滑点至70%")
		}
		txHash, err = l.createAndSendTx(param)
		if err != nil {
			err = convertSwapErr(param.TradePoolName, err)
		}
	}

	// 8. 未签名交易（长度>100）直接返回，不更新数据库
	if len(txHash) > 100 {
		l.Infof("返回未签名交易给客户端，长度: %d", len(txHash))
		return txHash, err
	}

	// 9. 更新数据库订单状态
	err2 := l.updateDbByTxResult(order, param, txHash, err)
	if err2 != nil {
		l.Error(err2)
		return "", err
	}

	l.Infof("createMarketTxWithPairInfo 返回: txHash长度=%d, err=%v", len(txHash), err)
	return txHash, err
}

// createAndSendTx 创建并发送交易
// 根据链ID路由到对应的链处理逻辑
func (l *CreateMarketOrderLogic) createAndSendTx(param *trade2.CreateMarketTx) (string, error) {
	l.Debugf("createAndSendTx 参数: UserWalletAddress=%s, InTokenCa=%s, OutTokenCa=%s",
		param.UserWalletAddress, param.InTokenCa, param.OutTokenCa)

	switch param.ChainId {
	case constants.SolChainIdInt:
		if l.svcCtx.SolTxMananger == nil {
			return "", fmt.Errorf("SolTxMananger 未初始化，请检查Sol配置")
		}

		// 构建未签名交易供客户端签名
		unsignedTxBase64, err := l.svcCtx.SolTxMananger.BuildUnsignedTransaction(l.ctx, param)
		if err != nil {
			l.Errorf("SolTxMananger.BuildUnsignedTransaction 失败: %v", err)
			return "", err
		}

		l.Infof("BuildUnsignedTransaction 成功，长度=%d", len(unsignedTxBase64))
		return unsignedTxBase64, nil
	default:
		return "", xcode.RequestErr
	}
}

// convertSwapErr 将底层交换错误转换为业务错误码
// 根据错误信息和池子类型进行错误分类
func convertSwapErr(poolName string, err error) error {
	result := err.Error()

	// 通用错误类型
	if strings.Contains(result, "liquidity") {
		return xcode.PoolLiquidityNotEnough
	}
	if strings.Contains(result, "insufficient") {
		return xcode.BalanceNotEnough
	}
	if strings.Contains(result, "frozen") {
		return xcode.TokenAccountFrozen
	}
	if strings.Contains(result, "slippage") || strings.Contains(result, "TooLittleOutputReceived") {
		return xcode.SlippageLimit
	}

	// 根据池子类型的特定错误
	switch poolName {
	case constants.RaydiumV4, constants.RaydiumCPMM, constants.RaydiumConcentratedLiquidity:
		if strings.Contains(result, "InsufficientLiquidityForDirection") {
			return xcode.PoolLiquidityNotEnough
		}
	case constants.PumpFun:
		if strings.Contains(result, "TooLittleSolReceived") || strings.Contains(result, "attempt to subtract with overflow") {
			return xcode.PumpPoolZeroErr
		}
	}

	return err
}
