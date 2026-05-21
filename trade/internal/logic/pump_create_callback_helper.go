package logic

import (
	"context"
	"errors"
	"fmt"
	"math"
	"strings"
	"time"

	sdkclient "github.com/blocto/solana-go-sdk/client"
	"github.com/blocto/solana-go-sdk/rpc"
	ag_solanago "github.com/gagliardetto/solana-go"
	ag_rpc "github.com/gagliardetto/solana-go/rpc"
	"github.com/klen-ygs/gorm-zero/gormc"
	"github.com/shopspring/decimal"
	"github.com/zeromicro/go-zero/core/logx"
	"gorm.io/gorm"

	"richcode.cc/dex/model/solmodel"
	"richcode.cc/dex/pkg/constants"
	"richcode.cc/dex/pkg/pumpfun"
	pumpgen "richcode.cc/dex/pkg/pumpfun/generated/pump"
	"richcode.cc/dex/pkg/util"
)

// PumpFun 代币创建相关常量定义
const (
	initBaseTokenAmount        = 0.015        // 初始基础代币金额（SOL）
	initPumpTokenAmount        = 873000000    // 初始 Pump 代币供应量
	virtualInitPumpTokenAmount = 1073000191   // 虚拟初始代币供应量（用于计算）
	defaultPumpVirtualBase     = 30.0         // 默认虚拟基础代币储备
	defaultSolPriceUSD         = 161.87666258362614 // 默认 SOL 价格（USD）
	pumpMigrationPoint         = 0.999        // Pump 迁移阈值点
	pumpStatusTrading          = 1            // Pump 状态：交易中
	pumpStatusMigrating        = 2            // Pump 状态：迁移中
	pumpStatusEnd              = 3            // Pump 状态：结束
	defaultPumpTypeName        = constants.PumpFun // 默认 Pump 类型名称
	defaultIPFSGateway         = "https://ipfs.io/ipfs/" // 默认 IPFS 网关
)

// CompletePumpCreateBySignature 完成 PumpFun 代币创建的后续处理
// 该函数负责：
// 1. 更新未签名交易记录的状态
// 2. 如果创建成功，从链上获取数据并写入交易对和代币记录
// 3. 使用规范化的元数据填充记录
func CompletePumpCreateBySignature(
	ctx context.Context,
	db *gorm.DB,
	rpcEndpoint string,
	rpcClient *ag_rpc.Client,
	chainId int64,
	mintBase58, userWallet, signature string,
	success bool,
	errMsg string,
	baseTokenPriceUSD float64,
	ipfsGateway string,
) error {
	logger := logx.WithContext(ctx)

	// 1. 查找未签名交易记录
	unsignedModel := solmodel.NewPumpCreateTokenUnsignedModel(db)
	rec, err := unsignedModel.FindOneByChainIdMint(ctx, chainId, mintBase58)
	if err != nil {
		return fmt.Errorf("unsigned tx record not found for chain=%d mint=%s: %w", chainId, mintBase58, err)
	}

	// 2. 如果创建失败，更新状态并返回
	if !success {
		rec.Status = 2           // 标记为失败
		rec.TxSignature = signature
		rec.ErrorMessage = errMsg
		rec.UpdatedAt = time.Now()
		return unsignedModel.Update(ctx, rec)
	}

	// 3. 校验 RPC 客户端
	if rpcClient == nil {
		return fmt.Errorf("rpc client is nil")
	}

	// 4. 解析代币 mint 地址
	mint, err := ag_solanago.PublicKeyFromBase58(mintBase58)
	if err != nil {
		return fmt.Errorf("invalid mint: %w", err)
	}

	// 5. 派生交易对 PDA 地址（Bonding Curve）
	pairPDA, _, err := pumpfun.GetBondingCurvePDA(mint)
	if err != nil {
		return fmt.Errorf("derive bonding curve pda: %w", err)
	}
	pairAddr := pairPDA.String()

	// 6. 获取代币供应量和精度
	decimals, totalSupply := fetchTokenSupply(ctx, rpcEndpoint, mintBase58, logger)

	// 7. 获取 Bonding Curve 状态
	bondingCurve, err := fetchBondingCurveState(ctx, rpcClient, pairPDA)
	if err != nil {
		return fmt.Errorf("fetch bonding curve: %w", err)
	}

	logger.Infof("fetchBondingCurveState: bondingCurve=%+v", bondingCurve)

	// 8. 获取交易所在的区块信息
	slot, blockTime := fetchTxSlotAndTime(ctx, rpcClient, signature, logger)
	if blockTime.IsZero() {
		blockTime = time.Now()
	}

	// 9. 获取基础代币信息（SOL）
	baseToken := util.GetBaseToken(chainId)

	// 10. 计算当前基础代币储备（带默认值保护）
	currentBase := amountToFloat(bondingCurve.RealSolReserves, constants.SolDecimal)
	if currentBase <= 0 || currentBase < initBaseTokenAmount {
		currentBase = initBaseTokenAmount
	}

	// 11. 计算当前代币储备（带默认值保护）
	currentToken := amountToFloat(bondingCurve.RealTokenReserves, int(decimals))
	if currentToken <= 0 || currentToken < float64(virtualInitPumpTokenAmount) {
		currentToken = float64(virtualInitPumpTokenAmount)
	}

	// 12. 计算虚拟基础代币储备（带默认值保护）
	pumpVirtualBase := amountToFloat(bondingCurve.VirtualSolReserves, constants.SolDecimal)
	if pumpVirtualBase <= 0 || pumpVirtualBase < defaultPumpVirtualBase {
		pumpVirtualBase = defaultPumpVirtualBase
	}

	// 13. 计算虚拟代币储备（带默认值保护）
	pumpVirtualToken := amountToFloat(bondingCurve.VirtualTokenReserves, int(decimals))
	if pumpVirtualToken <= 0 {
		pumpVirtualToken = float64(virtualInitPumpTokenAmount)
	}

	// 14. 计算 Pump 进度点和状态
	pumpPoint := calculatePumpPoint(currentToken)
	pumpStatus := determinePumpStatus(pumpPoint, bondingCurve.Complete)

	// 15. 确保总供应量有效
	totalSupply = ensureTotalSupply(totalSupply)

	// 16. 设置基础代币价格（带默认值保护）
	if baseTokenPriceUSD <= 0 {
		baseTokenPriceUSD = defaultSolPriceUSD
	}

	// 17. 计算代币价格、FDV 和流动性
	tokenPrice := calcTokenPrice(currentBase, currentToken, baseTokenPriceUSD)
	fdv := calcFDV(tokenPrice, totalSupply)
	liquidity := calcPumpLiquidity(currentBase, baseTokenPriceUSD)

	logger.Infof("fetchTokenSupply: currentBase=%f, currentToken=%f, pumpVirtualBase=%f, pumpVirtualToken=%f, pumpPoint=%f, pumpStatus=%d, tokenPrice=%f, fdv=%f, liquidity=%f", currentBase, currentToken, pumpVirtualBase, pumpVirtualToken, pumpPoint, pumpStatus, tokenPrice, fdv, liquidity)

	// 18. 确定代币所有者
	owner := strings.TrimSpace(userWallet)
	if owner == "" {
		owner = bondingCurve.Creator.String()
	}

	// 19. 解析代币图标 URL
	icon := resolveTokenIcon(ctx, rec, ipfsGateway)
	now := time.Now()

	// 20. 构建交易对记录
	pairRecord := &solmodel.Pair{
		ChainId:                      chainId,
		Address:                      pairAddr,
		Name:                         defaultPumpTypeName,
		FactoryAddress:               "",
		BaseTokenAddress:             baseToken.Address,
		TokenAddress:                 mintBase58,
		BaseTokenSymbol:              baseToken.Symbol,
		TokenSymbol:                  rec.Symbol,
		BaseTokenDecimal:             baseToken.Decimal,
		TokenDecimal:                 decimals,
		BaseTokenIsNativeToken:       1,
		BaseTokenIsToken0:            0,
		InitBaseTokenAmount:          initBaseTokenAmount,
		InitTokenAmount:              virtualInitPumpTokenAmount,
		CurrentBaseTokenAmount:       currentBase,
		CurrentTokenAmount:           currentToken,
		Fdv:                          fdv,
		MktCap:                       fdv,
		TokenPrice:                   tokenPrice,
		BaseTokenPrice:               baseTokenPriceUSD,
		BlockNum:                     slot,
		BlockTime:                    blockTime,
		HighestTokenPrice:            tokenPrice,
		LatestTradeTime:              blockTime,
		PumpPoint:                    pumpPoint,
		PumpLaunched:                 0,
		PumpMarketCap:                fdv,
		PumpOwner:                    owner,
		PumpSwapPairAddr:             pairAddr,
		PumpVirtualBaseTokenReserves: pumpVirtualBase,
		PumpVirtualTokenReserves:     pumpVirtualToken,
		PumpStatus:                   int64(pumpStatus),
		PumpPairAddr:                 pairAddr,
		Slot:                         slot,
		Liquidity:                    liquidity,
		LaunchPadPoint:               0,
		LaunchPadStatus:              0,
		CreatedAt:                    now,
		UpdatedAt:                    now,
	}

	// 21. 构建代币记录
	tokenRecord := &solmodel.Token{
		ChainId:         chainId,
		Address:         mintBase58,
		Program:         ag_solanago.TokenProgramID.String(),
		Name:            rec.Name,
		Symbol:          rec.Symbol,
		Decimals:        decimals,
		TotalSupply:     totalSupply,
		Icon:            icon,
		Description:     rec.Description,
		Website:         rec.Website,
		Telegram:        rec.Telegram,
		TwitterUsername: rec.Twitter,
		CreatedAt:       now,
		UpdatedAt:       now,
		Slot:            slot,
	}

	// 22. 确保区块时间有效
	if pairRecord.BlockTime.IsZero() {
		pairRecord.BlockTime = now
		pairRecord.LatestTradeTime = now
	}

	// 23. 更新未签名交易记录状态
	rec.Status = 1           // 标记为成功
	rec.TxSignature = signature
	rec.PairAddr = pairAddr
	rec.ErrorMessage = ""
	rec.UpdatedAt = now

	// 24. 在事务中写入数据库
	return db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := upsertPair(ctx, solmodel.NewPairModel(tx), pairRecord); err != nil {
			return err
		}
		if err := upsertToken(ctx, solmodel.NewTokenModel(tx), tokenRecord); err != nil {
			return err
		}
		return solmodel.NewPumpCreateTokenUnsignedModel(tx).Update(ctx, rec)
	})
}

// fetchTokenSupply 获取代币供应量和精度
// 该函数会尝试多种 commitment 级别获取数据，确保数据的可靠性
func fetchTokenSupply(ctx context.Context, endpoint, mint string, logger logx.Logger) (int64, float64) {
	decimals := int64(6) // 默认精度
	client := sdkclient.NewClient(endpoint)
	if client == nil {
		return decimals, 0
	}

	var (
		supply    sdkclient.TokenAmount
		err       error
		attempts  = 4                                             // 重试次数
		commitSet = []rpc.Commitment{rpc.CommitmentProcessed, rpc.CommitmentConfirmed, rpc.CommitmentFinalized} // 尝试不同的确认级别
	)

	// 带重试的获取策略
	for attempt := 0; attempt < attempts; attempt++ {
		if attempt > 0 {
			time.Sleep(time.Duration(attempt) * 250 * time.Millisecond) // 指数退避
		}
		for _, commitment := range commitSet {
			supply, err = client.GetTokenSupplyWithConfig(ctx, mint, sdkclient.GetTokenSupplyConfig{Commitment: commitment})
			if err == nil {
				goto supplyReady
			}
			logger.Infof("GetTokenSupply (commitment=%s) err: %v, mint=%s", commitment, err, mint)
		}
	}

supplyReady:
	if err != nil {
		return decimals, 0
	}

	decimals = int64(supply.Decimals)
	var total float64

	// 优先使用 UIAmountString（已格式化的字符串）
	if supply.UIAmountString != "" {
		if d, err := decimal.NewFromString(supply.UIAmountString); err == nil {
			total = d.InexactFloat64()
		}
	}

	// 备用计算方式
	if total == 0 {
		total = float64(supply.Amount) / math.Pow10(int(supply.Decimals))
	}

	logger.Infof("fetchTokenSupply: supply=%d, decimals=%d, total=%f, mint=%s", supply.Amount, decimals, total, mint)
	return decimals, total
}

// fetchBondingCurveState 获取 Bonding Curve 账户状态
func fetchBondingCurveState(ctx context.Context, rpcClient *ag_rpc.Client, addr ag_solanago.PublicKey) (*pumpgen.BondingCurve, error) {
	info, err := rpcClient.GetAccountInfoWithOpts(ctx, addr, &ag_rpc.GetAccountInfoOpts{
		Encoding:   ag_solanago.EncodingBase64,
		Commitment: ag_rpc.CommitmentConfirmed,
	})
	if err != nil {
		return nil, err
	}
	if info == nil || info.Value == nil {
		return nil, fmt.Errorf("bonding curve account missing")
	}
	data := info.Value.Data.GetBinary()
	if len(data) == 0 {
		return nil, fmt.Errorf("empty bonding curve account data")
	}
	return pumpgen.ParseAccount_BondingCurve(data)
}

// fetchTxSlotAndTime 获取交易所在的区块号和时间
func fetchTxSlotAndTime(ctx context.Context, rpcClient *ag_rpc.Client, sig string, logger logx.Logger) (int64, time.Time) {
	if sig == "" {
		return 0, time.Time{}
	}

	// 解析交易签名
	signature, err := ag_solanago.SignatureFromBase58(sig)
	if err != nil {
		logger.Errorf("invalid signature %s: %v", sig, err)
		return 0, time.Time{}
	}

	// 获取交易信息
	res, err := rpcClient.GetTransaction(ctx, signature, &ag_rpc.GetTransactionOpts{
		Commitment: ag_rpc.CommitmentConfirmed,
	})
	if err != nil || res == nil {
		if err != nil {
			logger.Errorf("GetTransaction err: %v, sig=%s", err, sig)
		}
		return 0, time.Time{}
	}

	// 提取区块时间
	var blockTime time.Time
	if res.BlockTime != nil {
		blockTime = time.Unix(int64(*res.BlockTime), 0)
	}

	return int64(res.Slot), blockTime
}

// calculatePumpPoint 计算 Pump 进度点
// 进度点 = 1 - (当前代币量 / 初始代币量)
// 返回值范围: 0 ~ 1
func calculatePumpPoint(currentToken float64) float64 {
	if currentToken <= 0 {
		return 1
	}
	point := 1 - (currentToken / initPumpTokenAmount)
	if point < 0 {
		point = 0
	}
	if point > 1 {
		point = 1
	}
	return point
}

// determinePumpStatus 根据进度点和完成标志确定 Pump 状态
func determinePumpStatus(point float64, complete bool) int {
	switch {
	case complete:
		return pumpStatusEnd        // 已完成
	case point >= pumpMigrationPoint:
		return pumpStatusMigrating  // 迁移中
	default:
		return pumpStatusTrading    // 交易中
	}
}

// calcTokenPrice 计算代币价格（USD）
// 价格 = (基础代币储备 * 基础代币价格) / 当前代币储备
func calcTokenPrice(currentBase, currentToken, basePrice float64) float64 {
	if currentBase <= 0 || currentToken <= 0 || basePrice <= 0 {
		return 0
	}
	return (currentBase * basePrice) / currentToken
}

// calcFDV 计算 Fully Diluted Valuation（完全稀释估值）
// FDV = 代币价格 * 总供应量
func calcFDV(tokenPrice, totalSupply float64) float64 {
	if tokenPrice <= 0 || totalSupply <= 0 {
		return 0
	}
	return tokenPrice * totalSupply
}

// calcPumpLiquidity 计算流动性
// 流动性 = 基础代币储备 * 基础代币价格 * 2（因为包含两种代币）
func calcPumpLiquidity(currentBase, basePrice float64) float64 {
	if currentBase <= 0 || basePrice <= 0 {
		return 0
	}
	return currentBase * basePrice * 2
}

// amountToFloat 将原始金额（最小单位）转换为浮点数值
func amountToFloat(value uint64, decimals int) float64 {
	if value == 0 {
		return 0
	}
	if decimals <= 0 {
		return float64(value)
	}
	return float64(value) / math.Pow10(decimals)
}

// ensureTotalSupply 确保总供应量有效
// 如果供应量为零或负数，返回默认虚拟初始供应量
func ensureTotalSupply(supply float64) float64 {
	if supply > 0 {
		return supply
	}
	return float64(virtualInitPumpTokenAmount)
}

// resolveTokenIcon 解析代币图标 URL
func resolveTokenIcon(_ context.Context, rec *solmodel.PumpCreateTokenUnsigned, gateway string) string {
	return normalizeIPFSURL(rec.ImageUri, gateway)
}

// normalizeIPFSURL 规范化 IPFS URL
// 将 ipfs:// 格式转换为 HTTP 可访问的 URL
func normalizeIPFSURL(value, gateway string) string {
	trimmed := strings.TrimSpace(value)
	if trimmed == "" {
		return ""
	}

	lower := strings.ToLower(trimmed)
	if strings.HasPrefix(lower, "ipfs://") {
		// 提取 IPFS path
		path := trimmed[len("ipfs://"):]
		path = strings.TrimPrefix(path, "ipfs/")
		path = strings.TrimLeft(path, "/")

		// 构建完整 URL
		gw := strings.TrimSpace(gateway)
		if gw == "" {
			gw = defaultIPFSGateway
		}
		if !strings.HasSuffix(gw, "/") {
			gw += "/"
		}
		return gw + path
	}

	return trimmed
}

// upsertPair 插入或更新交易对记录
func upsertPair(ctx context.Context, model solmodel.PairModel, data *solmodel.Pair) error {
	existing, err := model.FindOneByChainIdAddress(ctx, data.ChainId, data.Address)
	if err != nil {
		if errors.Is(err, gormc.ErrNotFound) {
			return model.Insert(ctx, data)
		}
		return fmt.Errorf("query pair: %w", err)
	}
	updatePairRecord(existing, data)
	return model.Update(ctx, existing)
}

// updatePairRecord 更新交易对记录字段
func updatePairRecord(dst, src *solmodel.Pair) {
	dst.Name = src.Name
	dst.FactoryAddress = src.FactoryAddress
	dst.BaseTokenAddress = src.BaseTokenAddress
	dst.TokenAddress = src.TokenAddress
	dst.BaseTokenSymbol = src.BaseTokenSymbol
	dst.TokenSymbol = src.TokenSymbol
	dst.BaseTokenDecimal = src.BaseTokenDecimal
	dst.TokenDecimal = src.TokenDecimal
	dst.BaseTokenIsNativeToken = src.BaseTokenIsNativeToken
	dst.BaseTokenIsToken0 = src.BaseTokenIsToken0

	// 仅更新非零值
	if src.InitBaseTokenAmount > 0 {
		dst.InitBaseTokenAmount = src.InitBaseTokenAmount
	}
	if src.InitTokenAmount > 0 {
		dst.InitTokenAmount = src.InitTokenAmount
	}

	dst.CurrentBaseTokenAmount = src.CurrentBaseTokenAmount
	dst.CurrentTokenAmount = src.CurrentTokenAmount
	dst.Fdv = src.Fdv
	dst.MktCap = src.MktCap
	dst.TokenPrice = src.TokenPrice
	dst.BaseTokenPrice = src.BaseTokenPrice

	if src.BlockNum > 0 {
		dst.BlockNum = src.BlockNum
	}
	if !src.BlockTime.IsZero() {
		dst.BlockTime = src.BlockTime
	}
	if src.Liquidity > 0 {
		dst.Liquidity = src.Liquidity
	}

	// 价格和时间的特殊处理
	if src.HighestTokenPrice > dst.HighestTokenPrice {
		dst.HighestTokenPrice = src.HighestTokenPrice
	}
	if src.LatestTradeTime.After(dst.LatestTradeTime) {
		dst.LatestTradeTime = src.LatestTradeTime
	}

	// Pump 相关字段
	dst.PumpPoint = src.PumpPoint
	dst.PumpLaunched = src.PumpLaunched
	dst.PumpMarketCap = src.PumpMarketCap
	dst.PumpOwner = src.PumpOwner
	dst.PumpSwapPairAddr = src.PumpSwapPairAddr
	dst.PumpVirtualBaseTokenReserves = src.PumpVirtualBaseTokenReserves
	dst.PumpVirtualTokenReserves = src.PumpVirtualTokenReserves
	dst.PumpStatus = src.PumpStatus
	dst.PumpPairAddr = src.PumpPairAddr
	dst.Slot = src.Slot
	dst.LaunchPadPoint = src.LaunchPadPoint
	dst.LaunchPadStatus = src.LaunchPadStatus
	dst.UpdatedAt = src.UpdatedAt
}

// upsertToken 插入或更新代币记录
func upsertToken(ctx context.Context, model solmodel.TokenModel, data *solmodel.Token) error {
	existing, err := model.FindOneByChainIdAddress(ctx, data.ChainId, data.Address)
	if err != nil {
		if errors.Is(err, gormc.ErrNotFound) {
			return model.Insert(ctx, data)
		}
		return fmt.Errorf("query token: %w", err)
	}
	updateTokenRecord(existing, data)
	return model.Update(ctx, existing)
}

// updateTokenRecord 更新代币记录字段
func updateTokenRecord(dst, src *solmodel.Token) {
	if src.Name != "" {
		dst.Name = src.Name
	}
	if src.Symbol != "" {
		dst.Symbol = src.Symbol
	}
	if src.Decimals > 0 {
		dst.Decimals = src.Decimals
	}
	if src.TotalSupply > 0 {
		dst.TotalSupply = src.TotalSupply
	}
	if src.Icon != "" {
		dst.Icon = src.Icon
	}

	dst.Description = src.Description
	dst.Website = src.Website
	dst.Telegram = src.Telegram
	dst.TwitterUsername = src.TwitterUsername
	dst.Program = src.Program
	dst.Slot = src.Slot
	dst.UpdatedAt = src.UpdatedAt
}
