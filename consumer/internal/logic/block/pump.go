package block

import (
	"bytes"
	"context"
	"encoding/base64"
	"fmt"
	"strings"

	"github.com/blocto/solana-go-sdk/common"
	solTypes "github.com/blocto/solana-go-sdk/types"
	"github.com/near/borsh-go"
	"github.com/shopspring/decimal"
	"github.com/zeromicro/go-zero/core/logx"
	"richcode.cc/dex/consumer/internal/svc"
	constants "richcode.cc/dex/pkg/constants"
	"richcode.cc/dex/pkg/pumpfun/generated/pump"
	"richcode.cc/dex/pkg/rediskeys"
	"richcode.cc/dex/pkg/types"
	"richcode.cc/dex/pkg/util"
)

const (
	// Pump 状态定义
	PumpStatusNotStart  = 0
	PumpStatusCreate    = -1
	PumpStatusTrading   = 1
	PumpStatusMigrating = 2
	PumpStatusEnd       = 3

	// 虚拟储备金和实际储备金之间的差值（单位：最小单位）
	// Token 差值 = VirtualInitPumpTokenAmount - InitPumpTokenAmount = 1073000191 - 873000000 = 200000191
	TokenReservesDiff = 200000191
	// SOL 差值 = 30 SOL - 0.015 SOL = 29.985 SOL = 29985000000 lamports (1 SOL = 10^9 lamports)
	SolReservesDiff = 29985000000

	// Pump 交易账户索引
	// Buy/BuyExactSolIn/Sell 指令账户索引 (参考 @instructions.go NewBuyInstruction/NewBuyExactSolInInstruction/NewSellInstruction)
	// Account 0: global
	// Account 1: fee_recipient
	// Account 2: mint
	// Account 3: bonding_curve (pair)
	// Account 4: associated_bonding_curve
	// Account 5: associated_user (token account)
	// Account 6: user (maker/signer)
	// Account 7: system_program
	// Account 8: token_program (Buy/BuyExactSolIn) / creator_vault (Sell)
	// Account 9: creator_vault (Buy/BuyExactSolIn) / token_program (Sell)
	// Account 10: event_authority
	// Account 11: program
	// BuyExactSolIn 额外账户:
	// Account 12: global_volume_accumulator
	// Account 13: user_volume_accumulator
	// Account 14: fee_config
	// Account 15: fee_program
	pumpSwapPairAccountIndex  = 3  // bonding_curve
	pumpSwapToAccountIndex    = 4  // associated_bonding_curve (接收方)
	pumpSwapTokenAccountIndex = 5  // associated_user (用户 token 账户)
	pumpSwapMakerAccountIndex = 6  // user (交易发起人)
	pumpSwapMinAccountCount   = 12 // 最小账户数量 (Buy/Sell 为 12 个，BuyExactSolIn 为 16 个)

	// Create 指令账户索引 (参考 @instructions.go NewCreateInstruction)
	// Account 0: mint (token address)
	// Account 1: mint_authority
	// Account 2: bonding_curve (pair)
	// Account 3: associated_bonding_curve (pair token account)
	// Account 4: global
	// Account 5: mpl_token_metadata
	// Account 6: metadata
	// Account 7: user (creator/maker)
	// Account 8: system_program
	// Account 9: token_program
	// Account 10: associated_token_program
	// Account 11: rent
	// Account 12: event_authority
	// Account 13: program
	pumpCreateAccountCount          = 14 // 固定 14 个账户
	pumpCreateTokenAccountIndex     = 0  // mint (token 地址)
	pumpCreatePairAccountIndex      = 2  // bonding_curve (交易对地址)
	pumpCreatePairTokenAccountIndex = 3  // associated_bonding_curve (交易对的 token 账户)
	pumpCreateMakerAccountIndex     = 7  // user (创建者)

	// Pump 价格和数量常量
	pumpMigrationPoint          = 0.999
	pumpLogDataPrefix           = "Program data: vdt/007m"
	pumpLogDataMinLength        = 100
	defaultPumpVirtualBaseToken = 30.0
	defaultSolPriceUSD          = 161.87666258362614
)

// DecodePumpEvent 从日志中解码 Pump 事件
func DecodePumpEvent(logs []string) (events []PumpEvent, err error) {
	for _, log := range logs {
		if !isPumpEventLog(log) {
			continue
		}

		event, err := parsePumpEventLog(log)
		if err != nil {
			return nil, err
		}
		events = append(events, event)
	}
	return events, nil
}

// isPumpEventLog 检查日志是否为 Pump 事件日志
func isPumpEventLog(log string) bool {
	return len(log) > pumpLogDataMinLength && strings.HasPrefix(log, pumpLogDataPrefix)
}

// parsePumpEventLog 解析单个 Pump 事件日志
func parsePumpEventLog(log string) (PumpEvent, error) {
	prefix := strings.TrimPrefix(log, "Program data: ")
	data, err := decodeBase64(prefix)
	if err != nil {
		return PumpEvent{}, fmt.Errorf("base64 decode failed: %w", err)
	}

	event, err := DeserializePumpEvent(data)
	if err != nil {
		return PumpEvent{}, fmt.Errorf("borsh deserialize failed: %w", err)
	}

	return event, nil
}

// decodeBase64 尝试使用标准和 Raw 编码解码 base64 字符串
func decodeBase64(s string) ([]byte, error) {
	data, err := base64.StdEncoding.DecodeString(s)
	if err != nil {
		data, err = base64.RawStdEncoding.DecodeString(s)
		if err != nil {
			return nil, err
		}
	}
	return data, nil
}

// DeserializePumpEvent 反序列化 Pump 事件数据
func DeserializePumpEvent(data []byte) (result PumpEvent, err error) {
	err = borsh.Deserialize(&result, data)
	return
}

// DecodePumpFunInstruction 解码 Pump Fun 指令
// 根据指令类型分发到对应的处理函数
// 使用 discriminator 判别器进行指令匹配 (参考 @discriminators.go)
//
// 支持的指令类型:
//   - Buy: 购买 token (参考 @discriminators.go Instruction_Buy)
//   - BuyExactSolIn: 使用精确 SOL 数量购买 token (参考 @discriminators.go Instruction_BuyExactSolIn)
//   - Sell: 卖出 token (参考 @discriminators.go Instruction_Sell)
//   - Create: 创建新的 token 和 bonding curve (参考 @discriminators.go Instruction_Create)
//   - SyncUserVolumeAccumulator: 同步用户交易量 (参考 @discriminators.go Instruction_SyncUserVolumeAccumulator)
func DecodePumpFunInstruction(ctx context.Context, sc *svc.ServiceContext, dtx *DecodedTx, instruction *solTypes.CompiledInstruction, logIndex int) (trade *types.TradeWithPair, err error) {
	discriminator := GetInstructionDiscriminator(instruction.Data)
	if discriminator == nil {
		logx.Errorf("invalid instruction data length, hash: %v", dtx.TxHash)
		return nil, nil
	}

	// 使用 discriminator 判别器进行指令匹配 (参考 @discriminators.go)
	if bytes.Equal(discriminator, pump.Instruction_Buy[:]) ||
		bytes.Equal(discriminator, pump.Instruction_BuyExactSolIn[:]) ||
		bytes.Equal(discriminator, pump.Instruction_Sell[:]) {
		// 处理买卖交易 (参考 @instructions.go NewBuyInstruction/NewSellInstruction/NewBuyExactSolInInstruction)
		return decodePumpSwap(ctx, sc, dtx, instruction, logIndex)
	} else if bytes.Equal(discriminator, pump.Instruction_SyncUserVolumeAccumulator[:]) {
		// 同步指令，不需要处理
		return nil, nil
	} else if bytes.Equal(discriminator, pump.Instruction_Create[:]) {
		// 处理创建交易 (参考 @instructions.go NewCreateInstruction)
		logx.Infof("Find pump fun create tx: %v", dtx.TxHash)
		return DecodePumpCreate(ctx, sc, dtx, instruction, logIndex)
	}

	logx.Errorf("unknown pump instruction discriminator: %v, hash: %v", discriminator, dtx.TxHash)
	return nil, nil
}

// decodePumpSwap 解码 Pump 买卖交易
// 处理 Buy、BuyExactSolIn 和 Sell 指令 (参考 @instructions.go NewBuyInstruction/NewBuyExactSolInInstruction/NewSellInstruction)
//
// Buy/BuyExactSolIn/Sell 指令账户结构 (前 12 个账户相同):
//   - Account 0: global (全局状态)
//   - Account 1: fee_recipient (手续费接收者)
//   - Account 2: mint (token 地址)
//   - Account 3: bonding_curve (交易对地址)
//   - Account 4: associated_bonding_curve (交易对的关联账户)
//   - Account 5: associated_user (用户的 token 账户)
//   - Account 6: user (交易发起人/签名者)
//   - Account 7: system_program (系统程序)
//   - Account 8: token_program (Token 程序) / creator_vault (Sell 时)
//   - Account 9: creator_vault (Buy/BuyExactSolIn) / token_program (Sell 时)
//   - Account 10: event_authority (事件权限)
//   - Account 11: program (Pump 程序)
//   - Account 12+: BuyExactSolIn 额外账户 (global_volume_accumulator, user_volume_accumulator, fee_config, fee_program)
//
// 注意: BuyExactSolIn 有 16 个账户，而 Buy/Sell 有 12 个账户，但前 12 个账户结构相同，可以统一处理
func decodePumpSwap(ctx context.Context, sc *svc.ServiceContext, dtx *DecodedTx, instruction *solTypes.CompiledInstruction, logIndex int) (*types.TradeWithPair, error) {
	// 验证账户数量（至少需要前 12 个账户）
	if len(instruction.Accounts) < pumpSwapMinAccountCount {
		return nil, fmt.Errorf("pump swap instruction account count insufficient: got %d, need %d, hash: %v",
			len(instruction.Accounts), pumpSwapMinAccountCount, dtx.TxHash)
	}

	// 提取账户信息
	accounts := extractPumpSwapAccounts(dtx.Tx.AccountKeys, instruction.Accounts)

	// 获取 token 账户信息
	tokenAccountInfo := dtx.TokenAccountMap[accounts.tokenAccount]
	if tokenAccountInfo == nil {
		return nil, fmt.Errorf("tokenAccountInfo not found for account: %s, hash: %v", accounts.tokenAccount, dtx.TxHash)
	}

	// 解析事件
	event, err := getPumpEventFromTx(dtx)
	if err != nil {
		return nil, err
	}

	// 构建交易信息
	trade := buildPumpTrade(dtx, accounts, tokenAccountInfo, event, logIndex)

	return trade, nil
}

// pumpSwapAccounts 存储 Pump 交易相关的账户地址
type pumpSwapAccounts struct {
	pair         string
	to           string
	maker        string
	tokenAccount string
}

// extractPumpSwapAccounts 从账户列表中提取 Pump 交易相关的账户
// 账户索引映射 (参考 @instructions.go):
//   - accounts[3]: bonding_curve (交易对地址)
//   - accounts[4]: associated_bonding_curve (交易对的关联账户，作为接收方)
//   - accounts[5]: associated_user (用户的 token 账户)
//   - accounts[6]: user (交易发起人/签名者)
func extractPumpSwapAccounts(accountKeys []common.PublicKey, accounts []int) pumpSwapAccounts {
	return pumpSwapAccounts{
		pair:         accountKeys[accounts[pumpSwapPairAccountIndex]].String(),  // Account 3: bonding_curve
		to:           accountKeys[accounts[pumpSwapToAccountIndex]].String(),    // Account 4: associated_bonding_curve
		maker:        accountKeys[accounts[pumpSwapMakerAccountIndex]].String(), // Account 6: user (signer)
		tokenAccount: accountKeys[accounts[pumpSwapTokenAccountIndex]].String(), // Account 5: associated_user
	}
}

// getPumpEventFromTx 从交易中获取 Pump 事件
// 注意: Buy、BuyExactSolIn 和 Sell 指令都会产生相同格式的 PumpEvent 事件
// 因此可以使用统一的事件解析逻辑
func getPumpEventFromTx(dtx *DecodedTx) (PumpEvent, error) {
	// 解码事件（如果尚未解码）
	if len(dtx.PumpEvents) == 0 {
		events, err := DecodePumpEvent(dtx.Tx.Meta.LogMessages)
		if err != nil {
			logx.Errorf("pump swap decode event error: %v, hash: %v", err, dtx.TxHash)
			return PumpEvent{}, err
		}
		dtx.PumpEvents = events
	}

	// 验证事件索引
	if dtx.PumpEventIndex >= len(dtx.PumpEvents) {
		return PumpEvent{}, fmt.Errorf("pump event index out of range: index=%d, len=%d, hash=%v",
			dtx.PumpEventIndex, len(dtx.PumpEvents), dtx.TxHash)
	}

	event := dtx.PumpEvents[dtx.PumpEventIndex]
	dtx.PumpEventIndex++

	return event, nil
}

// buildPumpTrade 构建 Pump 交易信息
func buildPumpTrade(dtx *DecodedTx, accounts pumpSwapAccounts, tokenInfo *TokenAccount, event PumpEvent, logIndex int) *types.TradeWithPair {
	tokenDecimal := tokenInfo.TokenDecimal
	tokenAddress := event.Mint.String()

	// 直接使用事件中的实际储备量（如果事件提供了），否则通过虚拟储备量计算
	// 注意：新版本的 Pump.fun 事件包含 RealSolReserves 和 RealTokenReserves 字段
	realSolReserves := event.RealSolReserves
	realTokenReserves := event.RealTokenReserves

	// 兼容旧版本：如果事件中没有实际储备量，则通过虚拟储备量计算
	if realSolReserves == 0 && event.VirtualSolReserves > 0 {
		if event.VirtualSolReserves > SolReservesDiff {
			realSolReserves = event.VirtualSolReserves - SolReservesDiff
		}
		logx.Infof("buildPumpTrade: calculated realSolReserves from virtual (VirtualSolReserves=%d, realSolReserves=%d), hash=%s",
			event.VirtualSolReserves, realSolReserves, dtx.TxHash)
	}
	if realTokenReserves == 0 && event.VirtualTokenReserves > 0 {
		if event.VirtualTokenReserves > TokenReservesDiff {
			realTokenReserves = event.VirtualTokenReserves - TokenReservesDiff
		}
		logx.Infof("buildPumpTrade: calculated realTokenReserves from virtual (VirtualTokenReserves=%d, realTokenReserves=%d), hash=%s",
			event.VirtualTokenReserves, realTokenReserves, dtx.TxHash)
	}

	// 验证储备量是否合理
	if realSolReserves == 0 && realTokenReserves == 0 {
		logx.Errorf("buildPumpTrade: both real reserves are zero, event may be incomplete, hash=%s", dtx.TxHash)
	}

	trade := &types.TradeWithPair{
		ChainId:  SolChainId,
		TxHash:   dtx.TxHash,
		PairAddr: accounts.pair,
		Maker:    accounts.maker,
		To:       accounts.to,

		CurrentBaseTokenInPoolAmount: decimal.New(int64(realSolReserves), -constants.SolDecimal).InexactFloat64(),
		CurrentTokenInPoolAmount:     decimal.New(int64(realTokenReserves), -int32(tokenDecimal)).InexactFloat64(),
		PumpVirtualBaseTokenReserves: decimal.New(int64(event.VirtualSolReserves), -constants.SolDecimal).InexactFloat64(),
		PumpVirtualTokenReserves:     decimal.New(int64(event.VirtualTokenReserves), -int32(tokenDecimal)).InexactFloat64(),

		BaseTokenAmount:    decimal.New(int64(event.SolAmount), -constants.SolDecimal).InexactFloat64(),
		TokenAmount:        decimal.New(int64(event.TokenAmount), -int32(tokenDecimal)).InexactFloat64(),
		BaseTokenAmountInt: int64(event.SolAmount),
		TokenAmountInt:     int64(event.TokenAmount),

		BaseTokenPriceUSD: dtx.SolPrice,

		BlockNum:         dtx.BlockDb.Slot,
		BlockTime:        dtx.BlockDb.BlockTime.Unix(),
		HashId:           fmt.Sprintf("%v#%d", dtx.BlockDb.Slot, dtx.TxIndex),
		TransactionIndex: dtx.TxIndex,
		LogIndex:         logIndex,

		SwapName:                constants.PumpFun,
		BaseTokenAccountAddress: "",
		TokenAccountAddress:     accounts.tokenAccount,
		PumpLaunched:            false,
		PumpPairAddr:            accounts.pair,
	}

	// 设置交易类型
	if event.IsBuy {
		trade.Type = types.TradeTypeBuy
	} else {
		trade.Type = types.TradeTypeSell
	}

	// 设置交易对信息
	baseToken := util.GetBaseToken(SolChainIdInt)
	trade.PairInfo = types.Pair{
		ChainId:                SolChainId,
		Addr:                   accounts.pair,
		BaseTokenAddr:          baseToken.Address,
		BaseTokenDecimal:       uint8(baseToken.Decimal),
		BaseTokenSymbol:        baseToken.Symbol,
		TokenAddr:              tokenAddress,
		TokenSymbol:            tokenInfo.TokenSymbol, // 使用从数据库获取的 token symbol
		TokenDecimal:           tokenDecimal,
		BlockTime:              dtx.BlockDb.BlockTime.Unix(),
		BlockNum:               dtx.BlockDb.Slot,
		Name:                   constants.PumpFun,
		InitTokenAmount:        float64(VirtualInitPumpTokenAmount),
		InitBaseTokenAmount:    InitSolTokenAmount,
		TokenTotalSupply:       float64(VirtualInitPumpTokenAmount),
		CurrentBaseTokenAmount: trade.CurrentBaseTokenInPoolAmount,
		CurrentTokenAmount:     trade.CurrentTokenInPoolAmount,
	}

	// 计算价格和市值
	calculatePumpPrices(trade)

	// 计算 Pump 进度
	calculatePumpProgress(trade)

	// 添加详细日志，帮助诊断问题
	logx.Infof("buildPumpTrade: pair=%s, type=%s, realSol=%d(%.6f SOL), realToken=%d(%.2f), virtualSol=%d, virtualToken=%d, price=%.10f USD, fdv=%.2f, hash=%s",
		accounts.pair,
		trade.Type,
		realSolReserves, trade.CurrentBaseTokenInPoolAmount,
		realTokenReserves, trade.CurrentTokenInPoolAmount,
		event.VirtualSolReserves, event.VirtualTokenReserves,
		trade.TokenPriceUSD, trade.PumpMarketCap,
		dtx.TxHash)

	return trade
}

// calculatePumpPrices 计算 Pump 交易的价格和市值
func calculatePumpPrices(trade *types.TradeWithPair) {
	if trade == nil {
		return
	}
	trade.TotalUSD = decimal.NewFromFloat(trade.BaseTokenAmount).
		Mul(decimal.NewFromFloat(trade.BaseTokenPriceUSD)).
		InexactFloat64()

	baseReserves := trade.PumpVirtualBaseTokenReserves
	tokenReserves := trade.PumpVirtualTokenReserves
	if baseReserves <= 0 || tokenReserves <= 0 {
		baseReserves = trade.CurrentBaseTokenInPoolAmount
		tokenReserves = trade.CurrentTokenInPoolAmount
	}
	trade.TokenPriceUSD = calcPumpTokenPrice(baseReserves, tokenReserves, trade.BaseTokenPriceUSD)
	updatePumpMarketCap(trade)
	logx.Infof("calculatePumpPrices: trade.TotalUSD: %v, trade.TokenPriceUSD: %v, trade.PairInfo.TokenTotalSupply: %v, trade.PumpMarketCap: %v, virtualBase: %v, virtualToken: %v, realBase: %v, realToken: %v, basePriceUSD: %v",
		trade.TotalUSD,
		trade.TokenPriceUSD,
		trade.PairInfo.TokenTotalSupply,
		trade.PumpMarketCap,
		trade.PumpVirtualBaseTokenReserves,
		trade.PumpVirtualTokenReserves,
		trade.CurrentBaseTokenInPoolAmount,
		trade.CurrentTokenInPoolAmount,
		trade.BaseTokenPriceUSD)
}

// calculatePumpProgress 计算 Pump 进度和状态
func calculatePumpProgress(trade *types.TradeWithPair) {
	if trade == nil {
		return
	}

	initTokenAmount := float64(InitPumpTokenAmount)
	if initTokenAmount <= 0 {
		initTokenAmount = float64(InitPumpTokenAmount)
	}

	currentToken := trade.CurrentTokenInPoolAmount
	if currentToken <= 0 {
		currentToken = trade.PairInfo.CurrentTokenAmount
	}
	if currentToken <= 0 {
		currentToken = trade.PumpVirtualTokenReserves
	}

	var pumpPoint float64
	switch {
	case initTokenAmount <= 0:
		pumpPoint = 0
	case currentToken <= 0:
		pumpPoint = 1
	default:
		pumpPoint = 1 - (currentToken / initTokenAmount)
		pumpPoint = min(max(pumpPoint, 0), 1)
	}

	trade.PumpPoint = pumpPoint
	trade.PumpStatus = PumpStatusTrading

	// 判断是否进入迁移状态
	if trade.PumpPoint >= pumpMigrationPoint {
		trade.PumpStatus = PumpStatusMigrating
		trade.PumpPoint = 1
	}

}

// DecodePumpCreate 解码 Pump 创建指令
// 处理 Create 指令 (参考 @instructions.go NewCreateInstruction)
//
// Create 指令账户结构 (固定 14 个账户):
//   - Account 0: mint (新创建的 token 地址)
//   - Account 1: mint_authority (mint 权限)
//   - Account 2: bonding_curve (交易对地址)
//   - Account 3: associated_bonding_curve (交易对的 token 账户)
//   - Account 4: global (全局状态)
//   - Account 5: mpl_token_metadata (Metaplex token metadata 程序)
//   - Account 6: metadata (token 元数据账户)
//   - Account 7: user (创建者/签名者)
//   - Account 8: system_program (系统程序)
//   - Account 9: token_program (Token 程序)
//   - Account 10: associated_token_program (关联 Token 程序)
//   - Account 11: rent (租金系统变量)
//   - Account 12: event_authority (事件权限)
//   - Account 13: program (Pump 程序本身)
func DecodePumpCreate(ctx context.Context, sc *svc.ServiceContext, dtx *DecodedTx, instruction *solTypes.CompiledInstruction, logIndex int) (trade *types.TradeWithPair, err error) {
	// 验证账户数量
	if len(instruction.Accounts) != pumpCreateAccountCount {
		return nil, fmt.Errorf("pump create instruction account count mismatch: got %d, need %d, hash: %v",
			len(instruction.Accounts), pumpCreateAccountCount, dtx.TxHash)
	}

	// 提取账户信息
	accounts := extractPumpCreateAccounts(dtx.Tx.AccountKeys, instruction.Accounts)

	if shouldSkipPumpCreate(ctx, sc, accounts.pair) {
		logx.WithContext(ctx).Infof("skip pump create pair:%s tx:%s due to redis marker", accounts.pair, dtx.TxHash)
		return nil, nil
	}

	// 获取 token 账户信息
	tokenAccountInfo := dtx.TokenAccountMap[accounts.pairTokenAccount]
	if tokenAccountInfo == nil {
		return nil, fmt.Errorf("pairTokenAccount not found for account: %s, hash: %v", accounts.pairTokenAccount, dtx.TxHash)
	}

	// 构建创建交易信息
	trade = buildPumpCreateTrade(dtx, accounts, tokenAccountInfo, logIndex)

	return trade, nil
}

// pumpCreateAccounts 存储 Pump 创建相关的账户地址
type pumpCreateAccounts struct {
	tokenAddress     string
	pair             string
	pairTokenAccount string
	maker            string
}

// extractPumpCreateAccounts 从账户列表中提取 Pump 创建相关的账户
// 账户索引映射 (参考 @instructions.go NewCreateInstruction):
//   - accounts[0]: mint (新创建的 token 地址)
//   - accounts[2]: bonding_curve (交易对地址)
//   - accounts[3]: associated_bonding_curve (交易对的 token 账户)
//   - accounts[7]: user (创建者/签名者)
func extractPumpCreateAccounts(accountKeys []common.PublicKey, accounts []int) pumpCreateAccounts {
	return pumpCreateAccounts{
		tokenAddress:     accountKeys[accounts[pumpCreateTokenAccountIndex]].String(),     // Account 0: mint
		pair:             accountKeys[accounts[pumpCreatePairAccountIndex]].String(),      // Account 2: bonding_curve
		pairTokenAccount: accountKeys[accounts[pumpCreatePairTokenAccountIndex]].String(), // Account 3: associated_bonding_curve
		maker:            accountKeys[accounts[pumpCreateMakerAccountIndex]].String(),     // Account 7: user (creator)
	}
}

// buildPumpCreateTrade 构建 Pump 创建交易信息
func buildPumpCreateTrade(dtx *DecodedTx, accounts pumpCreateAccounts, tokenInfo *TokenAccount, logIndex int) *types.TradeWithPair {
	baseToken := util.GetBaseToken(SolChainIdInt)

	tokenDecimal := tokenInfo.TokenDecimal
	currentToken := decimal.New(tokenInfo.PostValue, -int32(tokenDecimal)).InexactFloat64()
	virtualInitToken := float64(VirtualInitPumpTokenAmount)
	realInitToken := float64(InitPumpTokenAmount)
	if currentToken <= 0 || currentToken < realInitToken {
		currentToken = realInitToken
	}

	currentBase := InitSolTokenAmount
	pumpVirtualBase := defaultPumpVirtualBaseToken
	pumpVirtualToken := virtualInitToken

	baseTokenPriceUSD := dtx.SolPrice
	if baseTokenPriceUSD <= 0 {
		baseTokenPriceUSD = defaultSolPriceUSD
	}

	tokenSupply := ensurePumpTotalSupply(currentToken)
	tokenPriceUSD := calcPumpTokenPrice(currentBase, currentToken, baseTokenPriceUSD)
	fdv := calcPumpFDV(tokenPriceUSD, tokenSupply)
	pumpPoint := calculatePumpPointFromAmount(currentToken, tokenDecimal)

	trade := &types.TradeWithPair{
		ChainId:  SolChainId,
		TxHash:   dtx.TxHash,
		PairAddr: accounts.pair,
		Maker:    accounts.maker,
		To:       "",
		Type:     types.TradePumpCreate,

		CurrentTokenInPoolAmount:     currentToken,
		CurrentBaseTokenInPoolAmount: currentBase,
		PumpVirtualBaseTokenReserves: pumpVirtualBase,
		PumpVirtualTokenReserves:     pumpVirtualToken,

		BaseTokenPriceUSD: baseTokenPriceUSD,
		TokenPriceUSD:     tokenPriceUSD,

		BlockNum:         dtx.BlockDb.Slot,
		BlockTime:        dtx.BlockDb.BlockTime.Unix(),
		HashId:           fmt.Sprintf("%v#%d", dtx.BlockDb.Slot, dtx.TxIndex),
		TransactionIndex: dtx.TxIndex,
		LogIndex:         logIndex,

		SwapName:                constants.PumpFun,
		BaseTokenAccountAddress: "",
		PumpLaunched:            false,
		PumpPoint:               pumpPoint,
		PumpPairAddr:            accounts.pair,
		PumpStatus:              PumpStatusCreate,
		PumpMarketCap:           fdv,
		Fdv:                     fdv,
		Mcap:                    fdv,
	}

	// 设置交易对信息
	trade.PairInfo = types.Pair{
		ChainId:                SolChainId,
		Addr:                   accounts.pair,
		BaseTokenAddr:          baseToken.Address,
		BaseTokenDecimal:       uint8(baseToken.Decimal),
		BaseTokenSymbol:        baseToken.Symbol,
		TokenAddr:              accounts.tokenAddress,
		TokenSymbol:            tokenInfo.TokenSymbol,
		TokenDecimal:           tokenDecimal,
		BlockTime:              dtx.BlockDb.BlockTime.Unix(),
		BlockNum:               dtx.BlockDb.Slot,
		Name:                   constants.PumpFun,
		InitTokenAmount:        float64(VirtualInitPumpTokenAmount),
		InitBaseTokenAmount:    InitSolTokenAmount,
		TokenTotalSupply:       tokenSupply,
		CurrentBaseTokenAmount: currentBase,
		CurrentTokenAmount:     currentToken,
	}

	trade.TotalUSD = decimal.NewFromFloat(currentBase).
		Mul(decimal.NewFromFloat(baseTokenPriceUSD)).
		InexactFloat64()

	// Note: WebSocket push moved to pair.go for complete token data

	return trade
}

func calcPumpTokenPrice(currentBase, currentToken, basePrice float64) float64 {
	if currentBase <= 0 || currentToken <= 0 || basePrice <= 0 {
		return 0
	}
	return (currentBase * basePrice) / currentToken
}

func calcPumpFDV(tokenPrice, totalSupply float64) float64 {
	if tokenPrice <= 0 || totalSupply <= 0 {
		return 0
	}
	return tokenPrice * totalSupply
}

func ensurePumpTotalSupply(supply float64) float64 {
	if supply > 0 {
		return supply
	}
	return float64(VirtualInitPumpTokenAmount)
}

func calculatePumpPointFromAmount(currentToken float64, tokenDecimal uint8) float64 {
	if currentToken <= 0 {
		return 1
	}
	initTokenAmount := float64(InitPumpTokenAmount)
	point := 1 - (currentToken / initTokenAmount)
	if point < 0 {
		point = 0
	}
	if point > 1 {
		point = 1
	}
	return point
}

func updatePumpMarketCap(trade *types.TradeWithPair) {
	if trade == nil {
		return
	}
	supply := trade.PairInfo.TokenTotalSupply
	if supply <= 0 || trade.TokenPriceUSD <= 0 {
		return
	}
	trade.PumpMarketCap = decimal.NewFromFloat(trade.TokenPriceUSD).
		Mul(decimal.NewFromFloat(supply)).
		InexactFloat64()
}

func shouldSkipPumpCreate(ctx context.Context, sc *svc.ServiceContext, pairAddr string) bool {
	if sc == nil || sc.Redis == nil || pairAddr == "" {
		return false
	}
	chainId := sc.Config.Sol.ChainId
	if chainId == 0 {
		chainId = constants.SolChainIdInt
	}
	key := rediskeys.PumpPairCreatedKey(chainId, pairAddr)
	exists, err := sc.Redis.Exists(key)
	if err != nil {
		logx.WithContext(ctx).Errorf("check redis pump pair key:%s err:%v", key, err)
		return false
	}
	return exists
}

func GetInstructionDiscriminator(data []byte) []byte {
	if len(data) < 8 || data == nil {
		return nil
	}
	return data[:8]
}
