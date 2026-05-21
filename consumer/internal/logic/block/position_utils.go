package block

import (
	"richcode.cc/dex/pkg/types"
)

// extractPositionNftMintFromTrade 从交易中提取 position_nft_mint
// 对于 increase/decrease liquidity 指令，可以从 NFT 账户的 mint 字段获取
func (s *BlockService) extractPositionNftMintFromTrade(trade *types.TradeWithPair) string {
	if trade == nil {
		return ""
	}

	// 对于 increase/decrease liquidity，可以从交易账户中获取
	// 但当前 TradeWithPair 结构可能不包含这些信息
	// 需要从指令账户中解析，或者从交易日志中提取
	// 这里简化处理：如果交易中有相关信息，直接返回

	// TODO: 从指令账户中提取 position_nft_mint
	// 需要访问 decoder 的账户信息，但当前结构不支持
	// 可以考虑在 DecodeRaydiumConcentratedLiquidityIncreaseLiquidityV2 中
	// 将 position_nft_mint 添加到 TradeWithPair 的扩展字段中

	return ""
}

