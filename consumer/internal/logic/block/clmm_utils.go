package block

import (
	"math"

	"github.com/zeromicro/go-zero/core/logx"
	"richcode.cc/dex/pkg/types"
)

// 1. 根据交易中的输入输出vault地址，获取对应的代币余额和小数位数，并格式化为UI展示的金额
// 2. 根据交易中的基础币和交易币地址，将获取到的余额和小数位数更新到trade对象中对应的池子token余额和精度字段
func (decoder *ConcentratedLiquidityDecoder) fillVaultBalances(trade *types.TradeWithPair, inputVaultAddr, outputVaultAddr, baseMint, tokenMint string) {
	if trade == nil {
		return
	}
	cli := decoder.svcCtx.GetSolClient()
	if cli == nil {
		logx.Infof("fillVaultBalances: no sol client available, skip. tx=%s", trade.TxHash)
		return
	}
	// 定义一个函数来获取指定地址的代币余额和小数位数，返回格式化后的余额和小数位数
	getBal := func(addr string) (uiAmt float64, decimals uint8) {
		if addr == "" {
			return 0, 0
		}
		resp, err := cli.GetTokenAccountBalance(decoder.ctx, addr)
		if err != nil {
			logx.Infof("fillVaultBalances: failed to get balance for %s err=%v tx=%s", addr, err, trade.TxHash)
			return 0, 0
		}
		decimals = uint8(resp.Decimals)
		uiAmt = float64(resp.Amount) / math.Pow10(int(decimals))
		logx.Infof("fillVaultBalances: vault %s balance=%.6f decimals=%d tx=%s", addr, uiAmt, decimals, trade.TxHash)
		return
	}

	inBal, inDec := getBal(inputVaultAddr)
	outBal, outDec := getBal(outputVaultAddr)

	// 定义一个函数来根据mint地址和余额信息更新trade对象中的池子token余额和精度
	assign := func(mint string, amt float64, dec uint8) {
		if amt <= 0 {
			return
		}
		if mint == trade.PairInfo.BaseTokenAddr {
			trade.CurrentBaseTokenInPoolAmount = amt
			trade.PairInfo.CurrentBaseTokenAmount = amt
			if trade.PairInfo.BaseTokenDecimal == 0 && dec > 0 {
				trade.PairInfo.BaseTokenDecimal = dec
			}
		} else if mint == trade.PairInfo.TokenAddr {
			trade.CurrentTokenInPoolAmount = amt
			trade.PairInfo.CurrentTokenAmount = amt
			if trade.PairInfo.TokenDecimal == 0 && dec > 0 {
				trade.PairInfo.TokenDecimal = dec
			}
		}
	}
	assign(baseMint, inBal, inDec)
	assign(tokenMint, outBal, outDec)
}
