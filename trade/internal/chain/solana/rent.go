package solana

import (
	"context"
	"time"

	ag_rpc "github.com/gagliardetto/solana-go/rpc"
	"github.com/zeromicro/go-zero/core/logx"
	"richcode.cc/dex/pkg/constants"
)

func (tm *TxManager) CheckRentFee() {
	tm.rentFee = 2039280
	tm.updateRentFee()
	ticker := time.NewTicker(1 * time.Minute)
	for {
		select {
		case <-ticker.C:
			tm.updateRentFee()
		}
	}
}
func (tm *TxManager) updateRentFee() {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	lamport4Atarent, err := tm.Client.GetMinimumBalanceForRentExemption(ctx, constants.AtaAccountSize, ag_rpc.CommitmentFinalized)
	if nil != err {
		logx.Error(err)
		return
	}
	tm.rentFee = lamport4Atarent
}
